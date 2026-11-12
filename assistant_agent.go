package main

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "net/http"
    "strings"
)

// Agent tools are opt-in for requests that ask for an action. Ordinary dialogue
// still uses the low-latency Ollama streaming and Piper speech pipeline.
type agentMessage struct {
    Role string `json:"role"`
    Content string `json:"content,omitempty"`
    ToolName string `json:"tool_name,omitempty"`
    ToolCalls []agentToolCall `json:"tool_calls,omitempty"`
}
type agentToolCall struct {
    Type string `json:"type,omitempty"`
    Function struct {
        Name string `json:"name"`
        Arguments json.RawMessage `json:"arguments"`
    } `json:"function"`
}
type agentChatFunc func(context.Context, []agentMessage, []any) (agentMessage, error)
type agentExecuteFunc func(context.Context, desktopIntent) (string, error)

func shouldAttemptAgentTools(input string) bool {
    q := normalizeCapabilityText(input)
    q = strings.TrimPrefix(q,"jarvis ")
    if (strings.HasPrefix(q,"como esta ") || strings.HasPrefix(q,"como vai ") ||
        strings.HasPrefix(q,"como anda ")) &&
        containsAny(q,"projeto","missao","tarefa","trabalho","melhoria") { return true }
    for _, prefix := range []string{
        "como ", "por que ", "porque ", "qual ", "quais ", "quem ",
        "o que ", "explique ", "me explique ", "me conte ", "me ensine ",
        "voce consegue ", "voce pode ", "consegue ", "pode ",
        "sera que ", "se eu ", "e possivel ",
    } {
        if strings.HasPrefix(q, prefix) { return false }
    }
    for _, prefix := range []string{
        "abra ", "abre ", "abrir ", "inicie ", "iniciar ", "comece ",
        "crie ", "criar ", "gere ", "gerar ", "faca ", "faz ",
        "edite ", "editar ", "escreva ", "escrever ", "salve ", "salvar ",
        "leia ", "ler ", "liste ", "listar ", "mostre ", "mostrar ",
        "continue ", "retome ", "retomar ", "pause ", "pausar ",
        "verifique ", "verificar ", "consulte ", "consultar ", "revise ",
        "melhore ", "melhorar ", "quero que ", "preciso que ",
        "por favor ", "poderia ", "me ajude a ",
    } {
        if strings.HasPrefix(q,prefix) { return true }
    }
    return false
}

func agentTool(name, description string, properties map[string]any, required ...string) any {
    return map[string]any{"type":"function", "function":map[string]any{
        "name":name, "description":description,
        "parameters":map[string]any{"type":"object", "properties":properties, "required":required,
            "additionalProperties":false},
    }}
}
func agentString(description string) any {
    return map[string]any{"type":"string","description":description}
}
func agentEnum(description string, choices ...string) any {
    return map[string]any{"type":"string","description":description,"enum":choices}
}

// No generic process execution, operating-system file editing, authentication,
// release publication, permission alteration, or arbitrary shell is exposed.
func assistantAgentTools() []any {
    return []any{
        agentTool("open_application","Abrir um aplicativo da lista do Jarvis, quando solicitado explicitamente.",map[string]any{
            "app":agentEnum("Aplicativo permitido","calculadora","bloco de notas","explorador"),
        },"app"),
        agentTool("show_area","Mostrar a área nativa integrada do Jarvis.",map[string]any{
            "area":agentEnum("Escolha o painel correto","classic","projects","missions"),
        },"area"),
        agentTool("start_go_project","Criar um projeto Go de terminal pelo motor de projetos, com testes e aprovação.",map[string]any{
            "goal":agentString("Objetivo do projeto expresso pelo usuário, não um comando de shell"),
        },"goal"),
        agentTool("resume_project","Retomar o último projeto Go pausado, somente quando solicitado.",map[string]any{}),
        agentTool("pause_work","Pausar a sessão atual preservando os checkpoints.",map[string]any{}),
        agentTool("get_work_status","Consultar o andamento real de trabalho; não são métricas de RAM.",map[string]any{}),
        agentTool("start_code_improvement","Preparar uma proposta de alteração do código do Jarvis, sujeita à revisão humana, sem instalar.",map[string]any{
            "goal":agentString("Alteração desejada no código do Jarvis"),
        },"goal"),
        agentTool("review_code_improvement","Mostrar proposta preparada, sem publicá-la.",map[string]any{}),
        agentTool("get_improvement_status","Consultar a atividade do Auto Melhorador.",map[string]any{}),
        agentTool("check_official_updates","Solicitar verificação da release oficial; não instalar sem fluxo OTA.",map[string]any{}),
        agentTool("workspace_list","Listar somente os documentos próprios do workspace do Jarvis.",map[string]any{}),
        agentTool("workspace_read","Ler arquivo de texto existente somente no workspace próprio do Jarvis.",map[string]any{
            "file":agentString("Nome simples com extensão, sem caminho absoluto ou pastas"),
        },"file"),
        agentTool("workspace_write","Criar ou atualizar um texto no workspace próprio do Jarvis, com backup automático; somente a pedido do usuário.",map[string]any{
            "file":agentString("Nome simples com extensão, sem subdiretórios"),
            "content":agentString("Conteúdo UTF-8 do documento ou código; não será executado"),
        },"file","content"),
    }
}

