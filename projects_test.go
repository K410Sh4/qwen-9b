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
		case *ProjectPatch:
			data := input.(map[string]any)
			diagnostic := data["diagnóstico"].(string)
			if diagnostic == "" {
				v.Files = append([]ProjectFile{}, demoProjectFiles...)
				v.Files[0].Source = strings.Replace(v.Files[0].Source, "greet(", "inexistente(", 1)
			} else if strings.Contains(diagnostic, "undefined: inexistente") {
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
