package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const projectKind = "projeto_go"
const projectBinary = "programa.exe"

type ProjectCase struct {
	Name          string
	Args          []string
	Stdin, Stdout string
}
type ProjectPlan struct {
	Title, Summary string
	Cases          []ProjectCase
}
type ProjectFile struct{ Path, Source string }
type ProjectPatch struct{ Files []ProjectFile }
type ProjectState struct {
	Plan             ProjectPlan
	Files            []ProjectFile
	Attempts, Failed int
	OutputDir        string
}

func validateProjectPlan(p ProjectPlan) error {
	if strings.TrimSpace(p.Title) == "" || len(p.Title) > 100 || strings.TrimSpace(p.Summary) == "" || len(p.Summary) > 900 || len(p.Cases) < 2 || len(p.Cases) > 4 {
		return errors.New("o plano precisa de nome, descrição e de dois a quatro testes de terminal")
	}
	seen := map[string]bool{}
	outputs := map[string]bool{}
	for _, c := range p.Cases {
		if strings.TrimSpace(c.Name) == "" || len(c.Name) > 80 || len(c.Args) > 8 || len(c.Stdin) > 1024 || len(c.Stdout) > 1024 {
			return errors.New("um teste do terminal excede os limites")
		}
		for _, arg := range c.Args {
			if len(arg) > 160 || strings.IndexByte(arg, 0) >= 0 {
				return errors.New("argumento de teste inválido")
			}
		}
		encoded, _ := json.Marshal(struct {
			Args  []string
			Stdin string
		}{c.Args, c.Stdin})
		if seen[string(encoded)] {
			return errors.New("os testes precisam de entradas diferentes")
		}
		seen[string(encoded)] = true
		outputs[normalizeConsole(c.Stdout)] = true
	}
	if len(outputs) < 2 {
		return errors.New("o plano precisa de pelo menos duas saídas esperadas diferentes")
	}
	return nil
}

var goFileName = regexp.MustCompile(`^[a-z][a-z0-9]{0,34}(_test)?\.go$`)
var windowsDevice = regexp.MustCompile(`^(con|prn|aux|nul|com[0-9]|lpt[0-9])\.go$`)

var projectImports = map[string]bool{
	"bufio": true, "bytes": true, "cmp": true, "encoding/base64": true, "encoding/csv": true, "encoding/hex": true,
	"encoding/json": true, "errors": true, "flag": true, "fmt": true, "io": true, "maps": true,
	"math": true, "math/big": true, "math/rand": true, "os": true, "regexp": true, "slices": true, "sort": true,
	"strconv": true, "strings": true, "testing": true, "text/tabwriter": true, "text/template": true,
	"time": true, "unicode": true, "unicode/utf8": true, "github.com/google/uuid": true,
}
var projectOS = map[string]bool{"Args": true, "Exit": true, "Stdin": true, "Stdout": true, "Stderr": true}

