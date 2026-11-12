package main

import (
    "context"
    "encoding/json"
    "errors"
    "os"
    "path/filepath"
    "strings"
    "sync/atomic"
    "testing"
    "time"
)

func fakeAgentCall(name string, args any) agentToolCall {
    raw,_:=json.Marshal(args)
    var call agentToolCall
    call.Type="function"
    call.Function.Name=name
    call.Function.Arguments=raw
    return call
}
func TestNaturalLanguageActionDetection(t *testing.T) {
    for _,text:=range []string{
        "Por favor crie um projeto Go que conte linhas de CSV",
        "Poderia abrir a calculadora para mim?",
        "Quero que você revise a última proposta",
        "Salve um documento no workspace",
        "Me ajude a criar um arquivo",
    }{
        if !shouldAttemptAgentTools(text){t.Errorf("ação natural ignorada: %s",text)}
    }
    for _,text:=range []string{
        "Olá Jarvis", "Explique o que é Go",
        "Você consegue abrir o Windows?", "Como criar um arquivo?",
        "Por que as estrelas brilham?", "Me conte uma história",
    }{
        if shouldAttemptAgentTools(text){t.Errorf("pergunta tratada como ação: %s",text)}
    }
}
func TestAgentToolMappingsAndValidation(t *testing.T) {
    cases:=[]struct{call agentToolCall; original,kind,value string}{
        {fakeAgentCall("open_application",map[string]string{"app":"calculadora"}),"Poderia abrir a calculadora?","open","calculadora"},
        {fakeAgentCall("show_area",map[string]string{"area":"projects"}),"Abra meus projetos","projects",""},
        {fakeAgentCall("start_go_project",map[string]string{"goal":"Crie um utilitário CSV de terminal"}),"Crie um aplicativo Go","build","Crie um utilitário CSV de terminal"},
        {fakeAgentCall("start_code_improvement",map[string]string{"goal":"Melhore o fluxo de voz Piper do Jarvis"}),"Melhore o Jarvis","dev_start","Melhore o fluxo de voz Piper do Jarvis"},
        {fakeAgentCall("workspace_write",map[string]string{"file":"projeto.md","content":"# Projeto\n"}),"Escreva um arquivo de texto","workspace_write",`{"File":"projeto.md","Content":"# Projeto\\n"}`},
    }
    for _,c:=range cases {
        got,err:=decodeAgentTool(c.call,c.original)
        if err!=nil{t.Fatal(err)}
        if got.Kind!=c.kind || (c.kind!="workspace_write"&&got.Value!=c.value) {
            t.Errorf("resultado %+v, esperado %s / %q",got,c.kind,c.value)
        }
    }
    for _,tc:=range []struct{call agentToolCall; user string}{
        {fakeAgentCall("open_application",map[string]string{"app":"powershell"}),"Abra o PowerShell"},
        {fakeAgentCall("open_application",map[string]string{"app":"calc.exe","command":"cmd"}),"Abra a calculadora"},
        {fakeAgentCall("shell",map[string]string{"command":"powershell"}),"Execute comando"},
        {fakeAgentCall("workspace_read",map[string]string{"file":"../senha.txt"}),"Leia meu arquivo"},
        {fakeAgentCall("workspace_write",map[string]string{"file":"nota.md","content":"texto"}),"Explique o arquivo"},
        {fakeAgentCall("workspace_write",map[string]string{"file":"nota.md","content":"texto"}),"Crie um filme"},
    }{
        if _,err:=decodeAgentTool(tc.call,tc.user);err==nil{
            t.Errorf("ação não autorizada aceita: %+v",tc.call)
        }
    }
}
func TestAgentOnlyExecutesReportedFunctionCalls(t *testing.T) {
    user:="Poderia abrir a calculadora, por favor?"
    history:=[]dialogueLine{{Role:"user",Content:user}}
    var calls atomic.Int32
    executor:=func(_ context.Context, i desktopIntent)(string,error){
        calls.Add(1)
        if i!=(desktopIntent{Kind:"open",Value:"calculadora"}){return "",errors.New("wrong tool")}
        return "Solicitei ao Windows a abertura da Calculadora.",nil
    }
    chat:=func(_ context.Context,_ []agentMessage, _ []any)(agentMessage,error){
        return agentMessage{Role:"assistant",Content:"Eu abri a calculadora, confie em mim."},nil
    }
    response,err:=runAssistantAgentWithChat(context.Background(),history,executor,chat)
    if err!=nil || calls.Load()!=0 || !strings.Contains(response,"Nenhuma ferramenta foi executada") || strings.Contains(response,"Eu abri"){
        t.Fatalf("texto foi executado? result=%q err=%v calls=%d",response,err,calls.Load())
    }
    chat=func(_ context.Context,_ []agentMessage,tools []any)(agentMessage,error){
        if len(tools)==0 {t.Fatal("ferramentas ausentes")}
        return agentMessage{Role:"assistant",ToolCalls:[]agentToolCall{
            fakeAgentCall("open_application",map[string]string{"app":"calculadora"}),
        }},nil
    }
    response,err=runAssistantAgentWithChat(context.Background(),history,executor,chat)
    if err!=nil || calls.Load()!=1 || response!="Solicitei ao Windows a abertura da Calculadora."{
        t.Fatalf("execução não confirmada: %q %v calls %d",response,err,calls.Load())
    }
    chat=func(_ context.Context,_ []agentMessage,_ []any)(agentMessage,error){
        return agentMessage{Role:"assistant",ToolCalls:[]agentToolCall{
            fakeAgentCall("open_application",map[string]string{"app":"calculadora"}),
            fakeAgentCall("open_application",map[string]string{"app":"explorador"}),
        }},nil
    }
    _,err=runAssistantAgentWithChat(context.Background(),history,executor,chat)
    if err==nil || calls.Load()!=1 {t.Fatal("múltiplas ações executadas na mesma rodada")}
}
func TestAgentReturnsRealToolFailure(t *testing.T) {
    history:=[]dialogueLine{{Role:"user",Content:"Verifique as atualizações agora"}}
    chat:=func(_ context.Context,_ []agentMessage,_ []any)(agentMessage,error){
        return agentMessage{ToolCalls:[]agentToolCall{
            fakeAgentCall("check_official_updates",map[string]string{}),
        }},nil
    }
    result,err:=runAssistantAgentWithChat(context.Background(),history,
        func(context.Context,desktopIntent)(string,error){return "",errors.New("rede indisponível")},chat)
    if err!=nil || !strings.Contains(result,"rede indisponível") || strings.Contains(result,"concluída") {
        t.Fatalf("falha escondida: %s, %v",result,err)
    }
}
func TestAgentWorkspaceBackupsAndPathProtection(t *testing.T) {
    ctx:=context.Background()
    profile:=t.TempDir()
    if _,err:=writeWorkspace(ctx,profile,"nota.md","# primeira versão");err!=nil{t.Fatal(err)}
    if _,err:=writeWorkspace(ctx,profile,"nota.md","# segunda versão");err!=nil{t.Fatal(err)}
    text,err:=readWorkspace(ctx,profile,"nota.md")
    if err!=nil || !strings.Contains(text,"segunda"){t.Fatal(text,err)}
    lines,err:=listWorkspace(ctx,profile)
    if err!=nil || !strings.Contains(lines,"nota.md"){t.Fatal(lines,err)}
    history,err:=os.ReadDir(filepath.Join(workspaceDirectory(profile),"Historico"))
    if err!=nil || len(history)!=1 {t.Fatalf("backup ausente: %+v %v",history,err)}
    original,err:=os.ReadFile(filepath.Join(workspaceDirectory(profile),"Historico",history[0].Name()))
    if err!=nil || string(original)!="# primeira versão" {t.Fatal("backup diferente do original")}
    for _,name:=range []string{"../secrets.txt","C:\\Windows\\System32\\cmd.exe","folder/file.md",".env","app.exe","CON.txt"} {
        if _,err:=writeWorkspace(ctx,profile,name,"x");err==nil {
            t.Fatalf("nome indevido permitido: %s",name)
        }
    }
    outside:=filepath.Join(profile,"outside.txt")
    if err:=os.WriteFile(outside,[]byte("segredo"),0600);err!=nil{t.Fatal(err)}
    link:=filepath.Join(workspaceDirectory(profile),"link.txt")
    if err:=os.Symlink(outside,link);err==nil{
        if _,err:=readWorkspace(ctx,profile,"link.txt");err==nil{t.Fatal("symlink lido")}
        if _,err:=writeWorkspace(ctx,profile,"link.txt","x");err==nil{t.Fatal("symlink sobrescrito")}
    }
}
func TestAgentConversationDispatchDoesNotBreakStreaming(t *testing.T) {
    a:=newAssistant(nil,nil,"")
    a.agentReply=func(ctx context.Context,_ []dialogueLine,execute agentExecuteFunc)(string,error){
        return execute(ctx,desktopIntent{Kind:"open",Value:"calculadora"})
    }
    completed:=make(chan struct{},1)
    if err:=a.submit("Poderia abrir a calculadora?",func(i desktopIntent)(string,error){
        if i.Kind!="open" {t.Errorf("wrong %v",i)}
        completed<-struct{}{}
        return "Abertura solicitada.",nil
    });err!=nil{t.Fatal(err)}
    select {case <-completed:case <-time.After(time.Second):t.Fatal("agente não executou")}
    waitAssistant(t,a)
    if a.snapshot().Messages[len(a.snapshot().Messages)-1].Content!="Abertura solicitada." {
        t.Fatal("resultado real da ferramenta não foi mostrado")
    }
}

