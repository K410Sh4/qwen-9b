package main

import (
 "context"
 "errors"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "sync/atomic"
 "testing"
 "time"
)

const devFixture = "package main\n\nfunc devExample() int {\n\treturn 2\n}\n"
var devTestPatch = devPatch{
 Before:"\treturn 2\n}",
 After:"\treturn 3\n}",
 Reason:"Corrige um resultado demonstrável sem adicionar permissões.",
 TestPlan:"go test ./... e go vet ./... no GitHub Actions.",
}

func TestDevAgentIntentAndAllowedFiles(t *testing.T) {
 cases:=[]struct{input,kind,value string}{
  {"Melhore seu código: reduza o tempo de resposta do Qwen","dev_start","reduza o tempo de resposta do Qwen"},
  {"Melhore o Jarvis: melhore a expressão facial","dev_start","melhore a expressão facial"},
  {"/melhorar reduza a latência de voz","dev_start","reduza a latência de voz"},
  {"Status da melhoria","dev_status",""},
  {"Ver proposta da melhoria","dev_review",""},
  {"/melhoria publicar","dev_publish",""},
  {"Abrir pasta da melhoria","dev_folder",""},
  {"Como posso melhorar código?","",""},
  {"Você consegue modificar seu código?","capability_detail","self-modify"},
 }
 for _,tc:=range cases{
  actual:=assistantIntent(tc.input)
  if actual.Kind!=tc.kind||actual.Value!=tc.value{
   t.Errorf("%q: %+v; wanted %s/%s",tc.input,actual,tc.kind,tc.value)
  }
 }
 for _,unsafe:=range []string{".github/workflows/windows.yml","ota.go","go.mod","dev_agent.go","../ota.go","README.md","script.ps1"}{
  if devAllowedPath(unsafe){t.Errorf("arquivo de alto risco autorizado: %q",unsafe)}
 }
}

func TestDevPatchStaticSafetyAndSyntax(t *testing.T) {
 next,err:=devApplyPatch("assistant_stream.go",devFixture,devTestPatch)
 if err!=nil{t.Fatal(err)}
 if !strings.Contains(next,"return 3")||strings.Contains(next,"return 2"){
  t.Fatalf("patch não aplicado: %s",next)
 }
 if _,err:=devApplyPatch("ota.go",devFixture,devTestPatch);err==nil{
  t.Fatal("arquivo sensível autorizado")
 }
 duplicate:=strings.ReplaceAll(devFixture,"\treturn 2", "\treturn 2")+
  "\nfunc more() int { return 2 }\n"
 // Exact replacement remains unique even with other similar text.
 if _,err:=devApplyPatch("assistant.go",duplicate,devTestPatch);err!=nil{t.Fatal(err)}
 for name,p:=range map[string]devPatch{
  "no change":{Before:devTestPatch.Before,After:devTestPatch.Before},
  "missing":{Before:"\treturn 27\n}",After:devTestPatch.After},
  "bad syntax":{Before:devTestPatch.Before,After:"\treturn (((\n}"},
  "new command":{Before:devTestPatch.Before,After:"\treturn 3\n}\n// exec.Command("},
  "new init":{Before:devTestPatch.Before,After:"\treturn 3\n}\nfunc init() {}"},
  "too large":{Before:devTestPatch.Before,After:strings.Repeat("z",3100)},
 }{
  if _,err:=devApplyPatch("assistant_stream.go",devFixture,p);err==nil{
   t.Errorf("%s: proposta inválida foi aceita",name)
  }
 }
 if _,err:=devApplyPatch("assistant.go",devFixture+devFixture,devTestPatch);err==nil{
  t.Fatal("trecho duplicado foi aceito")
 }
 if _,err:=fetchDevSource(context.Background(),"ota.go");err==nil{
  t.Fatal("leitura fora da lista de arquivos aceita")
 }
}

