package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAssistantCommandsRequireExplicitOrder(t *testing.T) {
	cases := []struct{ text, kind, value string }{
		{"Abra a calculadora", "open", "calculadora"},
		{"Novo projeto: Conte as linhas do CSV", "build", "Conte as linhas do CSV"},
		{"/projeto Faça isto: conte as linhas", "build", "Faça isto: conte as linhas"},
		{"Verifique atualizações", "update", ""},
		{"Pause a sessão", "pause", ""},
		{"Como abrir a calculadora?", "", ""},
		{"Abra o PowerShell", "", ""},
        {"Abra o bloco de notas", "open", "bloco de notas"},
        {"Abra o explorador de arquivos", "open", "explorador"},
        {"oqeu vc é capaz de fazer?", "capabilities", ""},
        {"O que você é capaz de fazer?", "capabilities", ""},
        {"Você pode navegar na internet?", "capability_detail", "web-browser"},
        {"Você pode se atualizar sozinho?", "capability_detail", "updates"},
        {"Pode compilar um executável?", "capability_detail", "go-project"},
        {"Quais são suas capacidades?", "capabilities", ""},
        {"Você consegue acessar a internet, abrir apps no meu PC, criar programas com executável ou modificar o próprio código?", "capabilities", ""},
        {"Consegue abrir a Steam?", "capability_detail", "arbitrary-apps"},
		{"A calculadora é útil", "", ""},
		{"Abra C:\\Windows\\System32\\cmd.exe", "", ""},
	}
	for _, c := range cases {
		if got := assistantIntent(c.text); got != (desktopIntent{c.kind, c.value}) {
			t.Errorf("%q: %+v", c.text, got)
		}
	}
}

func TestAssistantTaskExpressionReflectsActiveAction(t *testing.T) {
    cases := []struct {
        name, command, want string
    }{
        {"open", "Abra a calculadora", "executing"},
        {"update", "Verifique atualizações", "checking"},
        {"build", "Novo projeto: calculadora de testes", "building"},
        {"review", "Ver proposta da melhoria", "reviewing"},
        {"dev-start", "Melhore seu código: corrigir o layout", "building"},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            a := newAssistant(nil, nil, "")
            entered, release := make(chan struct{}), make(chan struct{})
            finished := make(chan error, 1)
            go func() {
                finished <- a.submit(tc.command, func(_ desktopIntent) (string, error) {
                    close(entered)
                    <-release
                    return "Comando processado.", nil
                })
            }()
            select {
            case <-entered:
            case <-time.After(time.Second):
                t.Fatal("a ação não iniciou")
            }
            if got := a.snapshot().Activity; got != tc.want {
                t.Errorf("durante a ação: %s, queria %s", got, tc.want)
            }
            close(release)
            select {
            case err := <-finished:
                if err != nil { t.Fatal(err) }
            case <-time.After(time.Second):
                t.Fatal("a ação não terminou")
            }
            if got := a.snapshot().Activity; got != "ready" {
                t.Errorf("após concluir ação: %s", got)
            }
        })
    }
}

func waitAssistant(t *testing.T, a *assistantDesk) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if a.snapshot().Activity == "ready" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("conversa não ficou pronta")
}

