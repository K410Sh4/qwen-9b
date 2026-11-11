package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

type projectDiagnostic struct{ info string }

func (e *projectDiagnostic) Error() string { return e.info }

type boundedOutput struct {
	buffer bytes.Buffer
	max    int
}

func (o *boundedOutput) String() string { return o.buffer.String() }
func (o *boundedOutput) Write(data []byte) (int, error) {
	n := len(data)
	if remaining := o.max - o.buffer.Len(); remaining > 0 {
		_, _ = o.buffer.Write(data[:min(remaining, n)])
	}
	return n, nil
}

func projectEnvironment(root, target, cache, modcache string, download bool) []string {
	env := []string{}
	for _, name := range []string{"SystemRoot", "WINDIR", "SYSTEMDRIVE", "TMP", "TEMP", "TMPDIR", "USERPROFILE", "HOME", "LOCALAPPDATA", "APPDATA"} {
		if v, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+v)
		}
	}
	proxy := "off"
	if download {
		proxy = "https://proxy.golang.org"
		for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy"} {
			if value, ok := os.LookupEnv(name); ok {
				env = append(env, name+"="+value)
			}
		}
	}
	arch := "wasm"
	if target == "windows" {
		arch = "amd64"
	}
	return append(env, "GOROOT="+root, "GOOS="+target, "GOARCH="+arch, "CGO_ENABLED=0", "GOWORK=off", "GOTOOLCHAIN=local",
		"GOENV=off", "GOFLAGS=", "GOPROXY="+proxy, "GOSUMDB=off", "GOCACHE="+cache, "GOMODCACHE="+modcache,
		"GOPATH="+filepath.Dir(modcache), "GOMAXPROCS=3")
}

func compileProject(ctx context.Context, root, mission string, files []ProjectFile, plan ProjectPlan) ([]string, string, error) {
	if err := checkProjectFiles(files); err != nil {
		return nil, "", &projectDiagnostic{err.Error()}
	}
	if err := validateProjectPlan(plan); err != nil {
		return nil, "", err
	}
	dir, err := os.MkdirTemp(mission, "tentativa-")
	if err != nil {
		return nil, "", err
	}
	cache := filepath.Join(mission, "cache-go")
	modcache := filepath.Join(mission, "dependencias")
	if err = os.MkdirAll(cache, 0700); err != nil {
		return nil, "", err
	}
	mod := "module jarvis.local/projeto\n\ngo 1.23.0\n"
	uuid := false
	for _, file := range files {
		if strings.Contains(file.Source, `"github.com/google/uuid"`) {
			uuid = true
		}
		if err = os.WriteFile(filepath.Join(dir, file.Path), []byte(file.Source), 0600); err != nil {
			return nil, "", err
		}
	}
	if uuid {
		mod += "\nrequire github.com/google/uuid v1.6.0\n"
		const sums = "github.com/google/uuid v1.6.0 h1:NIvaJDMOsjHA8n1jAhLSgzrAzy1Hgr+hNrb57e+94F0=\ngithub.com/google/uuid v1.6.0/go.mod h1:TIyPZe4MgqvfeYDBFedMoGGpEw/LqOeaOT+nhxU+yHo=\n"
		if err = os.WriteFile(filepath.Join(dir, "go.sum"), []byte(sums), 0600); err != nil {
			return nil, "", err
		}
	}
	if err = os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0600); err != nil {
		return nil, "", err
	}
	if err = writeProjectPlan(dir, plan); err != nil {
		return nil, "", err
	}
	goRun := func(target string, download bool, args ...string) error {
		cmd := exec.CommandContext(ctx, goTool(root), args...)
		configureProjectProcess(cmd)
		cmd.Dir = dir
		cmd.Env = projectEnvironment(root, target, cache, modcache, download)
		var output = boundedOutput{max: 12000}
		cmd.Stdout, cmd.Stderr = &output, &output
		if err := cmd.Run(); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			var exit *exec.ExitError
			if !errors.As(err, &exit) {
				return err
			}
			diagnostic := strings.TrimSpace(output.String())
			if diagnostic == "" {
				diagnostic = err.Error()
			}
			if download {
				return fmt.Errorf("dependência aprovada indisponível: %s", shortened(diagnostic, 1800))
			}
			return &projectDiagnostic{shortened(diagnostic, 2600)}
		}
		return nil
	}
	if uuid {
		if err = goRun("wasip1", true, "mod", "download", "github.com/google/uuid@v1.6.0"); err != nil {
			return nil, "", err
		}
	}
	if err = goRun("wasip1", false, "build", "-p", "3", "-mod=readonly", "-trimpath", "-buildvcs=false", "-o", "programa.wasm", "."); err != nil {
		return nil, "", err
	}
	if err = goRun("wasip1", false, "test", "-c", "-p", "3", "-mod=readonly", "-trimpath", "-buildvcs=false", "-o", "testes.wasm", "."); err != nil {
		return nil, "", err
	}
	results, err := testProjectWasm(ctx, dir, plan)
	if err != nil {
		return nil, "", err
	}
	if err = goRun("windows", false, "build", "-p", "3", "-mod=readonly", "-trimpath", "-buildvcs=false", "-o", projectBinary, "."); err != nil {
		return nil, "", err
	}
	results = append(results, "Compilação do executável Windows x64: OK")
	if err = os.WriteFile(filepath.Join(dir, "testes.txt"), []byte(strings.Join(results, "\r\n")), 0600); err != nil {
		return nil, "", err
	}
	_ = os.Remove(filepath.Join(dir, "testes.wasm"))
	_ = os.Remove(filepath.Join(dir, "programa.wasm"))
	return results, filepath.Base(dir), nil
}

