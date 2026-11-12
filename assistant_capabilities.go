package main

import "strings"

// This list describes the application's ACTUAL integrations, not the base LLM.
// Keep it aligned with assistantIntent, assistantShell.runIntent, and the
// restricted projects/missions engine when adding capabilities.
const assistantSystemPrompt =
    "Você é o Jarvis Local, instalado no Windows do usuário, executando o Qwen via Ollama. " +
    "Responda em português brasileiro, diretamente e com precisão. " +
    "Você NÃO é um chatbot genérico sem integrações: o aplicativo executa ações específicas, " +
    "mas você não pode inventar que abriu um programa, pesquisou a web ou compilou arquivos. " +
    "Capacidades do painel atual: conversar; receber ditado quando houver reconhecimento de fala no Windows; " +
    "falar por voz Piper Faber; verificar atualizações oficiais do GitHub; " +
    "abrir por comando explícito apenas Calculadora, Bloco de Notas e Explorador de Arquivos; " +
    "consultar andamento da área disponível. " +
    "Abertura de programas por nome aproximado, Steam e programas arbitrários NÃO está implementada. " +
    "Internet: a aplicação pode baixar releases oficiais e consultar fontes públicas RSS/Atom previamente configuradas no módulo legado; " +
    "não há ferramenta de busca ou navegador web geral acessível por este chat. " +
    "Projetos: o modo legado --classic pode criar/editar arquivos de projetos limitados em Go para terminal, " +
    "executar testes e compilar programa.exe, sujeito à política de execução e aprovação; " +
    "o painel flutuante atual não inicia novos projetos, nem gera executáveis de qualquer tipo sob demanda. " +
    "Você não modifica seu próprio código-fonte ou executável autonomamente; novas versões são publicadas externamente no GitHub " +
    "e o atualizador instalado verifica a integridade antes de aplicar. " +
    "Se o usuário perguntar sobre ferramentas, diferencie implementado, restrito ao modo clássico e ainda não implementado. " +
    "Só confirme sucesso de ações depois do retorno da ferramenta. " +
    "Comandos explícitos disponíveis incluem Abra a calculadora, Abra o bloco de notas, " +
    "Mostre o andamento e Verifique atualizações."

// Capability questions are answered from the actual application contract,
// avoiding generic LLM refusals and fabricated actions.
func assistantCapabilitiesQuestion(input string) bool {
    q := desktopAccent.Replace(strings.ToLower(strings.TrimSpace(input)))
    for _, question := range []string{
        "quais sao suas capacidades",
        "quais as suas capacidades",
        "o que voce consegue fazer",
        "o que voce pode fazer",
        "quais funcionalidades voce tem",
        "voce tem acesso ao meu computador",
    } {
        if strings.Contains(q, question) {
            return true
        }
    }
    if !(strings.HasPrefix(q, "voce consegue") || strings.HasPrefix(q, "jarvis consegue") ||
        strings.HasPrefix(q, "consegue acessar")) {
        return false
    }
    topics := 0
    for _, term := range []string{"internet", "apps", "aplicativ", "programa", "executavel", "codigo", "computador", "pc"} {
        if strings.Contains(q, term) {
            topics++
        }
    }
    return topics >= 2
}

func assistantCapabilitiesAnswer() string {
    return "Tenho algumas integrações reais no Windows, mas não acesso irrestrito ao PC.\n\n" +
        "• Aplicativos: posso abrir Calculadora, Bloco de Notas e Explorador por comando explícito. " +
        "Ainda não identifico nomes aproximados nem abro Steam ou outros programas arbitrários.\n" +
        "• Internet: posso baixar atualizações oficiais do GitHub. O módulo clássico também usa fontes públicas RSS/Atom configuradas; " +
        "não navego ou pesquiso sites livremente pelo chat.\n" +
        "• Programas e executáveis: no modo --classic existe um fluxo para criar e editar projetos Go de terminal, " +
        "testá-los e compilar um .exe. O painel flutuante não cria novos projetos, e não compilo qualquer tipo de aplicativo.\n" +
        "• Melhorias no próprio Jarvis: não modifico meu código sozinho. Novas versões precisam ser publicadas no GitHub; " +
        "meu atualizador verifica a integridade antes da instalação.\n\n" +
        "Para testar uma ação disponível, diga 'Abra a calculadora'. " +
        "Para usar os projetos anteriores, inicie o Jarvis no modo --classic."
}
