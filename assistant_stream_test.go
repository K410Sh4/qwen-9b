package main

import (
    "context"
    "errors"
    "strings"
    "sync"
    "testing"
    "time"
)

func TestSpeechSegmentsSplitNaturally(t *testing.T) {
    var segment speechSegments
    if got := segment.Add("O valor é 3."); len(got) != 0 {
        t.Fatalf("quebrou um número antes do próximo token: %q", got)
    }
    got := segment.Add("14. A Dra. Ana chegou! Tudo bem? ")
    want := []string{"O valor é 3.14.", "A Dra. Ana chegou!", "Tudo bem?"}
    if len(got) != len(want) {
        t.Fatalf("frases: %q; esperado: %q", got, want)
    }
    for i := range want {
        if got[i] != want[i] {
            t.Fatalf("frase %d: %q != %q", i, got[i], want[i])
        }
    }
    if s := segment.Flush(); s != "" {
        t.Fatalf("resto inesperado: %q", s)
    }
}

func TestSpeechSegmentsBoundLongText(t *testing.T) {
    var segment speechSegments
    chunks := segment.Add(strings.Repeat("palavra ", 90))
    if len(chunks) == 0 {
        t.Fatal("frase sem pontuação não foi liberada")
    }
    for _, chunk := range chunks {
        if len(chunk) > speechSegmentLimit {
            t.Fatalf("frase sem limite: %d bytes", len(chunk))
        }
    }
}

func TestAssistantStartsSpeakingBeforeOllamaFinishes(t *testing.T) {
    started := make(chan string, 1)
    release := make(chan struct{})
    var mu sync.Mutex
    var spoken []string
    a := newAssistant(func(_ context.Context, phrase string, _ voiceProfile) error {
        mu.Lock()
        spoken = append(spoken, phrase)
        mu.Unlock()
        select {
        case started <- phrase:
        default:
        }
        return nil
    }, nil, "")
    a.setVoice(true)
    a.streamReply = func(ctx context.Context, _ []dialogueLine, emit func(string) error) (string, error) {
        if err := emit("Primeira frase. "); err != nil {
            return "", err
        }
        select {
        case <-release:
        case <-ctx.Done():
            return "", ctx.Err()
        }
        if err := emit("Segunda frase."); err != nil {
            return "", err
        }
        return "Primeira frase. Segunda frase.", nil
    }
    if err := a.submit("Explique", nil); err != nil {
        t.Fatal(err)
    }
    select {
    case first := <-started:
        if first != "Primeira frase." {
            t.Fatalf("começou com %q", first)
        }
    case <-time.After(time.Second):
        t.Fatal("não falou antes de terminar a resposta")
    }
    if a.snapshot().Partial != "Primeira frase. " {
        t.Fatalf("texto incremental não apareceu: %q", a.snapshot().Partial)
    }
    close(release)
    waitAssistant(t, a)
    mu.Lock()
    defer mu.Unlock()
    if len(spoken) != 2 || spoken[1] != "Segunda frase." {
        t.Fatalf("ordem das frases: %q", spoken)
    }
    state := a.snapshot()
    if state.Partial != "" || state.Messages[len(state.Messages)-1].Content != "Primeira frase. Segunda frase." {
        t.Fatalf("estado final: %+v", state)
    }
}

func TestAssistantStreamingErrorClearsPartial(t *testing.T) {
    a := newAssistant(nil, nil, "")
    a.streamReply = func(ctx context.Context, _ []dialogueLine, emit func(string) error) (string, error) {
        if err := emit("Incompleto"); err != nil {
            return "", err
        }
        return "", errors.New("conexão interrompida")
    }
    if err := a.submit("Olá", nil); err != nil {
        t.Fatal(err)
    }
    waitAssistant(t, a)
    state := a.snapshot()
    if state.Partial != "" || !strings.Contains(state.Error, "conexão interrompida") {
        t.Fatalf("falha de streaming: %+v", state)
    }
}

func TestAssistantStreamingStopCancelsGenerator(t *testing.T) {
    started := make(chan struct{})
    a := newAssistant(nil, nil, "")
    a.streamReply = func(ctx context.Context, _ []dialogueLine, _ func(string) error) (string, error) {
        close(started)
        <-ctx.Done()
        return "", ctx.Err()
    }
    if err := a.submit("Olá", nil); err != nil {
        t.Fatal(err)
    }
    <-started
    select {
    case <-a.stop():
    case <-time.After(time.Second):
        t.Fatal("geração incremental não encerrou")
    }
}
