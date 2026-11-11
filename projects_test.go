package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var demoProjectPlan = ProjectPlan{
	Title: "Saudação", Summary: "Escreve uma saudação para o nome recebido como argumento.",
	Cases: []ProjectCase{
		{Name: "Primeira entrada", Args: []string{"Ana"}, Stdout: "Olá, Ana"},
		{Name: "Segunda entrada", Args: []string{"Bia"}, Stdout: "Olá, Bia"},
	},
}

var demoProjectFiles = []ProjectFile{
	{Path: "main.go", Source: `package main
import ("fmt"; "os")
func main() { fmt.Println(greet(os.Args[1])) }`},
	{Path: "logic.go", Source: `package main
func greet(name string) string { return "Olá, " + name }`},
	{Path: "logic_test.go", Source: `package main
import "testing"
func TestGreet(t *testing.T) {
 if got := greet("Jarvis"); got != "Olá, Jarvis" { t.Fatal(got) }
}`},
}

func TestProjectRejectsFileEscapesAndPlatformFiles(t *testing.T) {
	if err := checkProjectFiles(demoProjectFiles); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../main.go", "helper_windows.go", "con.go", "helper_linux_test.go"} {
		files := append(append([]ProjectFile{}, demoProjectFiles...), ProjectFile{Path: path, Source: "package main"})
		if err := checkProjectFiles(files); err == nil {
			t.Fatalf("arquivo aceito: %s", path)
		}
	}
	for _, code := range []string{
		"package main\nimport \"os/exec\"",
		"package main\n//go:wasmimport host method",
		"package main\nimport \"os\"\nvar _ = os.ReadFile",
	} {
		files := append(append([]ProjectFile{}, demoProjectFiles...), ProjectFile{Path: "extra.go", Source: code})
		if err := checkProjectFiles(files); err == nil {
			t.Fatalf("código aceito: %s", code)
		}
	}
}

func TestProjectRepairsRealCompilerFailure(t *testing.T) {
	e := fakeEngine(t)
	e.build = func(ctx context.Context, dir string, files []ProjectFile, plan ProjectPlan) ([]string, string, error) {
		return compileProject(ctx, runtime.GOROOT(), dir, files, plan)
	}
	repaired := false
	e.ask = func(_ context.Context, _ string, input, out any) error {
		switch v := out.(type) {
		case *ProjectPlan:
			*v = demoProjectPlan
		case *ProjectSource:
			data := input.(map[string]any)
			for _, file := range demoProjectFiles {
				if file.Path != data["arquivo_pedido"] {
					continue
				}
				v.Source = file.Source
				if file.Path == "main.go" {
					v.Source = strings.Replace(v.Source, "greet(", "inexistente(", 1)
				}
				return nil
			}
			return errors.New("arquivo inesperado")
		case *ProjectPatch:
			data := input.(map[string]any)
			diagnostic := data["diagnóstico"].(string)
			if strings.Contains(diagnostic, "undefined: inexistente") {
				repaired = true
				v.Files = []ProjectFile{demoProjectFiles[0]}
			} else {
				return errors.New(diagnostic)
			}
		default:
			return errors.New("resposta inesperada")
		}
		return nil
	}
	if err := e.startSession("Crie um programa de saudação para dois nomes", projectKind); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.pause)
	deadline := time.Now().Add(5 * time.Minute)
	for {
		_, running := e.snapshot()
		if !running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("compilação não concluiu")
		}
		time.Sleep(50 * time.Millisecond)
	}
	s, _ := e.snapshot()
	m := s.Missions[0]
	if !repaired || m.Project.Attempts != 2 || m.Phase != "aprovação" || len(m.Tests) != 4 || len(m.Project.Files) != 3 {
		t.Fatalf("fluxo inesperado: %+v", m)
	}
	data, err := os.ReadFile(filepath.Join(projectFolder(e, m), projectBinary))
	if err != nil || len(data) < 2 || string(data[:2]) != "MZ" {
		t.Fatalf("executável Windows ausente: %v", err)
	}
}

func TestProjectToolchainArchive(t *testing.T) {
	archive := os.Getenv("JARVIS_GO_ARCHIVE")
	if archive == "" {
		t.Skip("arquivo oficial do compilador não fornecido")
	}
	dir := t.TempDir()
	if err := extractProjectGo(context.Background(), archive, dir); err != nil {
		t.Fatal(err)
	}
}

