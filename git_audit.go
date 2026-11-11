package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const advisoryURL = "https://github.blog/open-source/git/git-security-vulnerabilities-announced-7/"
const gitAuditKind = "auditoria_git"
const calculatorKind = "calculadora"

var gitVersionPattern = regexp.MustCompile(`^git version 2\.([0-9]{1,3})\.([0-9]{1,4})(\.windows\.[0-9]{1,4})?$`)
var gitSettings = [...]string{"core.hooksPath", "protocol.file.allow", "submodule.recurse", "fetch.recurseSubmodules"}

func gitAdvisoryStatus(version string) string {
	match := gitVersionPattern.FindStringSubmatch(strings.TrimSpace(version))
	if match == nil {
		return "Versão fora do formato reconhecido. Consulte o distribuidor do Git."
	}
	minor, _ := strconv.Atoi(match[1])
	patch, _ := strconv.Atoi(match[2])
	thresholds := map[int]int{50: 2, 51: 3, 52: 1, 53: 1, 54: 1, 55: 1, 56: 1}
	fixed, known := thresholds[minor]
	switch {
	case minor > 56 || known && patch >= fixed:
		return "Versão compatível com a correção deste aviso. Confira a procedência do binário."
	case known:
		return fmt.Sprintf("A versão informada precede 2.%d.%d na mesma série. Consulte o distribuidor para atualizar ou confirmar um backport.", minor, fixed)
	default:
		return "Série sem correção listada neste aviso. Consulte o distribuidor do Git."
	}
}

func gitAuditEvidence(m Mission) (Evidence, bool) {
	// A intenção precisa mencionar Git e um diagnóstico. O aviso sozinho não muda a entrega.
	task := strings.ToLower(m.Opportunity.Title + " " + m.Opportunity.NextTask)
	wantsGit := strings.Contains(task, " git") && (strings.Contains(task, "audit") || strings.Contains(task, "vulnerabil"))
	for _, e := range m.Evidence {
		if wantsGit && strings.TrimRight(e.Link, "/") == strings.TrimRight(advisoryURL, "/") {
			return e, true
		}
	}
	return Evidence{}, false
}

func prepareGitAudit(m *Mission, advisory Evidence) {
	m.Kind = gitAuditKind
	m.Opportunity = Opportunity{
		Title:         "Diagnóstico local de atualização do Git",
		Audience:      "Equipes que autorizam agentes a operar repositórios",
		Problem:       "O aviso do Git de 10/11/2026 lista correções para seis vulnerabilidades. É preciso saber qual binário a automação usa e quais opções estão declaradas no repositório.",
		Hypothesis:    "Um relatório local pode reduzir o esforço para conferir a versão e as configurações selecionadas antes de habilitar a automação.",
		Contradiction: "A hipótese perde força se a equipe já identifica com precisão o binário e verifica as opções no fluxo atual.",
		NextTask:      "Conferir o Git autorizado e ler quatro opções do arquivo de configuração escolhido.",
		EvidenceIDs:   []string{advisory.ID},
	}
	m.Tool = Tool{}
	m.Tests = nil
	m.Review = Critique{}
	m.Failure = ""
	m.Phase = "protótipo"
	m.Log = append(m.Log, "Escopo corrigido: diagnóstico local do aviso do Git. A fonte demonstra o problema técnico. Demanda de mercado ainda não foi medida.")
}

func gitAuditChecks() ([]string, error) {
	cases := []struct{ input, result string }{
		{"git version 2.56.0.windows.1", "precede 2.56.1"},
		{"git version 2.56.1.windows.1", "compatível"},
		{"git version 2.51.2", "precede 2.51.3"},
		{"git version 2.51.3", "compatível"},
		{"git version 2.56.1-rc0", "fora do formato"},
	}
	var results []string
	for _, c := range cases {
		if !strings.Contains(gitAdvisoryStatus(c.input), c.result) {
			return nil, fmt.Errorf("classificação inesperada para %s", c.input)
		}
		results = append(results, strings.TrimPrefix(c.input, "git version ")+" → "+c.result+": OK")
	}
	return results, nil
}

type GitSetting struct{ Key, Value string }
type GitAuditReport struct {
	Git, Version, Status string
	Settings             []GitSetting
}

func readGitAudit(ctx context.Context, git, config, workingDir string) (GitAuditReport, error) {
	if !filepath.IsAbs(git) || !filepath.IsAbs(config) || strings.HasPrefix(git, `\\`) || strings.HasPrefix(config, `\\`) {
		return GitAuditReport{}, errors.New("escolha caminhos locais absolutos")
	}
	info, err := os.Stat(config)
	if err != nil {
		return GitAuditReport{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > 256<<10 {
		return GitAuditReport{}, errors.New("selecione um arquivo de configuração regular de até 256 KiB")
	}
	run := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, git, args...)
		cmd.Dir = workingDir
		for _, v := range os.Environ() {
			if !strings.HasPrefix(strings.ToUpper(v), "GIT_") {
				cmd.Env = append(cmd.Env, v)
			}
		}
		cmd.Env = append(cmd.Env, "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
		out, err := cmd.Output()
		return shortened(string(out), 8192), err
	}
	version, err := run("--version")
	if err != nil {
		return GitAuditReport{}, fmt.Errorf("não foi possível ler a versão: %w", err)
	}
	version = strings.TrimSpace(version)
	if !strings.HasPrefix(version, "git version ") {
		return GitAuditReport{}, errors.New("o executável não retornou uma versão reconhecível do Git")
	}
	report := GitAuditReport{Git: git, Version: version, Status: gitAdvisoryStatus(version)}
	for _, key := range gitSettings {
		value, err := run("config", "--file", config, "--no-includes", "--null", "--get-all", key)
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 && ctx.Err() == nil {
			value = "Não declarada no arquivo selecionado"
		} else if err != nil {
			return GitAuditReport{}, fmt.Errorf("falha lendo %s: %w", key, err)
		} else {
			value = strings.Join(strings.Split(strings.TrimSuffix(value, "\x00"), "\x00"), " · ")
			if value == "" {
				value = "Declarada sem valor"
			}
		}
		report.Settings = append(report.Settings, GitSetting{key, value})
	}
	return report, nil
}

var gitAuditPage = template.Must(template.New("git-audit").Parse(`<!doctype html><html lang="pt-BR">
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'">
<title>Diagnóstico local do Git</title>
<style>body{font:16px/1.6 system-ui,sans-serif;max-width:760px;margin:32px auto;padding:0 16px;color:#172536}li{overflow-wrap:anywhere}</style>
<h1>Diagnóstico local do Git</h1><p><strong>{{.Version}}</strong> · {{.Git}}</p><p>{{.Status}}</p>
<h2>Configurações declaradas no arquivo selecionado</h2><ul>{{range .Settings}}<li><code>{{.Key}}</code> · {{.Value}}</li>{{end}}</ul>
<p>Escopo: versão reportada pelo binário autorizado e opções declaradas no arquivo. Configurações globais e arquivos incluídos estão fora deste relatório.</p>
<p><a href="https://github.blog/open-source/git/git-security-vulnerabilities-announced-7/">Aviso do Git de 10/11/2026</a></p>
</html>`))

func renderGitAudit(r GitAuditReport) ([]byte, error) {
	var out bytes.Buffer
	err := gitAuditPage.Execute(&out, r)
	return out.Bytes(), err
}
