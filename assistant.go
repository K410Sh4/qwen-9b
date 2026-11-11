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
	switch normalized {
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
		switch app {
		case "calculadora", "bloco de notas", "explorador", "explorador de arquivos":
			return desktopIntent{Kind: "open", Value: app}
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
	reply     func(context.Context, []dialogueLine) (string, error)
	speak     func(context.Context, string, voiceProfile) error
	voiceFile string
	listen    func(context.Context) (string, error)
}

func newAssistant(speak func(context.Context, string, voiceProfile) error, listen func(context.Context) (string, error), voiceFile string) *assistantDesk {
	ctx, cancel := context.WithCancel(context.Background())
	return &assistantDesk{
		ctx: ctx, cancel: cancel,
		state: assistantState{Revision: 1, Activity: "ready", VoiceProfile: readVoiceProfile(voiceFile), Messages: []dialogueLine{{"assistant", "Estou aqui. Podemos conversar, acompanhar suas missões ou começar um projeto. Para construir algo, escreva “Novo projeto:” e o objetivo."}}},
		reply: assistantReply, speak: speak, listen: listen, voiceFile: voiceFile,
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
		result, err := action(intent)
		a.finish(result, err)
		return nil
	}
	go func() {
		defer a.workers.Done()
		ctx, cancel := context.WithTimeout(a.ctx, 4*time.Minute)
		defer cancel()
		answer, err := a.reply(ctx, history)
		a.finish(answer, err)
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
		ctx, cancel := context.WithTimeout(a.ctx, 45*time.Second)
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
	messages := []dialogueLine{{"system", "Você é Jarvis Local, um assistente pessoal do Windows. Converse naturalmente em português brasileiro. Responda com clareza e sem fingir que executou ações no PC. Comandos do aplicativo são disparados pelo usuário, por exemplo: Mostre o andamento, Abra a calculadora, Abra projetos e Novo projeto: objetivo. Trate os turnos como conversa e não invente estado do computador."}}
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