func TestProjectKeepsDraftAndResumesCSV(t *testing.T) {
	e := fakeEngine(t)
	dir := filepath.Join(e.profile, "missoes", "missao-000001")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	goal := "Leia CSV do stdin, conte linhas e some valor"
	plan := ProjectPlan{Title: "Contador CSV", Summary: goal, Cases: []ProjectCase{
		{Name: "CSV Básico", Stdin: "id,valor\n1,100\n2,200\n3,300\n", Stdout: "Linhas: 3\nTotal Valor: 600"},
		{Name: "CSV Vazio", Stdin: "id,valor\n", Stdout: "Linhas: 0\nTotal Valor: 0"},
		{Name: "CSV com Valores Negativos", Stdin: "id,valor\n1,-50\n2,100\n3,-25\n", Stdout: "Linhas: 3\nTotal Valor: 25"},
		{Name: "CSV com Linhas Extras", Stdin: "id,valor\n1,100\n# comentário\n2,200\n", Stdout: "Linhas: 2\nTotal Valor: 300"},
	}}
	state := e.state
	state.Goal = goal
	state.Missions = []Mission{{ID: "missao-000001", Phase: "código", Kind: projectKind, Goal: goal, Failure: "limite da sessão: gere de 3 a 8 arquivos Go", Project: ProjectState{Plan: plan, Failed: 4}}}
	if err := e.save(state); err != nil {
		t.Fatal(err)
	}
	firstLogic := `package main
import("encoding/csv";"io";"strconv")
func count(in io.Reader)(int,int64){
 r:=csv.NewReader(in);r.Comment='#'
 head,err:=r.Read();if err!=nil{return 0,0}
 value:=-1
 for i,name:=range head{if name=="valor"{value=i;break}}
 if value<0{return 0,0}
 n:=0
 var total int64
 for{fields,err:=r.Read();if err==io.EOF{break};if err!=nil{continue}
  v,err:=strconv.ParseInt(fields[value],10,64);if err!=nil{continue}
  n++;total+=v
 }
 return n,total
}`
	mainCode := `package main
import("fmt";"os")
func main(){n,total:=count(os.Stdin);fmt.Printf("Linhas: %d\nTotal Valor: %d\n",n,total)}`
	testCode := `package main
import("strings";"testing")
func TestCount(t *testing.T){n,total:=count(strings.NewReader("id,valor\n1,-5\n"));if n!=1 || total!=-5 {t.Fatalf("%d %d",n,total)}}`
	e.ask = func(_ context.Context, _ string, input, out any) error {
		data := input.(map[string]any)
		response := out.(*ProjectSource)
		switch data["arquivo_pedido"] {
		case "logic.go":
			response.Source = firstLogic
		case "main.go":
			if len(data["arquivos"].([]ProjectFile)) != 1 {
				return errors.New("lógica não preservada")
			}
			response.Source = ""
		default:
			return errors.New("etapa inesperada")
		}
		return nil
	}
	if err := e.startSession(goal, projectKind); err != nil {
		t.Fatal(err)
	}
	waitSession(t, e)
	reopened, err := openEngine(e.profile)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := reopened.snapshot()
	if len(s.Missions[0].Project.Files) != 1 || s.Missions[0].Project.Files[0].Path != "logic.go" || s.Missions[0].Project.Attempts != 0 {
		t.Fatalf("rascunho não foi preservado: %+v", s.Missions[0].Project)
	}
	reopened.ask = func(_ context.Context, _ string, input, out any) error {
		data := input.(map[string]any)
		response := out.(*ProjectSource)
		switch data["arquivo_pedido"] {
		case "main.go":
			response.Source = mainCode
		case "logic_test.go":
			response.Source = testCode
		default:
			return errors.New("tentou regerar o arquivo já salvo")
		}
		return nil
	}
	reopened.build = func(ctx context.Context, dir string, files []ProjectFile, accept ProjectPlan) ([]string, string, error) {
		return compileProject(ctx, runtime.GOROOT(), dir, files, accept)
	}
	if err := reopened.startSession(goal, projectKind); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.pause)
	deadline := time.Now().Add(5 * time.Minute)
	for {
		_, running := reopened.snapshot()
		if !running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("compilação não concluiu")
		}
		time.Sleep(50 * time.Millisecond)
	}
	s, _ = reopened.snapshot()
	m := s.Missions[0]
	if m.Phase != "aprovação" || len(m.Tests) != 6 || m.Project.Attempts != 1 || m.Project.Plan.Cases[3].Stdin != plan.Cases[3].Stdin {
		t.Fatalf("CSV não chegou à aprovação: fase %s, erro %s, testes %v", m.Phase, m.Failure, m.Tests)
	}
}
