package main

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "net/http"
    "strings"
    "time"
    "unicode/utf8"
)

// assistantReplyStream consumes Ollama's JSON event stream without waiting for
// the complete response. Only text emitted by the assistant is forwarded.
func assistantReplyStream(ctx context.Context, history []dialogueLine, onDelta func(string) error) (string, error) {
    if len(history) > 10 {
        history = history[len(history)-10:]
    }
    messages := []dialogueLine{{"system", assistantSystemPrompt}}
    messages = append(messages, history...)
    body, err := json.Marshal(map[string]any{
        "model": model, "messages": messages, "stream": true, "think": false,
        "options": map[string]any{"num_ctx": 6144, "num_predict": 640, "temperature": 0.55},
    })
    if err != nil {
        return "", err
    }
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://127.0.0.1:11434/api/chat", strings.NewReader(string(body)))
    if err != nil {
        return "", err
    }
    req.Header.Set("Content-Type", "application/json")
    client := &http.Client{Transport: &http.Transport{}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
    defer client.CloseIdleConnections()
    res, err := client.Do(req)
    if err != nil {
        return "", fmt.Errorf("Abra o Ollama local com %s e tente novamente: %w", model, err)
    }
    defer res.Body.Close()
    if res.StatusCode != http.StatusOK {
        data, readErr := readLimited(res.Body, 1<<16)
        if readErr != nil {
            return "", readErr
        }
        return "", fmt.Errorf("Ollama HTTP %d: %s", res.StatusCode, shortened(string(data), 300))
    }
    decoder := json.NewDecoder(io.LimitReader(res.Body, 1<<20))
    var answer strings.Builder
    finished := false
    for {
        var event struct {
            Message struct { Content string }
            Done bool
            Error string
        }
        if err := decoder.Decode(&event); err != nil {
            if errors.Is(err, io.EOF) {
                break
            }
            return "", fmt.Errorf("stream do Ollama interrompido: %w", err)
        }
        if event.Error != "" {
            return "", errors.New(shortened(event.Error, 300))
        }
        if event.Message.Content != "" {
            if answer.Len()+len(event.Message.Content) > 5000 {
                return "", errors.New("resposta do Ollama excede 5.000 bytes")
            }
            answer.WriteString(event.Message.Content)
            if onDelta != nil {
                if err := onDelta(event.Message.Content); err != nil {
                    return "", err
                }
            }
        }
        if event.Done {
            finished = true
            break
        }
    }
    if !finished {
        return "", errors.New("o Ollama terminou sem sinalizar a resposta completa")
    }
    result := strings.TrimSpace(answer.String())
    if result == "" {
        return "", errors.New("o modelo respondeu sem texto")
    }
    return result, nil
}

// speechSegments keeps partial Ollama tokens until a speakable phrase ends.
// The length limit avoids stalling speech on long text without punctuation.
type speechSegments struct {
    pending string
}

const speechSegmentLimit = 300

func (s *speechSegments) Add(delta string) []string {
    s.pending += delta
    var phrases []string
    for s.pending != "" {
        boundary := sentenceBoundary(s.pending)
        if boundary == 0 && len(s.pending) > speechSegmentLimit {
            boundary = strings.LastIndexAny(s.pending[:speechSegmentLimit], " \t\r\n")
            if boundary < speechSegmentLimit/2 {
                boundary = speechSegmentLimit
                for boundary > 0 && !utf8.RuneStart(s.pending[boundary]) {
                    boundary--
                }
            }
        }
        if boundary == 0 {
            break
        }
        if chunk := strings.TrimSpace(s.pending[:boundary]); chunk != "" {
            phrases = append(phrases, chunk)
        }
        s.pending = s.pending[boundary:]
    }
    return phrases
}

func (s *speechSegments) Flush() string {
    phrase := strings.TrimSpace(s.pending)
    s.pending = ""
    return phrase
}

func sentenceBoundary(s string) int {
    for i := 0; i < len(s); i++ {
        ch := s[i]
        if ch == '\n' {
            return i + 1
        }
        if ch != '.' && ch != '!' && ch != '?' {
            continue
        }
        if ch == '.' && i > 0 && i+1 < len(s) && asciiDigit(s[i-1]) && asciiDigit(s[i+1]) {
            continue
        }
        if ch == '.' && speechAbbreviation(s[:i]) {
            continue
        }
        end := i + 1
        for end < len(s) && (s[end] == '"' || s[end] == '\'' || s[end] == ')' || s[end] == ']') {
            end++
        }
        if end < len(s) && (s[end] == ' ' || s[end] == '\t' || s[end] == '\r' || s[end] == '\n') {
            return end
        }
    }
    return 0
}

func asciiDigit(ch byte) bool { return ch >= '0' && ch <= '9' }

func speechAbbreviation(prefix string) bool {
    lower := strings.ToLower(prefix)
    for _, word := range []string{"dr", "dra", "sr", "sra", "prof", "profa", "etc", "av", "ex"} {
        if strings.HasSuffix(lower, word) {
            start := len(lower) - len(word)
            if start == 0 || lower[start-1] == ' ' || lower[start-1] == '\n' {
                return true
            }
        }
    }
    return false
}

func (a *assistantDesk) streamConversation(ctx context.Context, history []dialogueLine) {
    a.mu.Lock()
    voice := a.state.Voice && (a.speakStream != nil || a.speak != nil)
    profile, _ := findVoiceProfile(a.state.VoiceProfile)
    speak, speakStream := a.speak, a.speakStream
    a.mu.Unlock()

    speechCtx, cancelSpeech := context.WithTimeout(ctx, 8*time.Minute)
    defer cancelSpeech()
    var queue chan string
    var speechDone chan error
    if voice {
        queue = make(chan string, 32)
        speechDone = make(chan error, 1)
        go func() {
            var err error
            if speakStream != nil {
                err = speakStream(speechCtx, queue, profile)
            } else {
                for phrase := range queue {
                    if err == nil {
                        err = speak(speechCtx, phrase, profile)
                    }
                }
            }
            // Continue consuming if synthesis failed so the Ollama producer
            // cannot deadlock on a full queue.
            if err != nil && speakStream != nil {
                for range queue {
                }
            }
            speechDone <- err
        }()
    }
    pushPhrase := func(phrase string) error {
        if !voice || phrase == "" {
            return nil
        }
        select {
        case queue <- phrase:
            a.mu.Lock()
            if a.state.Activity == "thinking" {
                a.state.Activity = "speaking"
                a.state.Revision++
            }
            a.mu.Unlock()
            return nil
        case <-ctx.Done():
            return ctx.Err()
        case <-speechCtx.Done():
            return speechCtx.Err()
        }
    }

    var segments speechSegments
    answer, err := a.streamReply(ctx, history, func(delta string) error {
        if ctx.Err() != nil {
            return ctx.Err()
        }
        a.mu.Lock()
        a.state.Partial += delta
        a.state.Revision++
        a.mu.Unlock()
        if voice {
            for _, phrase := range segments.Add(delta) {
                if err := pushPhrase(phrase); err != nil {
                    return err
                }
            }
        }
        return nil
    })
    if err == nil && voice {
        if pending := segments.Flush(); pending != "" {
            err = pushPhrase(pending)
        }
    }
    if voice {
        close(queue)
    }
    a.mu.Lock()
    if a.ctx.Err() == nil {
        a.state.Partial = ""
        a.state.Revision++
        if err != nil {
            a.state.Error, a.state.Activity = shortened(err.Error(), 500), "ready"
        } else {
            a.add("assistant", answer)
            if voice {
                a.state.Activity = "speaking"
            } else {
                a.state.Activity = "ready"
            }
        }
    }
    a.mu.Unlock()
    if !voice {
        return
    }
    if err != nil {
        cancelSpeech()
    }
    speechErr := <-speechDone
    a.mu.Lock()
    defer a.mu.Unlock()
    if a.ctx.Err() != nil {
        return
    }
    if speechErr != nil && err == nil {
        a.state.Error = shortened(speechErr.Error(), 400)
    }
    a.state.Activity = "ready"
    a.state.Revision++
}
