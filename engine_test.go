package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var sampleTool = Tool{Name: "Calculadora de custo", Operation: "sum", InputA: "Preço", InputB: "Frete", Result: "Total", Tests: []Example{{3, 2, 5}, {10, 2, 12}}}

func waitSession(t *testing.T, e *Engine) {
	t.Helper()
	limit := time.Now().Add(5 * time.Second)
	for time.Now().Before(limit) {
		if _, running := e.snapshot(); !running {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("sessão não finalizou")
}

func fakeEngine(t *testing.T) *Engine {
	t.Helper()
	e, err := openEngine(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e.collect = func(context.Context, string, string) ([]Evidence, []string) {
		return []Evidence{{ID: "F1", Source: "a.example", Title: "Um"}, {ID: "F2", Source: "b.example", Title: "Dois"}}, nil
	}
	e.ask = func(_ context.Context, _ string, _ any, out any) error {
		switch x := out.(type) {
		case *Opportunity:
			*x = Opportunity{Title: "Uma ideia", Audience: "Público", Problem: "Preço total", Hypothesis: "A soma ajuda", Contradiction: "Pessoas já automatizam", NextTask: "Comparar", EvidenceIDs: []string{"F1", "F2"}}
		case *Tool:
			*x = sampleTool
		case *Critique:
			*x = Critique{Pass: true, Reason: "Atende ao cenário", Lesson: "Testar com novos usuários", Strategy: "contrapontos"}
		default:
			return errors.New("resposta inesperada")
		}
		return nil
	}
	return e
}

func TestSessionRequiresStartAndApproval(t *testing.T) {
	e := fakeEngine(t)
	const goal = "Encontrar oportunidade e criar protótipo"
	if err := e.resolve(true); err == nil {
		t.Fatal("aprovação sem missão")
	}
	if err := e.start(goal); err != nil {
		t.Fatal(err)
	}
	waitSession(t, e)
	s, _ := e.snapshot()
	if len(s.Missions) != 1 || s.Missions[0].Phase != "aprovação" || len(s.Tools) != 1 || len(s.Missions[0].Tests) != 3 {
		t.Fatalf("etapas incorretas: %+v", s)
	}
	if err := e.start(goal); err == nil {
		t.Fatal("iniciou sem decisão sobre a missão anterior")
	}
	if err := e.resolve(true); err != nil {
		t.Fatal(err)
	}
	reopened, err := openEngine(e.profile)
	if err != nil {
		t.Fatal(err)
	}
	s, running := reopened.snapshot()
	if running || s.Missions[0].Phase != "concluída" || s.Candidate != "contrapontos" {
		t.Fatalf("memória incorreta: %+v", s)
	}
	html, err := os.ReadFile(filepath.Join(e.profile, "missoes", s.Missions[0].ID, "prototipo.html"))
	if err != nil || !strings.Contains(string(html), "Calculadora de custo") {
		t.Fatal("protótipo ausente", err)
	}
}

func TestPauseRequiresExplicitResume(t *testing.T) {
	e := fakeEngine(t)
	collect := e.collect
	entered := make(chan struct{})
	e.collect = func(ctx context.Context, _, _ string) ([]Evidence, []string) {
		close(entered)
		<-ctx.Done()
		return nil, nil
	}
	const goal = "Pesquisar e construir uma ferramenta"
	if err := e.start(goal); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("não iniciou")
	}
	e.pause()
	waitSession(t, e)
	reloaded, err := openEngine(e.profile)
	if err != nil {
		t.Fatal(err)
	}
	state, running := reloaded.snapshot()
	if running || state.Missions[0].Phase != "fontes" {
		t.Fatalf("retomada incorreta: %+v", state)
	}
	reloaded.ask, reloaded.collect = e.ask, collect
	if err := reloaded.start(goal); err != nil {
		t.Fatal(err)
	}
	waitSession(t, reloaded)
	state, _ = reloaded.snapshot()
	if len(state.Missions) != 1 || state.Missions[0].Phase != "aprovação" {
		t.Fatalf("missão não retomada: %+v", state)
	}
}

func TestPrototypeMainFlow(t *testing.T) {
	if _, err := testTool(sampleTool); err != nil {
		t.Fatal(err)
	}
	x := sampleTool
	x.Tests = []Example{{3, 2, 5}, {10, 2, 2}}
	if _, err := testTool(x); err == nil {
		t.Fatal("aceitou caso incorreto")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node indisponível para simular o DOM")
	}
	b, err := compilePrototype(sampleTool, Opportunity{Problem: "<script>forged()</script>"})
	if err != nil {
		t.Fatal(err)
	}
	code := strings.SplitN(strings.SplitN(string(b), "<script>", 2)[1], "</script>", 2)[0]
	setup := `const controls = {a:{valueAsNumber:10},b:{valueAsNumber:2},result:{},form:{addEventListener(_,f){this.submit=f}}}; const document={getElementById(id){return controls[id]}};`
	check := `controls.form.submit({preventDefault(){}}); if (controls.result.textContent !== "Total: 12") process.exit(1); controls.a.valueAsNumber=NaN;controls.form.submit({preventDefault(){}}); if (!controls.result.textContent.includes("Preencha")) process.exit(2);`
	if output, err := exec.Command(node, "-e", setup+code+check).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, output)
	}
	if strings.Contains(string(b), "<script>forged") {
		t.Fatal("a entrada do modelo não foi escapada")
	}
}

func TestFeedAndVersionValidation(t *testing.T) {
	raw := []byte(`<rss><channel><item><title>Teste</title><link>https://example.com/post</link><description>Texto &amp; dados.</description></item></channel></rss>`)
	entries, err := parseFeed(raw, "example.com")
	if err != nil || len(entries) != 1 || entries[0].Summary != "Texto & dados." {
		t.Fatalf("%+v %v", entries, err)
	}
	if !newer("v0.10.0", "v0.9.2") || newer("v0.9.0", "v0.9.0") || newer("v0.10.0-beta", "v0.9.0") || newer("v00.10.0", "v0.9.0") {
		t.Fatal("comparação de versões incorreta")
	}
}
