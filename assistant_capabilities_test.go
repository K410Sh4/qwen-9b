package main

import (
	"errors"
	"strings"
	"testing"
)

func TestCapabilityRegistryIsSingleSourceOfTruth(t *testing.T) {
	ids := make(map[string]bool)
	binaries := make(map[string]bool)
	for _, entry := range capabilityRegistry {
		if entry.ID == "" || ids[entry.ID] || entry.Name == "" || entry.Detail == "" {
			t.Fatalf("capacidade inválida ou duplicada: %+v", entry)
		}
		ids[entry.ID] = true
		if !strings.Contains(assistantSystemPrompt, entry.Detail) ||
			!strings.Contains(assistantCapabilitiesAnswer(), entry.Detail) {
			t.Errorf("entrada não refletida no prompt e resposta: %q", entry.ID)
		}
		if entry.Executable == "" {
			continue
		}
		if entry.Scope != capabilityOverlay || entry.InstallRoot == "" || len(entry.Aliases) == 0 {
			t.Fatalf("permissão de execução incompleta: %+v", entry)
		}
		if binaries[entry.Executable] {
			t.Errorf("executável duplicado: %q", entry.Executable)
		}
		binaries[entry.Executable] = true
		for _, alias := range entry.Aliases {
			actual, ok := findAllowedDesktopApp(alias)
			if !ok || actual.ID != entry.ID {
				t.Errorf("alias inválido %q: %+v", alias, actual)
			}
		}
	}
	if len(binaries) != 3 || !binaries["calc.exe"] || !binaries["notepad.exe"] || !binaries["explorer.exe"] {
		t.Fatalf("allowlist de aplicativos foi alterada: %+v", binaries)
	}
	for _, bad := range []string{"steam", "powershell", "cmd.exe", "C:/Windows/System32/cmd.exe", "outro programa", "calculadora && cmd"} {
		if _, ok := findAllowedDesktopApp(bad); ok {
			t.Fatalf("aplicativo não autorizado: %q", bad)
		}
	}
	for _, required := range []string{"Steam", "--classic", "RSS/Atom", "GitHub", "RAM", "automaticamente"} {
		if !strings.Contains(assistantSystemPrompt, required) {
			t.Fatalf("instrução do modelo omite %q", required)
		}
	}
}

func TestCapabilityQuestionParaphrasesAndNegativeExamples(t *testing.T) {
	cases := []struct {
		input   string
		broad   bool
		detail  string
	}{
		{"oqeu vc é capaz de fazer?", true, ""},
		{"O que você é capaz de fazer?", true, ""},
		{"O que você pode fazer?", true, ""},
		{"O que você faz?", true, ""},
		{"Quais são suas habilidades?", true, ""},
		{"Quais as suas capacidades?", true, ""},
		{"Você consegue acessar a internet, abrir apps no meu PC e criar programas?", true, ""},
		{"Consegue abrir a Steam?", false, "arbitrary-apps"},
		{"Você pode navegar na internet?", false, "web-browser"},
		{"Pode compilar um executável?", false, "go-project"},
		{"Você pode se atualizar sozinho?", false, "updates"},
		{"Você consegue medir a memória RAM?", false, "status"},
		{"Você pode abrir a calculadora?", false, "calculadora"},
		{"Como abrir a calculadora?", false, ""},
		{"Abra a calculadora", false, ""},
		{"Novo projeto: uma calculadora com internet", false, ""},
		{"Quais são os efeitos da internet?", false, ""},
		{"A calculadora é útil", false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			if got := assistantCapabilitiesQuestion(tc.input); got != tc.broad {
				t.Errorf("pergunta geral: %v; esperado %v", got, tc.broad)
			}
			if !tc.broad {
				if got := assistantCapabilityInquiry(tc.input); got != tc.detail {
					t.Errorf("pergunta específica: %q; esperado %q", got, tc.detail)
				}
			}
		})
	}
}

func TestCapabilityAnswerAndStatusSemantics(t *testing.T) {
	answer := assistantCapabilitiesAnswer()
	for _, item := range capabilityRegistry {
		if !strings.Contains(answer, item.Detail) {
			t.Errorf("resposta não contém %q", item.ID)
		}
	}
	if !strings.Contains(answer, "não memória RAM") || !strings.Contains(answer, "automaticamente") ||
		!strings.Contains(answer, "Steam") {
		t.Fatal("inventário perdeu limites relevantes")
	}
	if got := assistantCapabilityDetail("web-browser"); !strings.Contains(got, "não implementado") {
		t.Errorf("navegação web indevidamente anunciada: %q", got)
	}
	if got := assistantCapabilityDetail("go-project"); !strings.Contains(got, "somente --classic") {
		t.Errorf("projeto Go anunciado no painel: %q", got)
	}
	if got := assistantCapabilityDetail("updates"); !strings.Contains(got, "automaticamente") {
		t.Errorf("OTA não reconhecido: %q", got)
	}
}

func TestActionAcknowledgementDependsOnToolResult(t *testing.T) {
	a := newAssistant(nil, nil, "")
	calls := 0
	if err := a.submit("Abra a calculadora", func(intent desktopIntent) (string, error) {
		calls++
		return "", errors.New("Windows recusou a abertura")
	}); err != nil {
		t.Fatal(err)
	}
	state := a.snapshot()
	if calls != 1 || !strings.Contains(state.Error, "Windows recusou") {
		t.Fatalf("falha da ferramenta foi escondida: %+v", state)
	}
	for _, msg := range state.Messages {
		if msg.Role == "assistant" && strings.Contains(msg.Content, "Solicitei") {
			t.Fatal("falha foi apresentada como sucesso")
		}
	}
	if err := a.submit("Abra a calculadora", func(intent desktopIntent) (string, error) {
		calls++
		return "Solicitei ao Windows a abertura da Calculadora.", nil
	}); err != nil {
		t.Fatal(err)
	}
	state = a.snapshot()
	if calls != 2 || state.Error != "" ||
		state.Messages[len(state.Messages)-1].Content != "Solicitei ao Windows a abertura da Calculadora." {
		t.Fatalf("sucesso não dependeu do retorno da ferramenta: %+v", state)
	}
	if err := a.submit("Consegue abrir a Steam?", func(i desktopIntent) (string, error) {
		calls++
		if i.Kind != "capability_detail" || i.Value != "arbitrary-apps" {
			return "", errors.New("roteamento de capacidade incorreto")
		}
		return assistantCapabilityDetail(i.Value), nil
	}); err != nil {
		t.Fatal(err)
	}
	state = a.snapshot()
	if calls != 3 || !strings.Contains(state.Messages[len(state.Messages)-1].Content, "não implementado") {
		t.Fatal("pergunta específica tentou executar um programa")
	}
}

func TestCapabilityIntentDoesNotExecuteFromModelText(t *testing.T) {
	for _, phrase := range []string{
		"Como abrir a calculadora?", "A calculadora é útil", "Abra o PowerShell",
		"Instale o jogo Steam em C:/Windows", "Você acha que devo abrir o bloco de notas?",
	} {
		if got := assistantIntent(phrase); got.Kind == "open" {
			t.Errorf("comando não autorizado: %q => %+v", phrase, got)
		}
	}
	for _, phrase := range []string{"Abra a calculadora", "Abra o bloco de notas", "Abra o explorador de arquivos"} {
		if got := assistantIntent(phrase); got.Kind != "open" {
			t.Errorf("comando válido não reconhecido: %q => %+v", phrase, got)
		}
	}
}