func TestDevAgentPreparesAndPersistsProposal(t *testing.T){
 ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 root:=t.TempDir()
 a:=newDevAgent(ctx,root)
 const sha1="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
 const sha2="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
 a.generate=func(_ context.Context,_ string,_ any,out any)error{
  switch result:=out.(type){
  case *devPlan: *result=devPlan{Path:"assistant_stream.go",Reason:"Testar código de streaming"}
  case *devPatch: *result=devTestPatch
  default: return errors.New("saída não reconhecida")
  }
  return nil
 }
 a.fetch=func(_ context.Context,path string)(devSource,error){
  if path!="assistant_stream.go"{return devSource{},errors.New("arquivo errado")}
  return devSource{MainSHA:sha1,BlobSHA:sha2,Path:path,Content:devFixture},nil
 }
 result:=make(chan string,1)
 msg,err:=a.start("corrigir cálculo numa função de exemplo",func(v string){result<-v})
 if err!=nil||!strings.Contains(msg,"Iniciei"){t.Fatalf("não iniciou: %q: %v",msg,err)}
 select{
 case output:=<-result:
  if !strings.Contains(output,"proposta preparada"){t.Fatalf("mensagem de resultado: %s",output)}
 case <-time.After(3*time.Second):t.Fatal("auto melhorador não retornou")
 }
 review,err:=a.reviewText()
 if err!=nil||!strings.Contains(review,"ANTES:")||!strings.Contains(review,"DEPOIS:"){
  t.Fatalf("revisão não disponível: %q, %v",review,err)
 }
 saved,err:=os.ReadFile(filepath.Join(root,"proposta.json"))
 if err!=nil||!strings.Contains(string(saved),"assistant_stream.go"){t.Fatalf("cópia local não salva: %v",err)}
 loaded:=newDevAgent(ctx,root)
 if _,err:=loaded.reviewText();err!=nil{t.Fatalf("proposta não persistiu: %v",err)}
}

func TestDevAgentPublishRequiresSeparateApproval(t *testing.T) {
 ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 a:=newDevAgent(ctx,t.TempDir())
 if _,err:=a.publishForReview(func(string){});err==nil{
  t.Fatal("publicou sem proposta")
 }
 var calls atomic.Int32
 a.publish=func(_ context.Context,p devProposal)(string,error){
  calls.Add(1)
  if p.Path!="assistant_stream.go"{return "",errors.New("arquivo incorreto")}
  return "https://github.com/K410Sh4/qwen-9b/pull/101",nil
 }
 a.mu.Lock()
 a.proposal=&devProposal{
  Goal:"corrigir cálculo numa função de exemplo",Path:"assistant_stream.go",
  MainSHA:"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",BlobSHA:"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
  Original:devFixture,Candidate:strings.Replace(devFixture,devTestPatch.Before,devTestPatch.After,1),
  Before:devTestPatch.Before,After:devTestPatch.After,
 }
 a.mu.Unlock()
 if calls.Load()!=0{t.Fatal("publicação sem aprovação")}
 done:=make(chan string,1)
 _,err:=a.publishForReview(func(msg string){done<-msg})
 if err!=nil{t.Fatal(err)}
 select{
 case msg:=<-done:
  if !strings.Contains(msg,"Pull Request draft"){t.Fatalf("resultado: %q",msg)}
 case <-time.After(3*time.Second):t.Fatal("publicação autorizada não terminou")
 }
 if calls.Load()!=1{t.Fatal("publicação repetida ou ausente")}
 if _,err:=a.publishForReview(func(string){});err!=nil{t.Fatal(err)}
 if calls.Load()!=1{t.Fatal("o mesmo patch foi enviado mais de uma vez")}
}

func TestDevSyntaxChecksOnlyNeverExecutesJS(t *testing.T) {
 if _,err:=exec.LookPath("node");err!=nil{t.Skip("Node.js não instalado neste ambiente")}
 if err:=devCheckJavaScript("const answer = 42;");err!=nil{t.Fatal(err)}
 if err:=devCheckJavaScript("const = ;");err==nil{t.Fatal("parser não detectou sintaxe inválida")}
}

func TestDevGithubSHAValidation(t *testing.T){
 for _,sha:=range []string{"","ghijkl","aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaG",strings.Repeat("a",39)}{
  if validDevSHA(sha){t.Errorf("SHA inválido aceito: %q",sha)}
 }
 if !validDevSHA(strings.Repeat("a",40)){t.Fatal("SHA válido rejeitado")}
}