func TestAssistantSeparatesConversationActionsAndDictation(t *testing.T) {
	a := newAssistant(nil, func(context.Context) (string, error) { return "Abra a calculadora", nil }, "")
	a.reply = func(_ context.Context, lines []dialogueLine) (string, error) {
		if lines[len(lines)-1].Content != "Me explique uma curiosidade sobre astronomia." {
			return "", errors.New("contexto errado")
		}
		return "Abra a calculadora", nil
	}
	actions := 0
	execute := func(intent desktopIntent) (string, error) {
		actions++
		if intent.Value != "calculadora" {
			return "", errors.New("ação errada")
		}
		return "Aplicativo aberto.", nil
	}
	if err := a.submit("Me explique uma curiosidade sobre astronomia.", execute); err != nil {
		t.Fatal(err)
	}
	waitAssistant(t, a)
	if actions != 0 {
		t.Fatal("a resposta do modelo executou uma ação")
	}
	if err := a.record(); err != nil {
		t.Fatal(err)
	}
	waitAssistant(t, a)
	if actions != 0 || a.takeDraft() != "Abra a calculadora" || a.takeDraft() != "" {
		t.Fatal("ditado deve virar apenas um rascunho")
	}
	if err := a.submit("Abra a calculadora", execute); err != nil {
		t.Fatal(err)
	}
	s := a.snapshot()
	if actions != 1 || s.Messages[len(s.Messages)-1].Content != "Aplicativo aberto." {
		t.Fatal("ação explícita não executada")
	}
	if err := a.submit(strings.Repeat("x", 4001), execute); err == nil {
		t.Fatal("mensagem fora do limite aceita")
	}
}

func TestAssistantVoiceProfilesPreviewAndPersist(t *testing.T) {
	if len(voiceProfiles) != 1 {
		t.Fatal("quantidade de vozes incorreta")
	}
	path := filepath.Join(t.TempDir(), "voz.txt")
	type spoken struct {
		text    string
		profile voiceProfile
	}
	played := make(chan spoken, 3)
	speak := func(_ context.Context, text string, profile voiceProfile) error {
		played <- spoken{text, profile}
		return nil
	}
	a := newAssistant(speak, nil, path)
	if s := a.snapshot(); s.VoiceProfile != "pt_BR-faber-medium" || s.Voice {
		t.Fatal(s)
	}
	if err := a.selectVoiceProfile("inexistente"); err == nil {
		t.Fatal("aceitou voz desconhecida")
	}
	for _, name := range []string{"pt_BR-faber-medium"} {
		if err := a.selectVoiceProfile(name); err != nil {
			t.Fatal(err)
		}
		if err := a.previewVoice(); err != nil {
			t.Fatal(err)
		}
		waitAssistant(t, a)
		clip := <-played
		if clip.profile.ID != name || !strings.Contains(clip.text, clip.profile.Name) {
			t.Fatalf("prévia: %+v", clip)
		}
		if s := a.snapshot(); s.Voice || len(s.Messages) != 1 {
			t.Fatal("a prévia alterou o chat")
		}
	}
	reopened := newAssistant(speak, nil, path)
	if reopened.snapshot().VoiceProfile != "pt_BR-faber-medium" {
		t.Fatal("não recuperou a escolha")
	}
	reopened.setVoice(true)
	reopened.finish("Estou respondendo.", nil)
	waitAssistant(t, reopened)
	clip := <-played
	if clip.profile.ID != "pt_BR-faber-medium" || clip.text != "Estou respondendo." {
		t.Fatalf("fala: %+v", clip)
	}
}

func TestAssistantStopCancelsSpeechAndPreventsNewWork(t *testing.T) {
	started := make(chan struct{})
	a := newAssistant(func(ctx context.Context, _ string, _ voiceProfile) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}, nil, "")
	if err := a.previewVoice(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("a fala não iniciou")
	}
	select {
	case <-a.stop():
	case <-time.After(time.Second):
		t.Fatal("a fala continuou após encerrar")
	}
	if err := a.previewVoice(); err == nil {
		t.Fatal("aceitou nova prévia após encerrar")
	}
	if err := a.submit("Olá", nil); err == nil {
		t.Fatal("aceitou nova conversa após encerrar")
	}
}

func TestOldVoiceSettingsUsePiperDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "voz.txt")
	for _, legacy := range []string{"natural", "grave", "clara", "pf_dora", "pm_alex", "pt_BR-edresson-low"} {
		if err := saveVoiceProfile(path, legacy); err != nil {
			t.Fatal(err)
		}
		if got := readVoiceProfile(path); got != "pt_BR-faber-medium" {
			t.Errorf("%s: voz padrão = %s", legacy, got)
		}
	}
}
