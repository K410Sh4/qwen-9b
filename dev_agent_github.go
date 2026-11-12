package main

import (
 "bytes"
 "context"
 "crypto/rand"
 "encoding/base64"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "os"
 "os/exec"
 "strconv"
 "regexp"
 "strings"
 "time"
)

func validDevSHA(value string) bool {
 if len(value)!=40{return false}
 for _,char:=range value {
  if (char<'a'||char>'f')&&(char<'0'||char>'9'){return false}
 }
 return true
}

// Only reads pinned public source from the project's own GitHub repository.
// No credentials, arbitrary URLs or redirects are accepted on this read path.
func devGetJSON(ctx context.Context,url string,out any) error {
 req,err:=http.NewRequestWithContext(ctx,http.MethodGet,url,nil)
 if err!=nil{return err}
 if req.URL.Scheme!="https"||req.URL.Host!="api.github.com"{
  return errors.New("a origem do código deve ser a API oficial do GitHub")
 }
 req.Header.Set("Accept","application/vnd.github+json")
 req.Header.Set("User-Agent","JarvisLocal-DevAgent")
 client:=&http.Client{Timeout:22*time.Second,
  CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse}}
 defer client.CloseIdleConnections()
 response,err:=client.Do(req)
 if err!=nil{return fmt.Errorf("GitHub indisponível: %w",err)}
 defer response.Body.Close()
 if response.StatusCode!=http.StatusOK{
  return fmt.Errorf("GitHub HTTP %d ao consultar código",response.StatusCode)
 }
 body,err:=io.ReadAll(io.LimitReader(response.Body,110<<10))
 if err!=nil{return err}
 if len(body)>=110<<10{return errors.New("resposta do GitHub excede o limite")}
 return json.Unmarshal(body,out)
}

func devMainSHA(ctx context.Context)(string,error) {
 var response struct{Object struct{SHA string}}
 url:="https://api.github.com/repos/"+devRepository+"/git/ref/heads/main"
 if err:=devGetJSON(ctx,url,&response);err!=nil{return "",err}
 if !validDevSHA(response.Object.SHA){return "",errors.New("SHA do main não validado")}
 return response.Object.SHA,nil
}

func fetchDevSource(ctx context.Context,path string)(devSource,error){
 if !devAllowedPath(path){return devSource{},errors.New("leitura fora da lista de arquivos")}
 sha,err:=devMainSHA(ctx)
 if err!=nil{return devSource{},err}
 var response struct{SHA,Encoding,Content string}
 url:="https://api.github.com/repos/"+devRepository+"/contents/"+path+"?ref="+sha
 if err:=devGetJSON(ctx,url,&response);err!=nil{return devSource{},err}
 if !validDevSHA(response.SHA)||response.Encoding!="base64"{
  return devSource{},errors.New("o GitHub não retornou o blob original")
 }
 decoded,err:=base64.StdEncoding.DecodeString(strings.ReplaceAll(response.Content,"\n",""))
 if err!=nil{return devSource{},err}
 return devSource{MainSHA:sha,BlobSHA:response.SHA,Path:path,Content:string(decoded)},nil
}