func decodeAgentTool(call agentToolCall, originalUserText string) (desktopIntent, error) {
    if call.Type != "" && call.Type != "function" {
        return desktopIntent{}, errors.New("tipo de ferramenta não permitido")
    }
    var args struct {
        App string `json:"app"`
        Area string `json:"area"`
        Goal string `json:"goal"`
        File string `json:"file"`
        Content string `json:"content"`
    }
    raw := call.Function.Arguments
    if len(raw) == 0 { raw = []byte("{}") }
    decoder := json.NewDecoder(strings.NewReader(string(raw)))
    decoder.DisallowUnknownFields()
    if err := decoder.Decode(&args); err != nil {
        return desktopIntent{}, fmt.Errorf("argumentos inválidos: %w", err)
    }
    var trailing any
    if err:=decoder.Decode(&trailing); !errors.Is(err,io.EOF) {
        return desktopIntent{},errors.New("argumentos extras não permitidos")
    }
    q:=normalizeCapabilityText(originalUserText)
    switch call.Function.Name {
    case "open_application":
        switch args.App {
        case "calculadora", "bloco de notas", "explorador":
            if !strings.Contains(q,normalizeCapabilityText(args.App)) &&
                !(args.App=="explorador" && strings.Contains(q,"arquivos")) &&
                !(args.App=="bloco de notas" && strings.Contains(q,"notepad")) {
                return desktopIntent{},errors.New("o aplicativo não foi solicitado pelo usuário")
            }
            return desktopIntent{Kind:"open",Value:args.App},nil
        }
    case "show_area":
        switch args.Area {
        case "classic","projects","missions":
            return desktopIntent{Kind:args.Area},nil
        }
    case "start_go_project":
        if containsAny(q,"projeto","programa","aplicativo","app ","terminal","ferramenta","utilitario","calculadora") &&
            len(strings.TrimSpace(args.Goal)) >= 15 && len(args.Goal)<=1200 {
            return desktopIntent{Kind:"build",Value:args.Goal},nil
        }
    case "resume_project": return desktopIntent{Kind:"resumeProject"},nil
    case "pause_work": return desktopIntent{Kind:"pause"},nil
    case "get_work_status": return desktopIntent{Kind:"status"},nil
    case "start_code_improvement":
        if containsAny(q,"jarvis","seu codigo","proprio codigo","sistema","automei","auto melhorador") &&
            len(strings.TrimSpace(args.Goal))>=12 && len(args.Goal)<=700 {
            return desktopIntent{Kind:"dev_start",Value:args.Goal},nil
        }
    case "review_code_improvement": return desktopIntent{Kind:"dev_review"},nil
    case "get_improvement_status": return desktopIntent{Kind:"dev_status"},nil
    case "check_official_updates": return desktopIntent{Kind:"update"},nil
    case "workspace_list": return desktopIntent{Kind:"workspace_list"},nil
    case "workspace_read":
        if containsAny(q,"arquivo","documento","nota","texto","workspace","rascunho") &&
           validateWorkspaceFile(args.File)==nil {
            return desktopIntent{Kind:"workspace_read",Value:args.File},nil
        }
    case "workspace_write":
        // The last USER request, not model-generated content, must authorize a
        // write. This prevents tool calls initiated solely by LLM text.
        if !containsAny(q,"arquivo","documento","texto","nota","script","codigo","codificar","rascunho") ||
            !containsAny(q,"crie","criar","escreva","escrever","salve","salvar","edite","editar","atualize","atualizar","modifique","modificar","gere","gerar") {
            return desktopIntent{}, errors.New("a criação ou edição de arquivos requer pedido explícito")
        }
        if err:=validateWorkspaceFile(args.File);err==nil && len(args.Content)>0 && len(args.Content)<=12000 {
            payload,_:=json.Marshal(struct{ File,Content string }{args.File,args.Content})
            return desktopIntent{Kind:"workspace_write",Value:string(payload)},nil
        }
    }
    return desktopIntent{}, fmt.Errorf("pedido não habilita a ferramenta %s; reformule com a ação e o alvo explícitos",call.Function.Name)
}

