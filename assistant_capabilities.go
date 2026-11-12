package main

import (
	"fmt"
	"strings"
	"unicode"
)

// Registered capabilities are application contracts, not promises from the LLM.
// The same registry drives factual answers, system instructions and app access.
type capabilityScope string

const (
	capabilityOverlay capabilityScope = "painel"
	capabilityClassic capabilityScope = "classico"
	capabilityAbsent  capabilityScope = "indisponivel"
)

type capabilityDefinition struct {
	ID          string
	Name        string
	Scope       capabilityScope
	Detail      string
	Example     string
	Aliases     []string
	Executable  string
	InstallRoot string // "system" or "windows"; only for explicitly allowed apps
}

var capabilityRegistry = []capabilityDefinition{
	{
		ID: "conversation", Name: "Conversa com Qwen", Scope: capabilityOverlay,
		Detail: "Responde perguntas em texto com Qwen local via Ollama. Respostas do modelo não executam ferramentas.",
	},
	{
		ID: "dictation", Name: "Ditado", Scope: capabilityOverlay,
		Detail: "Recebe ditado pelo reconhecimento de fala do Windows, quando disponível; o resultado vira rascunho, não comando automático.",
	},
	{
		ID: "speech", Name: "Voz Faber", Scope: capabilityOverlay,
		Detail: "Reproduz respostas por Piper Faber local quando a opção de voz está habilitada.",
	},
	{
		ID: "calculadora", Name: "Calculadora", Scope: capabilityOverlay,
		Detail: "Solicita ao Windows a abertura da Calculadora por comando explícito. Início do processo não comprova janela visível.",
		Example: "Abra a calculadora", Aliases: []string{"calculadora"}, Executable: "calc.exe", InstallRoot: "system",
	},
	{
		ID: "bloco de notas", Name: "Bloco de Notas", Scope: capabilityOverlay,
		Detail: "Solicita ao Windows a abertura do Bloco de Notas por comando explícito.",
		Example: "Abra o bloco de notas", Aliases: []string{"bloco de notas", "bloco de nota"}, Executable: "notepad.exe", InstallRoot: "system",
	},
	{
		ID: "explorador", Name: "Explorador de Arquivos", Scope: capabilityOverlay,
		Detail: "Solicita ao Windows a abertura do Explorador de Arquivos por comando explícito.",
		Example: "Abra o explorador", Aliases: []string{"explorador", "explorador de arquivos"}, Executable: "explorer.exe", InstallRoot: "windows",
	},
	{
		ID: "status", Name: "Andamento de missões", Scope: capabilityOverlay,
		Detail: "Consulta o andamento de missões, não memória RAM, CPU, temperatura ou telemetria do PC.",
		Example: "Mostre o andamento",
	},
	{
		ID: "updates", Name: "Atualizações oficiais", Scope: capabilityOverlay,
		Detail: "Consulta releases oficiais do GitHub, verifica SHA-256 e pode atualizar automaticamente quando ocioso. Não edita seu código-fonte.",
		Example: "Verifique atualizações",
	},
	{
		ID: "feeds", Name: "Fontes RSS/Atom", Scope: capabilityClassic,
		Detail: "No módulo de missões --classic, baixa fontes públicas RSS/Atom previamente configuradas; não pesquisa livremente na web.",
	},
	{
		ID: "go-project", Name: "Projetos Go para terminal", Scope: capabilityClassic,
		Detail: "No modo --classic, cria e altera projetos Go de terminal, testa e pode compilar programa.exe sob restrições e aprovação. O overlay não inicia projetos.",
	},
	{
		ID: "arbitrary-apps", Name: "Outros programas e Steam", Scope: capabilityAbsent,
		Detail: "Steam, jogos e aplicativos arbitrários não são abertos pelo painel; não há descoberta por nomes aproximados ou execução livre.",
	},
	{
		ID: "web-browser", Name: "Navegação web geral", Scope: capabilityAbsent,
		Detail: "O chat não possui navegador ou ferramenta geral de busca. Atualizações e RSS/Atom não são acesso irrestrito à internet.",
	},
	{
		ID: "self-modify", Name: "Autoedição de código-fonte", Scope: capabilityAbsent,
		Detail: "O modelo não altera seu próprio código-fonte nem instala builds que ele gerou; recebe releases externas verificadas.",
	},
}

func capabilityByID(id string) (capabilityDefinition, bool) {
	for _, item := range capabilityRegistry {
		if item.ID == id {
			return item, true
		}
	}
	return capabilityDefinition{}, false
}

// This allowlist is shared by the command parser and the Windows launcher.
// Neither a language-model response nor a filename supplied by the user can
// add entries or select a binary outside this registry.
func findAllowedDesktopApp(name string) (capabilityDefinition, bool) {
	normalized := normalizeCapabilityText(name)
	for _, item := range capabilityRegistry {
		if item.Scope != capabilityOverlay || item.Executable == "" {
			continue
		}
		for _, alias := range item.Aliases {
			if normalized == normalizeCapabilityText(alias) {
				return item, true
			}
		}
	}
	return capabilityDefinition{}, false
}

func normalizeCapabilityText(input string) string {
	q := desktopAccent.Replace(strings.ToLower(strings.TrimSpace(input)))
	var buffer strings.Builder
	for _, r := range q {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			buffer.WriteRune(r)
		} else {
			buffer.WriteByte(' ')
		}
	}
	words := strings.Fields(buffer.String())
	for i, word := range words {
		switch word {
		case "vc", "vcs":
			words[i] = "voce"
		case "oqeu", "oque":
			words[i] = "o que"
		}
	}
	return strings.Join(strings.Fields(strings.Join(words, " ")), " ")
}

