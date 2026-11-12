package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type desktopIntent struct{ Kind, Value string }

var desktopAccent = strings.NewReplacer("á", "a", "à", "a", "â", "a", "ã", "a", "é", "e", "ê", "e", "í", "i", "ó", "o", "ô", "o", "õ", "o", "ú", "u", "ç", "c")

func assistantIntent(input string) desktopIntent {
	text := strings.TrimSpace(input)
	normalized := desktopAccent.Replace(strings.ToLower(strings.TrimRight(text, "!. ")))
    if assistantCapabilitiesQuestion(text) {
        return desktopIntent{Kind: "capabilities"}
    }
    if id := assistantCapabilityInquiry(text); id != "" {
        return desktopIntent{Kind: "capability_detail", Value: id}
    }
	switch normalized {
    case "/classic", "/classico", "/ambiente", "modo classico", "abrir modo classico", "abra o modo classico", "abrir ferramentas", "abrir area de trabalho", "mostrar area de trabalho":
        return desktopIntent{Kind: "classic"}
	case "/melhoria", "status da melhoria", "status do auto melhorador", "/melhoria status":
		return desktopIntent{Kind: "dev_status"}
	case "ver proposta da melhoria", "revisar melhoria", "/melhoria revisar":
		return desktopIntent{Kind: "dev_review"}
	case "/melhoria publicar", "enviar melhoria para revisao":
		return desktopIntent{Kind: "dev_publish"}
	case "abrir pasta da melhoria":
		return desktopIntent{Kind: "dev_folder"}
	case "/status", "status", "mostre o andamento", "mostrar andamento", "qual o andamento?":
		return desktopIntent{Kind: "status"}
	case "/projetos", "abrir projetos", "abra projetos", "abra os projetos", "projetos":
		return desktopIntent{Kind: "projects"}
	case "/missoes", "abrir missoes", "abra missoes", "abra as missoes", "missoes":
		return desktopIntent{Kind: "missions"}
	case "/pausar", "pause o trabalho", "pause a sessao", "pausar sessao":
		return desktopIntent{Kind: "pause"}
	case "/retomar", "retome a sessao":
		return desktopIntent{Kind: "resume"}
	case "retome o projeto", "retomar projeto":
		return desktopIntent{Kind: "resumeProject"}
	case "mostre o andamento do projeto":
		return desktopIntent{Kind: "statusProject"}
	case "/pasta", "abra a pasta da entrega", "abra a pasta do projeto", "abrir pasta do projeto":
		return desktopIntent{Kind: "folder"}
	case "/atualizar", "verifique atualizacoes", "verificar atualizacoes":
		return desktopIntent{Kind: "update"}
	}
	for _, prefix := range []string{"melhore seu codigo:", "melhore o jarvis:", "jarvis melhore seu codigo:", "/melhorar "} {
		if strings.HasPrefix(normalized, prefix) {
			offset := strings.Index(text, ":")
			if prefix == "/melhorar " {
				offset = len("/melhorar") 
			}
			if offset >= 0 && offset+1 <= len(text) {
				return desktopIntent{Kind: "dev_start", Value: strings.TrimSpace(text[offset+1:])}
			}
		}
	}
	for _, prefix := range []string{"/projeto ", "novo projeto:", "crie um projeto:", "criar projeto:"} {
		if strings.HasPrefix(normalized, prefix) {
			index := strings.Index(text, ":")
			if prefix == "/projeto " {
				index = strings.Index(text, " ")
			}
			return desktopIntent{Kind: "build", Value: strings.TrimSpace(text[index+1:])}
		}
	}
	for _, prefix := range []string{"/abrir ", "abra ", "abrir ", "abra o ", "abra a ", "abra um ", "abra uma ", "abrir o ", "abrir a "} {
		if !strings.HasPrefix(normalized, prefix) {
			continue
		}
		app := strings.TrimSpace(strings.TrimPrefix(normalized, prefix))
		if registered, ok := findAllowedDesktopApp(app); ok {
			return desktopIntent{Kind: "open", Value: registered.ID}
		}
	}
	return desktopIntent{}
}

