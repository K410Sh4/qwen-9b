package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"math"
	"strings"
)

type Evidence struct {
	ID      string
	Title   string
	Summary string
	Source  string
	Link    string
}

type Example struct {
	A, B, Expected float64
}

type Tool struct {
	Name, Operation, InputA, InputB, Result string
	Tests                                   []Example
}

type Opportunity struct {
	Title, Audience, Problem, Hypothesis, Contradiction, NextTask string
	EvidenceIDs                                                   []string
}

type Critique struct {
	Pass                     bool
	Reason, Lesson, Strategy string
}

func runTool(op string, a, b float64) (float64, error) {
	if math.IsNaN(a) || math.IsNaN(b) || math.IsInf(a, 0) || math.IsInf(b, 0) {
		return 0, errors.New("entrada inválida")
	}
	var value float64
	switch op {
	case "sum":
		value = a + b
	case "difference":
		value = a - b
	case "product":
		value = a * b
	case "ratio":
		if b == 0 {
			return 0, errors.New("O divisor precisa ser diferente de zero.")
		}
		value = a / b
	case "min":
		value = math.Min(a, b)
	case "max":
		value = math.Max(a, b)
	default:
		return 0, errors.New("operação desconhecida")
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, errors.New("resultado fora do intervalo")
	}
	return value, nil
}

func testTool(t Tool) ([]string, error) {
	for _, field := range []string{t.Name, t.InputA, t.InputB, t.Result} {
		if strings.TrimSpace(field) == "" || len(field) > 150 {
			return nil, errors.New("rótulos inválidos")
		}
	}
	if len(t.Tests) < 2 || len(t.Tests) > 6 {
		return nil, errors.New("forneça de dois a seis testes de aceite")
	}
	unique := map[[2]float64]bool{}
	cases := map[string]float64{"sum": 5, "difference": 1, "product": 6, "ratio": 1.5, "min": 2, "max": 3}
	expected, ok := cases[t.Operation]
	actual, err := runTool(t.Operation, 3, 2)
	if !ok || err != nil || actual != expected {
		return nil, errors.New("teste fixo da operação falhou")
	}
	if _, err = runTool("ratio", 3, 0); err == nil {
		return nil, errors.New("teste fixo de divisão por zero falhou")
	}
	results := []string{"Teste fixo da operação e divisão por zero: OK"}
	for _, c := range t.Tests {
		key := [2]float64{c.A, c.B}
		if unique[key] {
			return nil, errors.New("os testes precisam usar entradas distintas")
		}
		unique[key] = true
		actual, err = runTool(t.Operation, c.A, c.B)
		if err != nil || math.Abs(actual-c.Expected) > 1e-9*math.Max(1, math.Abs(c.Expected)) {
			return nil, fmt.Errorf("teste (%g, %g): esperado %g, obtido %g (%v)", c.A, c.B, c.Expected, actual, err)
		}
		results = append(results, fmt.Sprintf("Entrada (%g, %g) → %g: OK", c.A, c.B, actual))
	}
	return results, nil
}

var page = template.Must(template.New("prototype").Parse(`<!doctype html>
<html lang="pt-BR"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; form-action 'none'; base-uri 'none'">
<title>{{.Tool.Name}}</title>
<style>body{font:17px/1.5 system-ui,sans-serif;background:#111923;color:#edf5ff;margin:0}main{max-width:650px;margin:3rem auto;padding:0 1rem}h1{font-size:2.5rem;line-height:1.15}form{padding:1.4rem;border:1px solid #506378;border-radius:12px}label,input{display:block}input{box-sizing:border-box;width:100%;margin:.35rem 0 1rem;padding:.6rem;font:inherit}button{padding:.6rem 1rem;font:inherit;font-weight:650;background:#8de4ba;color:#101d16;border:0;border-radius:6px}output{display:block;margin-top:1rem;font-weight:650}</style>
<main><h1>{{.Tool.Name}}</h1><p>{{.Problem}}</p>
<form id="form"><label for="a">{{.Tool.InputA}}</label><input id="a" type="number" step="any" required><label for="b">{{.Tool.InputB}}</label><input id="b" type="number" step="any" required><button>Calcular</button><output id="result" aria-live="polite"></output></form></main>
<script>
const operation = {{.Tool.Operation}};
const label = {{.Tool.Result}};
const evaluate = (a, b) => {
 if (!Number.isFinite(a) || !Number.isFinite(b)) throw Error("Preencha as duas entradas.");
 if (operation === "ratio" && b === 0) throw Error("O divisor precisa ser diferente de zero.");
 const operations = {sum:()=>a+b, difference:()=>a-b, product:()=>a*b, ratio:()=>a/b, min:()=>Math.min(a,b), max:()=>Math.max(a,b)};
 const value = operations[operation]();
 if (!Number.isFinite(value)) throw Error("Resultado fora do intervalo.");
 return value;
};
document.getElementById("form").addEventListener("submit", (event) => {
 event.preventDefault();
 const result=document.getElementById("result");
 try {
  const a=document.getElementById("a").valueAsNumber;
  const b=document.getElementById("b").valueAsNumber;
  result.textContent=label+": "+new Intl.NumberFormat("pt-BR",{maximumFractionDigits:10}).format(evaluate(a,b));
 } catch (error) {result.textContent=error.message;}
});
</script></html>`))

func compilePrototype(t Tool, o Opportunity) ([]byte, error) {
	var output bytes.Buffer
	err := page.Execute(&output, struct {
		Tool    Tool
		Problem string
	}{t, o.Problem})
	return output.Bytes(), err
}

func validOpportunity(o Opportunity, evidence []Evidence) error {
	for _, field := range []string{o.Title, o.Audience, o.Problem, o.Hypothesis, o.Contradiction, o.NextTask} {
		if strings.TrimSpace(field) == "" || len(field) > 700 {
			return errors.New("a oportunidade tem campos vazios ou longos demais")
		}
	}
	sources := map[string]string{}
	for _, e := range evidence {
		sources[e.ID] = e.Source
	}
	domains := map[string]bool{}
	if len(o.EvidenceIDs) > 6 {
		return errors.New("limite de seis referências")
	}
	for _, id := range o.EvidenceIDs {
		if sources[id] == "" {
			return fmt.Errorf("referência não coletada: %s", id)
		}
		domains[sources[id]] = true
	}
	if len(domains) < 2 {
		return errors.New("são necessárias fontes de dois domínios distintos")
	}
	return nil
}

func sameTool(a, b Tool) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}