func containsAny(value string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(value, term) {
			return true
		}
	}
	return false
}

func assistantCapabilitiesQuestion(input string) bool {
	q := normalizeCapabilityText(input)
	switch q {
	case "quais sao suas capacidades", "quais as suas capacidades", "quais sao as suas capacidades",
		"quais sao seus recursos", "quais funcionalidades voce tem", "quais ferramentas voce possui",
		"o que voce consegue fazer", "o que voce pode fazer", "o que voce faz",
		"o que voce sabe fazer", "o que voce e capaz de fazer", "o que o jarvis faz",
		"do que voce e capaz", "como voce pode me ajudar", "voce tem acesso ao meu computador",
		"o que voce e capaz", "qual e sua capacidade":
		return true
	}
	if strings.HasPrefix(q, "quais ") && containsAny(q, "suas habilidades", "suas capacidades", "suas funcoes", "suas funcionalidades") {
		return true
	}
	if !(strings.HasPrefix(q, "voce consegue ") || strings.HasPrefix(q, "voce pode ") ||
		strings.HasPrefix(q, "jarvis consegue ") || strings.HasPrefix(q, "consegue ")) {
		return false
	}
	topics := 0
	for _, term := range []string{"internet", "aplicativ", "apps", "programa", "executavel", "codigo", "computador", "se atualizar"} {
		if strings.Contains(q, term) {
			topics++
		}
	}
	return topics >= 2
}

// Specific capability questions never run the underlying tool.
func assistantCapabilityInquiry(input string) string {
	q := normalizeCapabilityText(input)
	if !(strings.HasPrefix(q, "voce consegue ") || strings.HasPrefix(q, "voce pode ") ||
		strings.HasPrefix(q, "consegue ") || strings.HasPrefix(q, "pode ") ||
		strings.HasPrefix(q, "o jarvis consegue ") || strings.HasPrefix(q, "o jarvis pode ") ||
		strings.HasPrefix(q, "jarvis consegue ") || strings.HasPrefix(q, "tem como voce ")) {
		return ""
	}
	switch {
	case containsAny(q, "steam", "qualquer programa", "qualquer aplicativo", "jogos"):
		return "arbitrary-apps"
	case containsAny(q, "navegar", "pesquisar na internet", "pesquisar na web", "abrir sites", "internet"):
		return "web-browser"
	case containsAny(q, "seu codigo", "proprio codigo", "se modificar", "se reprogramar"):
		return "self-modify"
	case containsAny(q, "compilar", "criar exe", "gerar exe", "executavel", "criar programas"):
		return "go-project"
	case containsAny(q, "atualizar sozinho", "auto atualizar", "novas versoes"):
		return "updates"
	case containsAny(q, "memoria ram", "uso de ram", "cpu", "temperatura"):
		return "status"
	}
	for _, item := range capabilityRegistry {
		if item.Executable == "" {
			continue
		}
		for _, alias := range item.Aliases {
			if strings.Contains(q, normalizeCapabilityText(alias)) {
				return item.ID
			}
		}
	}
	return ""
}

func assistantCapabilityDetail(id string) string {
	item, ok := capabilityByID(id)
	if !ok {
		return "Esta funcionalidade não está cadastrada no Jarvis."
	}
	switch item.Scope {
	case capabilityOverlay:
		if item.Example != "" {
			return fmt.Sprintf("%s: %s Comando disponível: %s.", item.Name, item.Detail, item.Example)
		}
		return item.Name + ": " + item.Detail
	case capabilityClassic:
		return item.Name + " (somente --classic): " + item.Detail
	default:
		return item.Name + " (não implementado): " + item.Detail
	}
}

func assistantCapabilitiesAnswer() string {
	var out strings.Builder
	out.WriteString("Funcionalidades reais do Jarvis Local:\n")
	for _, scope := range []capabilityScope{capabilityOverlay, capabilityClassic, capabilityAbsent} {
		switch scope {
		case capabilityOverlay:
			out.WriteString("\nPainel atual:\n")
		case capabilityClassic:
			out.WriteString("\nSomente modo --classic:\n")
		case capabilityAbsent:
			out.WriteString("\nNão implementado:\n")
		}
		for _, item := range capabilityRegistry {
			if item.Scope == scope {
				out.WriteString("• " + item.Name + ": " + item.Detail + "\n")
			}
		}
	}
	out.WriteString("\nConfirmo que um pedido foi aceito pelo Windows somente após retorno da ferramenta; isso não comprova a visibilidade da janela.")
	return out.String()
}

func buildAssistantSystemPrompt() string {
	var out strings.Builder
	out.WriteString("Você é Jarvis Local, instalado no Windows, e conversa em português brasileiro usando Qwen via Ollama. ")
	out.WriteString("Não invente ferramentas ou execuções. Respostas do modelo não executam comandos e ditado é somente rascunho. ")
	out.WriteString("Use estas capacidades, derivadas do registro do próprio aplicativo:\n")
	for _, item := range capabilityRegistry {
		out.WriteString("- " + item.Name + " [" + string(item.Scope) + "]: " + item.Detail + "\n")
	}
	out.WriteString("Só diga que um pedido foi enviado ao Windows após a função correspondente retornar sem erro. ")
	out.WriteString("Andamento refere-se a missões, não memória RAM. Atualizações são automáticas quando ocioso, ")
	out.WriteString("mas não envolvem autoedição de código. Identifique as funções exclusivas de --classic.")
	return out.String()
}

var assistantSystemPrompt = buildAssistantSystemPrompt()
