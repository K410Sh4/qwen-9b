package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type piperTask struct {
	ctx    context.Context
	text   string
	output string
	done   chan error
}

// piperWarmEngine starts downloading/loading the Faber model immediately when
// the overlay is constructed, but does not block the UI or Ollama. A single
// worker serializes all synthesis requests through one resident Python process.
type piperWarmEngine struct {
	ctx        context.Context
	profileDir string
	installer  func(context.Context) (string, error)
	tasks      chan piperTask
	ready      chan struct{}
	stopped    chan struct{}
	mu         sync.RWMutex
	preloadErr error
	starts     atomic.Int32
}

func newPiperWarmEngine(ctx context.Context, profileDir string) *piperWarmEngine {
	return newPiperWarmWithInstaller(ctx, profileDir, func(ctx context.Context) (string, error) {
		return preparePiper(ctx, profileDir)
	})
}

func newPiperWarmWithInstaller(ctx context.Context, profileDir string, installer func(context.Context) (string, error)) *piperWarmEngine {
	w := &piperWarmEngine{
		ctx: ctx, profileDir: profileDir, installer: installer,
		tasks: make(chan piperTask), ready: make(chan struct{}), stopped: make(chan struct{}),
	}
	go w.run()
	return w
}

func (w *piperWarmEngine) run() {
	defer close(w.stopped)
	session, err := w.startSession()
	w.mu.Lock()
	w.preloadErr = err
	w.mu.Unlock()
	close(w.ready)
	defer func() {
		if session != nil {
			session.close()
		}
	}()
	for {
		select {
		case <-w.ctx.Done():
			return
		case task := <-w.tasks:
			if err := task.ctx.Err(); err != nil {
				task.done <- err
				continue
			}
			if session == nil {
				session, err = w.startSession()
			}
			if err == nil && session != nil {
				err = session.synthesize(task.ctx, task.text, task.output)
			}
			if err != nil && session != nil {
				session.close()
				session = nil // Recreate a crashed or cancelled worker next time.
			}
			task.done <- err
			err = nil
		}
	}
}

type piperProcess struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	decoder *json.Decoder
	stderr  *bytes.Buffer
}

func (p *piperProcess) close() {
	_ = p.stdin.Close()
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	_ = p.cmd.Wait()
}

func (w *piperWarmEngine) startSession() (*piperProcess, error) {
	dir, err := w.installer(w.ctx)
	if err != nil {
		return nil, fmt.Errorf("preparar voz Faber: %w", err)
	}
	cmd := exec.CommandContext(w.ctx, filepath.Join(dir, "python.exe"), "-B",
		filepath.Join(dir, "synthesize.py"), "--serve")
	cmd.Dir = dir
	configureProjectProcess(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("iniciar Piper Faber: %w", err)
	}
	proc := &piperProcess{cmd: cmd, stdin: stdin, decoder: json.NewDecoder(bufio.NewReader(stdout)), stderr: &stderr}
	// Guard model load from hanging forever even if the Python process is alive.
	deadline := time.AfterFunc(45*time.Second, func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	var hello struct {
		Ready bool `json:"ready"`
	}
	err = proc.decoder.Decode(&hello)
	deadline.Stop()
	if err != nil || !hello.Ready {
		proc.close()
		return nil, fmt.Errorf("pré-carregamento Piper não concluiu: %v: %s", err, shortened(stderr.String(), 300))
	}
	w.starts.Add(1)
	return proc, nil
}

func (p *piperProcess) synthesize(ctx context.Context, text, output string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	// Interrupt a stalled synthesis by stopping the resident process. The next
	// request will start a new process; nothing remains in the background.
	stop := context.AfterFunc(ctx, func() {
		if p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
		}
	})
	defer stop()
	if err := json.NewEncoder(p.stdin).Encode(map[string]string{
		"voice": defaultVoiceProfile, "text": text, "output": output,
	}); err != nil {
		return fmt.Errorf("enviar frase ao Piper: %w", err)
	}
	var response struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := p.decoder.Decode(&response); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("Piper interrompido: %w", err)
	}
	if !response.OK {
		return errors.New(shortened("Piper: "+response.Error, 300))
	}
	return nil
}

func (w *piperWarmEngine) synthesize(ctx context.Context, text, output string) error {
	if len(text) == 0 || len(text) > 1200 || strings.TrimSpace(text) == "" {
		return errors.New("texto para fala inválido")
	}
	task := piperTask{ctx: ctx, text: text, output: output, done: make(chan error, 1)}
	select {
	case w.tasks <- task:
	case <-ctx.Done():
		return ctx.Err()
	case <-w.ctx.Done():
		return w.ctx.Err()
	}
	select {
	case err := <-task.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-w.ctx.Done():
		return w.ctx.Err()
	}
}

// Speech preview and streamed replies share the same preloaded model.
func (w *piperWarmEngine) speak(ctx context.Context, text string, _ voiceProfile) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	file, err := os.CreateTemp(w.profileDir, ".piper-preview-*.wav")
	if err != nil {
		return err
	}
	path := file.Name()
	defer os.Remove(path)
	if err := file.Close(); err != nil {
		return err
	}
	if err := w.synthesize(ctx, text, path); err != nil {
		return err
	}
	return playPiperWAV(ctx, path)
}

func (w *piperWarmEngine) speakStream(ctx context.Context, phrases <-chan string, _ voiceProfile) error {
	var first string
	var ok bool
	select {
	case first, ok = <-phrases:
		if !ok {
			return nil
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	clips, err := os.MkdirTemp(w.profileDir, ".piper-frases-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(clips)
	renderCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ready := make(chan string, 2)
	synthDone := make(chan error, 1)
	go func() {
		defer close(ready)
		phrase, index := first, 0
		for {
			path := filepath.Join(clips, fmt.Sprintf("frase-%05d.wav", index))
			if err := w.synthesize(renderCtx, phrase, path); err != nil {
				synthDone <- err
				return
			}
			select {
			case ready <- path:
			case <-renderCtx.Done():
				synthDone <- renderCtx.Err()
				return
			}
			index++
			select {
			case next, valid := <-phrases:
				if !valid {
					synthDone <- nil
					return
				}
				phrase = next
			case <-renderCtx.Done():
				synthDone <- renderCtx.Err()
				return
			}
		}
	}()
	var playErr error
	for wav := range ready {
		if playErr == nil && ctx.Err() == nil {
			playErr = playPiperWAV(ctx, wav)
			if playErr != nil {
				cancel()
			}
		}
		_ = os.Remove(wav)
	}
	synthErr := <-synthDone
	if err := ctx.Err(); err != nil {
		return err
	}
	if playErr != nil {
		return playErr
	}
	return synthErr
}