type dialogueLine struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type assistantState struct {
	Revision     uint64         `json:"revision"`
	Messages     []dialogueLine `json:"messages"`
	Activity     string         `json:"activity"`
	Draft        string         `json:"draft"`
	Partial      string         `json:"partial,omitempty"`
	Error        string         `json:"error"`
	Voice        bool           `json:"voice"`
	VoiceProfile string         `json:"voiceProfile"`
}

type assistantDesk struct {
	mu        sync.Mutex
	workers   sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
	state     assistantState
	reply       func(context.Context, []dialogueLine) (string, error)
	streamReply func(context.Context, []dialogueLine, func(string) error) (string, error)
	speak       func(context.Context, string, voiceProfile) error
	speakStream func(context.Context, <-chan string, voiceProfile) error
	voiceFile string
	listen    func(context.Context) (string, error)
}

func newAssistant(speak func(context.Context, string, voiceProfile) error, listen func(context.Context) (string, error), voiceFile string) *assistantDesk {
	ctx, cancel := context.WithCancel(context.Background())
	return &assistantDesk{
		ctx: ctx, cancel: cancel,
		state: assistantState{Revision: 1, Activity: "ready", VoiceProfile: readVoiceProfile(voiceFile), Messages: []dialogueLine{{"assistant", "Jarvis pronto. Converse, abra Projetos ou Missões, crie projetos Go e acompanhe propostas de melhoria. Para listar os recursos reais, pergunte: Quais são suas capacidades?"}}},
		streamReply: assistantReplyStream, speak: speak, listen: listen, voiceFile: voiceFile,
	}
}
func (a *assistantDesk) snapshot() assistantState {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.state
	s.Messages = append([]dialogueLine{}, s.Messages...)
	return s
}
func (a *assistantDesk) snapshotAfter(revision uint64) *assistantState {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state.Revision == revision {
		return nil
	}
	state := a.state
	state.Messages = append([]dialogueLine{}, state.Messages...)
	return &state
}

func (a *assistantDesk) setVoice(v bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state.Voice != v {
		a.state.Voice = v
		a.state.Revision++
	}
}
// note records a background Dev Agent result without interrupting ongoing
// Ollama streaming or launching TTS, and wakes the WebView2 revision poll.
func (a *assistantDesk) note(message string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ctx.Err() != nil {
		return
	}
	a.add("assistant", message)
	a.state.Revision++
}

func (a *assistantDesk) add(role, content string) {
	a.state.Messages = append(a.state.Messages, dialogueLine{role, content})
	if len(a.state.Messages) > 32 {
		a.state.Messages = append([]dialogueLine(nil), a.state.Messages[len(a.state.Messages)-32:]...)
	}
}

func (a *assistantDesk) submit(text string, action func(desktopIntent) (string, error)) error {
	text = strings.TrimSpace(text)
	if text == "" || len(text) > 4000 {
		return errors.New("escreva uma mensagem de até 4.000 bytes")
	}
	intent := assistantIntent(text)
	a.mu.Lock()
	if a.ctx.Err() != nil {
		a.mu.Unlock()
		return errors.New("o Jarvis está encerrando")
	}
	if a.state.Activity != "ready" {
		a.mu.Unlock()
		return errors.New("o Jarvis está ocupado")
	}
	a.state.Error, a.state.Draft, a.state.Activity = "", "", "thinking"
	a.state.Revision++
	a.add("user", text)
	history := append([]dialogueLine(nil), a.state.Messages...)
	if intent.Kind == "" {
		a.workers.Add(1)
	}
	a.mu.Unlock()
	if intent.Kind != "" {
		if action == nil {
			a.finish("", errors.New("ação não disponível nesta interface"))
			return nil
		}
		result, err := action(intent)
		a.finish(result, err)
		return nil
	}
	go func() {
		defer a.workers.Done()
		ctx, cancel := context.WithTimeout(a.ctx, 4*time.Minute)
		defer cancel()
		if a.reply != nil {
			answer, err := a.reply(ctx, history)
			a.finish(answer, err)
		} else {
			a.streamConversation(ctx, history)
		}
	}()
	return nil
}