func checkProjectFiles(files []ProjectFile) error {
	if len(files) < 3 || len(files) > 8 {
		return errors.New("gere de 3 a 8 arquivos Go, com main.go, lógica separada e um arquivo _test.go")
	}
	total, unitCount := 0, 0
	mainFound := false
	seen := map[string]bool{}
	for _, file := range files {
		if !goFileName.MatchString(file.Path) || windowsDevice.MatchString(file.Path) || seen[file.Path] {
			return fmt.Errorf("nome de arquivo inválido ou repetido: %q", file.Path)
		}
		seen[file.Path] = true
		total += len(file.Source)
		if len(file.Source) == 0 || len(file.Source) > 6000 || total > 12000 {
			return errors.New("mantenha os arquivos compactos, até 12.000 bytes no total")
		}
		if strings.Contains(file.Source, "//go:") || strings.Contains(file.Source, "//line ") || strings.Contains(file.Source, "/*line ") || strings.Contains(file.Source, "// +build") {
			return fmt.Errorf("%s: diretivas do compilador estão fora do formato de projeto", file.Path)
		}
		fs := token.NewFileSet()
		parsed, err := parser.ParseFile(fs, file.Path, file.Source, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		if parsed.Name.Name != "main" {
			return fmt.Errorf("%s: use package main", file.Path)
		}
		for _, spec := range parsed.Imports {
			path, _ := strconv.Unquote(spec.Path.Value)
			if !projectImports[path] || spec.Name != nil {
				return fmt.Errorf("%s: importação fora do catálogo ou com alias: %q", file.Path, path)
			}
		}
		var invalid string
		ast.Inspect(parsed, func(n ast.Node) bool {
			if selector, ok := n.(*ast.SelectorExpr); ok {
				if pkg, ok := selector.X.(*ast.Ident); ok && pkg.Name == "os" && !projectOS[selector.Sel.Name] {
					invalid = selector.Sel.Name
					return false
				}
			}
			if d, ok := n.(*ast.FuncDecl); ok && d.Recv == nil {
				if file.Path == "main.go" && d.Name.Name == "main" {
					mainFound = true
				}
				if strings.HasSuffix(file.Path, "_test.go") && strings.HasPrefix(d.Name.Name, "Test") && d.Name.Name != "TestMain" {
					unitCount++
				}
				if strings.HasSuffix(file.Path, "_test.go") && d.Name.Name == "TestMain" {
					invalid = "TestMain"
				}
			}
			return true
		})
		if invalid != "" {
			return fmt.Errorf("%s: símbolo fora do formato de projeto: %s", file.Path, invalid)
		}
	}
	if !mainFound || unitCount == 0 {
		return errors.New("inclua a função main em main.go e pelo menos um teste de unidade")
	}
	return nil
}

func mergeProjectFiles(old, patch []ProjectFile) ([]ProjectFile, error) {
	if len(patch) == 0 || len(patch) > 8 {
		return nil, errors.New("envie de um a oito arquivos alterados")
	}
	merged := map[string]ProjectFile{}
	for _, file := range old {
		merged[file.Path] = file
	}
	seen := map[string]bool{}
	for _, file := range patch {
		if seen[file.Path] {
			return nil, fmt.Errorf("arquivo repetido: %s", file.Path)
		}
		seen[file.Path] = true
		merged[file.Path] = file
	}
	files := make([]ProjectFile, 0, len(merged))
	for _, file := range merged {
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, checkProjectFiles(files)
}

func (e *Engine) stepProject(ctx context.Context, m *Mission) error {
	p := &m.Project
	switch m.Phase {
	case "especificação":
		const prompt = `Planeje o projeto solicitado como um programa Go de terminal. Use argumentos e stdin para entradas e stdout determinístico para respostas. Use 2 a 4 casos de aceite distintos, fixos, que representem requisitos reais. Args deve ser um array; Stdin pode ser vazio. Cada caso deve terminar com código de saída 0. Sem rede, interface gráfica ou arquivos externos. JSON: {"Title":"","Summary":"","Cases":[{"Name":"","Args":[""],"Stdin":"","Stdout":""}]}`
		var plan ProjectPlan
		if err := e.ask(ctx, prompt, m.Goal, &plan); err != nil {
			return err
		}
		if err := validateProjectPlan(plan); err != nil {
			return err
		}
		p.Plan, m.Phase = plan, "código"
	case "código":
		const prompt = `Implemente o objetivo e o plano de aceite congelado. Devolva os arquivos completos alterados, mantendo os demais. Comece com main.go, lógica separada (exemplo: logic.go) e testes logic_test.go, todos package main. Nomes só com letras e dígitos, salvo o sufixo _test.go. Até 12.000 bytes no total. Use biblioteca padrão de processamento de texto e dados; os só para Args, Exit, Stdin, Stdout e Stderr. UUID opcional via github.com/google/uuid. Sem shell, rede, arquivos externos, aliases de importação, TestMain ou diretivas do compilador. Os testes não podem substituir o funcionamento real do programa. Se houver diagnóstico, corrija a causa. JSON: {"Files":[{"Path":"main.go","Source":"package main\n..."}]}`
		input := map[string]any{"objetivo": m.Goal, "plano": p.Plan, "arquivos": p.Files, "diagnóstico": m.Failure}
		var patch ProjectPatch
		if err := e.ask(ctx, prompt, input, &patch); err != nil {
			return err
		}
		files, err := mergeProjectFiles(p.Files, patch.Files)
		if err != nil {
			m.Failure = shortened(err.Error(), 1000)
			p.Failed++
			m.Log = append(m.Log, "Validação do código: "+m.Failure)
			if p.Failed >= 4 {
				return fmt.Errorf("limite da sessão: %w", err)
			}
			return nil
		}
		p.Files, m.Phase = files, "compilar projeto"
	case "compilar projeto":
		dir := filepath.Join(e.profile, "missoes", m.ID)
		p.Attempts++
		buildCtx, cancel := context.WithTimeout(ctx, 8*time.Minute)
		defer cancel()
		results, output, err := e.build(buildCtx, dir, p.Files, p.Plan)
		if err != nil {
			var diagnostic *projectDiagnostic
			if !errors.As(err, &diagnostic) {
				return err
			}
			p.Failed++
			m.Failure = shortened(err.Error(), 1800)
			m.Log = append(m.Log, fmt.Sprintf("Tentativa %d: %s", p.Attempts, m.Failure))
			m.Phase = "código"
			if p.Failed >= 4 {
				return fmt.Errorf("limite da sessão: %w", err)
			}
			return nil
		}
		p.OutputDir = output
		m.Tests, m.Failure, m.Phase = results, "", "aprovação"
	default:
		return errors.New("etapa de projeto desconhecida")
	}
	m.Log = append(m.Log, "Próxima etapa: "+m.Phase)
	return nil
}

func projectFolder(e *Engine, m Mission) string {
	base := filepath.Join(e.profile, "missoes", m.ID)
	if m.Project.OutputDir == filepath.Base(m.Project.OutputDir) && strings.HasPrefix(m.Project.OutputDir, "tentativa-") {
		return filepath.Join(base, m.Project.OutputDir)
	}
	return base
}

func writeProjectPlan(dir string, plan ProjectPlan) error {
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "aceite.json"), data, 0600)
}