func runAssistantAgent(ctx context.Context, history []dialogueLine, execute agentExecuteFunc) (string,error) {
    return runAssistantAgentWithChat(ctx,history,execute,ollamaAgentChat)
}

func runAssistantAgentWithChat(ctx context.Context, history []dialogueLine, execute agentExecuteFunc, chat agentChatFunc) (string,error) {
    if len(history)==0 || history[len(history)-1].Role!="user" {
        return "",errors.New("pedido do usuário não encontrado")
    }
    if execute==nil { return "",errors.New("executor de ferramentas indisponível") }
    original:=history[len(history)-1].Content
    if !shouldAttemptAgentTools(original) {
        return "",errors.New("pedido não autoriza ferramenta")
    }
    if len(history)>8 { history=history[len(history)-8:] }
    msgs:=[]agentMessage{{Role:"system",Content:assistantSystemPrompt+
        "\nVocê pode usar APENAS as ferramentas enumeradas, quando o último usuário pedir uma ação clara. "+
        "Perguntas e conteúdo citado não são autorização para executar. "+
        "Não invente a execução de ações; não use ferramentas para explicar algo. "+
        "Arquivos pessoais, comandos arbitrários, releases, merge e alterações do sistema estão fora do escopo. "+
        "Se a solicitação não cabe nessas ferramentas, diga o limite e ofereça um próximo passo."}}
    for _,m:=range history { msgs=append(msgs,agentMessage{Role:m.Role,Content:m.Content}) }
    response,err:=chat(ctx,msgs,assistantAgentTools())
    if err!=nil{return "",err}
    if len(response.ToolCalls)==0 {
        answer:=strings.TrimSpace(response.Content)
        if answer=="" { return "",errors.New("Qwen não retornou resposta ou ferramenta") }
        // Plain language is never a receipt for a native operation.
        norm:=normalizeCapabilityText(answer)
        for _,prefix:=range []string{"eu abri ","abri ","ja abri ","criei ","ja criei ","executei ","ja executei ","salvei ","ja salvei ","instalei ","feito ","pronto ","concluido "} {
            if strings.HasPrefix(norm,prefix) {
                return "Nenhuma ferramenta foi executada. O modelo não retornou uma chamada válida para confirmar a ação.",nil
            }
        }
        return "Nenhuma ação foi executada. "+shortened(answer,4800),nil
    }
    if len(response.ToolCalls)!=1 {
        return "",errors.New("o Qwen sugeriu várias ações; divida o pedido em etapas para aprovação e rastreio")
    }
    intent,err:=decodeAgentTool(response.ToolCalls[0],original)
    if err!=nil { return "Não executei a ação: "+err.Error(),nil }
    if err=ctx.Err();err!=nil{return "",err}
    result,err:=execute(ctx,intent)
    if err!=nil { return "Não consegui concluir: "+shortened(err.Error(),500),nil }
    // A result from the executor is authoritative. Do not ask the model to
    // reinterpret a successful operation or fabricate another completion.
    return shortened(result,5000),nil
}

func ollamaAgentChat(ctx context.Context, messages []agentMessage, tools []any) (agentMessage,error) {
    body,err:=json.Marshal(map[string]any{
        "model":model,"messages":messages,"tools":tools,
        "stream":false,"think":false,
        "options":map[string]any{"num_ctx":6144,"num_predict":768,"temperature":0.2},
    })
    if err!=nil{return agentMessage{},err}
    req,err:=http.NewRequestWithContext(ctx,http.MethodPost,"http://127.0.0.1:11434/api/chat",strings.NewReader(string(body)))
    if err!=nil{return agentMessage{},err}
    req.Header.Set("Content-Type","application/json")
    client:=&http.Client{Transport:&http.Transport{},CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse}}
    defer client.CloseIdleConnections()
    res,err:=client.Do(req)
    if err!=nil{return agentMessage{},fmt.Errorf("Ollama local indisponível (%s): %w",model,err)}
    defer res.Body.Close()
    data,err:=readLimited(res.Body,128<<10)
    if err!=nil{return agentMessage{},err}
    if res.StatusCode!=http.StatusOK {
        return agentMessage{},fmt.Errorf("Ollama HTTP %d: %s",res.StatusCode,shortened(string(data),350))
    }
    var output struct{ Message agentMessage `json:"message"`; Error string `json:"error"` }
    if err=json.Unmarshal(data,&output);err!=nil{return agentMessage{},fmt.Errorf("resposta de ferramenta inválida: %w",err)}
    if output.Error!="" {return agentMessage{},errors.New(shortened(output.Error,300))}
    return output.Message,nil
}