func (a *assistantDesk) finish(answer string, err error) {
	a.mu.Lock()
	if a.ctx.Err() != nil {
		a.mu.Unlock()
		return
	}
	a.state.Revision++
	a.state.Partial = ""
	if err != nil {
		a.state.Error, a.state.Activity = shortened(err.Error(), 500), "ready"
		a.mu.Unlock()
		return
	}
	a.add("assistant", answer)
	if !a.state.Voice || a.speak == nil {
		a.state.Activity = "ready"
		a.mu.Unlock()
		return
	}
	a.startSpeechLocked(answer)
	a.mu.Unlock()
}

func (a *assistantDesk) startSpeechLocked(text string) {
	profile, _ := findVoiceProfile(a.state.VoiceProfile)
	a.state.Activity = "speaking"
	a.state.Revision++
	a.workers.Add(1)
	go func() {
		defer a.workers.Done()
		ctx, cancel := context.WithTimeout(a.ctx, 8*time.Minute)
		defer cancel()
		err := a.speak(ctx, shortened(text, 1200), profile)
		a.mu.Lock()
		defer a.mu.Unlock()
		if err != nil {
			a.state.Error = shortened(err.Error(), 400)
		}
		a.state.Activity = "ready"
		a.state.Revision++
	}()
}
func (a *assistantDesk) record() error {
	a.mu.Lock()
	if a.ctx.Err() != nil || a.state.Activity != "ready" || a.listen == nil {
		a.mu.Unlock()
		return errors.New("o microfone está indisponível enquanto o Jarvis trabalha")
	}
	a.state.Activity, a.state.Error, a.state.Draft = "listening", "", ""
	a.state.Revision++
	a.workers.Add(1)
	a.mu.Unlock()
	go func() {
		defer a.workers.Done()
		ctx, cancel := context.WithTimeout(a.ctx, 22*time.Second)
		defer cancel()
		result, err := a.listen(ctx)
		a.mu.Lock()
		defer a.mu.Unlock()
		a.state.Activity = "ready"
		a.state.Revision++
		if err != nil {
			a.state.Error = shortened(err.Error(), 400)
			return
		}
		a.state.Draft = shortened(strings.TrimSpace(result), 4000)
	}()
	return nil
}
func (a *assistantDesk) takeDraft() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	value := a.state.Draft
	if value != "" {
		a.state.Draft = ""
		a.state.Revision++
	}
	return value
}

func (a *assistantDesk) stop() <-chan struct{} {
	a.mu.Lock()
	a.cancel()
	a.mu.Unlock()
	done := make(chan struct{})
	go func() { a.workers.Wait(); close(done) }()
	return done
}

func assistantReply(ctx context.Context, history []dialogueLine) (string, error) {
	if len(history) > 10 {
		history = history[len(history)-10:]
	}
	messages := []dialogueLine{{"system", assistantSystemPrompt}}
	messages = append(messages, history...)
	body, err := json.Marshal(map[string]any{"model": model, "messages": messages, "stream": false, "think": false,
		"options": map[string]any{"num_ctx": 6144, "num_predict": 640, "temperature": 0.55}})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "http://127.0.0.1:11434/api/chat", strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Transport: &http.Transport{}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Abra o Ollama local com %s e tente novamente.", model)
	}
	defer res.Body.Close()
	data, err := readLimited(res.Body, 1<<20)
	if err != nil {
		return "", err
	}
	var response struct {
		Message dialogueLine
		Error   string
	}
	if err = json.Unmarshal(data, &response); err != nil {
		return "", err
	}
	if res.StatusCode != 200 {
		return "", fmt.Errorf("Ollama HTTP %d: %s", res.StatusCode, shortened(response.Error, 300))
	}
	answer := strings.TrimSpace(response.Message.Content)
	if answer == "" {
		return "", errors.New("o modelo respondeu sem texto")
	}
	return shortened(answer, 5000), nil
}