var devVersionLine=regexp.MustCompile(`(?m)^const Version = "v([0-9]+)\.([0-9]+)\.([0-9]+)"package main

import (
 "bytes"
 "context"
 "crypto/rand"
 "encoding/base64"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "os"
 "os/exec"
 "strconv"
 "regexp"
 "strings"
 "time"
)

func validDevSHA(value string) bool {
 if len(value)!=40{return false}
 for _,char:=range value {
  if (char<'a'||char>'f')&&(char<'0'||char>'9'){return false}
 }
 return true
}

// Only reads pinned public source from the project's own GitHub repository.
// No credentials, arbitrary URLs or redirects are accepted on this read path.
func devGetJSON(ctx context.Context,url string,out any) error {
 req,err:=http.NewRequestWithContext(ctx,http.MethodGet,url,nil)
 if err!=nil{return err}
 if req.URL.Scheme!="https"||req.URL.Host!="api.github.com"{
  return errors.New("a origem do código deve ser a API oficial do GitHub")
 }
 req.Header.Set("Accept","application/vnd.github+json")
 req.Header.Set("User-Agent","JarvisLocal-DevAgent")
 client:=&http.Client{Timeout:22*time.Second,
  CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse}}
 defer client.CloseIdleConnections()
 response,err:=client.Do(req)
 if err!=nil{return fmt.Errorf("GitHub indisponível: %w",err)}
 defer response.Body.Close()
 if response.StatusCode!=http.StatusOK{
  return fmt.Errorf("GitHub HTTP %d ao consultar código",response.StatusCode)
 }
 body,err:=io.ReadAll(io.LimitReader(response.Body,110<<10))
 if err!=nil{return err}
 if len(body)>=110<<10{return errors.New("resposta do GitHub excede o limite")}
 return json.Unmarshal(body,out)
}

func devMainSHA(ctx context.Context)(string,error) {
 var response struct{Object struct{SHA string}}
 url:="https://api.github.com/repos/"+devRepository+"/git/ref/heads/main"
 if err:=devGetJSON(ctx,url,&response);err!=nil{return "",err}
 if !validDevSHA(response.Object.SHA){return "",errors.New("SHA do main não validado")}
 return response.Object.SHA,nil
}

func fetchDevSource(ctx context.Context,path string)(devSource,error){
 if !devAllowedPath(path){return devSource{},errors.New("leitura fora da lista de arquivos")}
 sha,err:=devMainSHA(ctx)
 if err!=nil{return devSource{},err}
 var response struct{SHA,Encoding,Content string}
 url:="https://api.github.com/repos/"+devRepository+"/contents/"+path+"?ref="+sha
 if err:=devGetJSON(ctx,url,&response);err!=nil{return devSource{},err}
 if !validDevSHA(response.SHA)||response.Encoding!="base64"{
  return devSource{},errors.New("o GitHub não retornou o blob original")
 }
 decoded,err:=base64.StdEncoding.DecodeString(strings.ReplaceAll(response.Content,"\n",""))
 if err!=nil{return devSource{},err}
 return devSource{MainSHA:sha,BlobSHA:response.SHA,Path:path,Content:string(decoded)},nil
}

)

func devNextVersion(source string) (string,string,error) {
 match:=devVersionLine.FindStringSubmatch(source)
 if len(match)!=4{return "","",errors.New("versão OTA sem formato semântico")}
 major,e1:=strconv.Atoi(match[1])
 minor,e2:=strconv.Atoi(match[2])
 patch,e3:=strconv.Atoi(match[3])
 if e1!=nil||e2!=nil||e3!=nil||patch>=999999{
  return "","",errors.New("número da versão fora dos limites")
 }
 tag:=fmt.Sprintf("v%d.%d.%d",major,minor,patch+1)
 changed:=strings.Replace(source,match[0],"const Version = "+strconv.Quote(tag),1)
 if changed==source{return "","",errors.New("a versão não foi alterada")}
 return changed,tag,nil
}

// Bump OTA only in the draft branch, not in the currently running executable.
func fetchDevNextVersion(ctx context.Context,mainSHA string)(string,string,string,error) {
 if !validDevSHA(mainSHA){return "","","",errors.New("SHA principal inválido")}
 var file struct{SHA,Encoding,Content string}
 url:="https://api.github.com/repos/"+devRepository+"/contents/ota.go?ref="+mainSHA
 if err:=devGetJSON(ctx,url,&file);err!=nil{return "","","",err}
 if !validDevSHA(file.SHA)||file.Encoding!="base64"{
  return "","","",errors.New("arquivo de versão oficial inválido")
 }
 raw,err:=base64.StdEncoding.DecodeString(strings.ReplaceAll(file.Content,"\n",""))
 if err!=nil{return "","","",err}
 changed,tag,err:=devNextVersion(string(raw))
 return file.SHA,changed,tag,err
}

// Publishing is a separate user-authorized action. GitHub CLI holds its own
// authentication; Jarvis never asks for PATs or saves tokens into its profile.
// No generated code is executed, merged or installed by this function.
func publishDevPullRequest(ctx context.Context,p devProposal)(string,error) {
 if err:=devValidateStored(p);err!=nil{return "",err}
 head,err:=devMainSHA(ctx)
 if err!=nil{return "",err}
 if head!=p.MainSHA{
  return "",errors.New("o main mudou desde a auditoria; peça uma proposta nova")
 }
 gh,err:=exec.LookPath("gh")
 if err!=nil{
  return "",errors.New("GitHub CLI ausente: instale com 'winget install GitHub.cli' e execute 'gh auth login' no Windows")
 }
 var nonce [4]byte
 if _,err:=rand.Read(nonce[:]);err!=nil{return "",err}
 branch:="jarvis-dev/"+p.MainSHA[:9]+"-"+hex.EncodeToString(nonce[:])
 call:=func(method,endpoint string,input any,result any)error{
  body,err:=json.Marshal(input)
  if err!=nil{return err}
  command:=exec.CommandContext(ctx,gh,"api","-X",method,endpoint,"--input","-")
  command.Stdin=bytes.NewReader(body)
  command.Env=append(os.Environ(),"GH_PROMPT_DISABLED=1","GIT_TERMINAL_PROMPT=0")
  var stdout,stderr bytes.Buffer
  command.Stdout=&stdout;command.Stderr=&stderr
  if err:=command.Run();err!=nil{
   return fmt.Errorf("GitHub CLI: %s: %w",shortened(stderr.String(),220),err)
  }
  if result!=nil {
   if err:=json.Unmarshal(stdout.Bytes(),result);err!=nil{
    return fmt.Errorf("resposta do GitHub CLI inválida: %w",err)
   }
  }
  return nil
 }
 if err:=call("POST","repos/"+devRepository+"/git/refs",map[string]any{
  "ref":"refs/heads/"+branch,"sha":p.MainSHA,
 },nil);err!=nil{return "",fmt.Errorf("criação de branch: %w",err)}
 if err:=call("PUT","repos/"+devRepository+"/contents/"+p.Path,map[string]any{
  "message":"refactor(dev-agent): "+shortened(p.Goal,78),
  "branch":branch,"sha":p.BlobSHA,
  "content":base64.StdEncoding.EncodeToString([]byte(p.Candidate)),
 },nil);err!=nil{
  return "",fmt.Errorf("gravar patch na branch %s: %w",branch,err)
 }
 // An approved PR must increase the semver tag or the existing OTA would
 // ignore the new executable. The model cannot choose or edit this version.
 versionSHA,nextVersion,tag,err:=fetchDevNextVersion(ctx,p.MainSHA)
 if err!=nil{return "",fmt.Errorf("preparar versão da release: %w",err)}
 if err:=call("PUT","repos/"+devRepository+"/contents/ota.go",map[string]any{
  "message":"chore(release): bump version to "+tag,
  "branch":branch,"sha":versionSHA,
  "content":base64.StdEncoding.EncodeToString([]byte(nextVersion)),
 },nil);err!=nil{
  return "",fmt.Errorf("gravar versão na branch %s: %w",branch,err)
 }
 var pr struct{HTMLURL string `json:"html_url"`}
 if err:=call("POST","repos/"+devRepository+"/pulls",map[string]any{
  "head":branch,"base":"main","draft":true,
  "title":"Jarvis Dev Agent: "+shortened(p.Goal,100),
  "body":"## Patch gerado localmente — revisão obrigatória\n\nObjetivo: "+shortened(p.Goal,450)+
   "\n\nArquivo: "+p.Path+
   "\n\nHipótese do modelo: "+shortened(p.Reason,450)+
   "\n\nTestes sugeridos (não executados localmente): "+shortened(p.TestPlan,450)+
   "\n\nInclui também aumento automático da versão em ota.go para permitir uma release após merge aprovado. "+
   "O diff passou por limites de tamanho e verificação sintática. "+
   "A CI Windows fará testes de comportamento. "+
   "Não aprove sem revisar a segurança do patch.",
 },&pr);err!=nil{
  return "",fmt.Errorf("criar PR draft a partir de %s: %w",branch,err)
 }
 const prefix="https://github.com/K410Sh4/qwen-9b/pull/"
 if !strings.HasPrefix(pr.HTMLURL,prefix){
  return "",errors.New("link do Pull Request inesperado")
 }
 number,err:=strconv.Atoi(strings.TrimPrefix(pr.HTMLURL,prefix))
 if err!=nil||number<1{return "",errors.New("número do Pull Request inválido")}
 return pr.HTMLURL,nil
}