func normalizeConsole(s string) string {
	return strings.TrimSuffix(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
}

func testProjectWasm(ctx context.Context, dir string, plan ProjectPlan) ([]string, error) {
	config := wazero.NewRuntimeConfig().WithCloseOnContextDone(true).WithMemoryLimitPages(2048)
	rt := wazero.NewRuntimeWithConfig(ctx, config)
	defer rt.Close(ctx)
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		return nil, err
	}
	load := func(name string) (wazero.CompiledModule, error) {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		return rt.CompileModule(ctx, b)
	}
	run := func(m wazero.CompiledModule, args []string, stdin string) (string, string, error) {
		attempt, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		stdout, stderr := boundedOutput{max: 4096}, boundedOutput{max: 4096}
		cfg := wazero.NewModuleConfig().WithName("").WithArgs(args...).WithStdin(strings.NewReader(stdin)).
			WithStdout(&stdout).WithStderr(&stderr).WithSysWalltime().WithSysNanotime().WithSysNanosleep()
		mod, err := rt.InstantiateModule(attempt, m, cfg)
		if mod != nil {
			_ = mod.Close(ctx)
		}
		var exit *sys.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 0 {
			err = nil
		}
		return stdout.String(), stderr.String(), err
	}
	unit, err := load("testes.wasm")
	if err != nil {
		return nil, err
	}
	out, stderr, err := run(unit, []string{"testes", "-test.v"}, "")
	if err != nil || !strings.Contains(out, "--- PASS: Test") {
		return nil, &projectDiagnostic{fmt.Sprintf("testes de unidade: %v\n%s\n%s", err, shortened(out, 1600), shortened(stderr, 500))}
	}
	program, err := load("programa.wasm")
	if err != nil {
		return nil, err
	}
	results := []string{"Testes de unidade executados: OK"}
	for _, c := range plan.Cases {
		out, stderr, err = run(program, append([]string{"programa"}, c.Args...), c.Stdin)
		if err != nil {
			return nil, &projectDiagnostic{fmt.Sprintf("%s: execução falhou: %v (%s)", c.Name, err, shortened(stderr, 500))}
		}
		if got, want := normalizeConsole(out), normalizeConsole(c.Stdout); got != want {
			return nil, &projectDiagnostic{fmt.Sprintf("%s: esperado %q, recebido %q", c.Name, want, shortened(got, 600))}
		}
		results = append(results, c.Name+": OK")
	}
	return results, nil
}

var configureProjectProcess = func(*exec.Cmd) {}