func TestConversationCanBeCanceledWithoutStoppingJarvis(t *testing.T) {
    a:=newAssistant(nil,nil,"")
    started:=make(chan struct{})
    a.agentReply=func(ctx context.Context,_ []dialogueLine,_ agentExecuteFunc)(string,error){
        close(started)
        <-ctx.Done()
        return "",ctx.Err()
    }
    if err:=a.submit("Poderia abrir a calculadora?",func(desktopIntent)(string,error){
        return "",errors.New("não deve executar depois de cancelar")
    });err!=nil{t.Fatal(err)}
    select{case <-started:case <-time.After(time.Second):t.Fatal("agente não iniciou")}
    if !a.cancelCurrent(){t.Fatal("não cancelou tarefa ativa")}
    waitAssistant(t,a)
    if a.cancelCurrent(){t.Fatal("aceitou cancelamento de tarefa encerrada")}
    a.agentReply=nil
    a.reply=func(context.Context,[]dialogueLine)(string,error){return "Jarvis disponível.",nil}
    if err:=a.submit("Olá novamente",nil);err!=nil{t.Fatalf("Jarvis ficou bloqueado: %v",err)}
    waitAssistant(t,a)
    if state:=a.snapshot();state.Messages[len(state.Messages)-1].Content!="Jarvis disponível."{
        t.Fatalf("o Jarvis não retomou o diálogo: %+v",state)
    }
}

func TestCancelStopsDirectSpeechAndDictation(t *testing.T) {
    voiceStarted:=make(chan struct{})
    a:=newAssistant(func(ctx context.Context,_ string,_ voiceProfile)error{
        close(voiceStarted)
        <-ctx.Done()
        return ctx.Err()
    },func(ctx context.Context)(string,error){
        <-ctx.Done()
        return "",ctx.Err()
    },"")
    if err:=a.previewVoice();err!=nil{t.Fatal(err)}
    <-voiceStarted
    if !a.cancelCurrent(){t.Fatal("não cancelou prévia de áudio")}
    waitAssistant(t,a)
    if err:=a.record();err!=nil{t.Fatal(err)}
    if !a.cancelCurrent(){t.Fatal("não cancelou ditado")}
    waitAssistant(t,a)
    if got:=a.takeDraft();got!=""{t.Fatalf("transcrição cancelada escapou: %q",got)}
}
