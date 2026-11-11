package main

import (
	"context"
	"errors"
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
		{"A calculadora é útil", "", ""},
		{"Abra C:\\Windows\\System32\\cmd.exe", "", ""},
	}
	for _, c := range cases {
		if got := assistantIntent(c.text); got != (desktopIntent{c.kind, c.value}) {
			t.Errorf("%q: %+v", c.text, got)
		}
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
	a := newAssistant(nil, func(context.Context) (string, error) { return "Abra a calculadora", nil })
	a.reply = func(_ context.Context, lines []dialogueLine) (string, error) {
		if lines[len(lines)-1].Content != "O que você faz?" {
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
	if err := a.submit("O que você faz?", execute); err != nil {
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
