package main

import (
    "strings"
    "testing"
)

func TestAssistantCapabilitiesTruthful(t *testing.T) {
    if !strings.Contains(assistantSystemPrompt, "Steam") ||
       !strings.Contains(assistantSystemPrompt, "--classic") ||
       !strings.Contains(assistantSystemPrompt, "RSS/Atom") ||
       !strings.Contains(assistantSystemPrompt, "GitHub") {
        t.Fatal("o contrato do modelo perdeu capacidades ou restrições reais")
    }
    cases := []struct{ question string; expected bool }{
        {"Quais são suas capacidades?", true},
        {"O que você consegue fazer?", true},
        {"Você consegue acessar a internet, abrir apps no meu PC, criar programas executáveis ou modificar o próprio código?", true},
        {"Você consegue gerar programas e abrir aplicativos?", true},
        {"Consegue abrir a Steam?", false},
        {"Abra a calculadora", false},
        {"Novo projeto: aplicativo com internet", false},
        {"Quais são os efeitos da internet?", false},
    }
    for _, tc := range cases {
        if actual := assistantCapabilitiesQuestion(tc.question); actual != tc.expected {
            t.Errorf("%q: obtido %v, esperado %v", tc.question, actual, tc.expected)
        }
    }
    answer := assistantCapabilitiesAnswer()
    for _, required := range []string{"Calculadora", "Bloco de Notas", "Explorador", "Steam", "RSS/Atom", "Go", ".exe", "--classic", "GitHub"} {
        if !strings.Contains(answer, required) {
            t.Fatalf("falta %q na descrição de capacidades", required)
        }
    }
}
