package main

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func waitPiperWarm(t *testing.T, worker *piperWarmEngine) {
	t.Helper()
	select {
	case <-worker.ready:
		worker.mu.RLock()
		err := worker.preloadErr
		worker.mu.RUnlock()
		if err != nil {
			t.Fatalf("preload da voz Faber: %v", err)
		}
	case <-time.After(90 * time.Second):
		t.Fatal("pré-carregamento da voz Faber excedeu o prazo")
	}
}

func TestPiperStartsWarmupWithoutSpeech(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	w := newPiperWarmWithInstaller(ctx, t.TempDir(), func(ctx context.Context) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("o pré-carregamento não foi iniciado ao abrir o Jarvis")
	}
	cancel()
	select {
	case <-w.stopped:
	case <-time.After(time.Second):
		t.Fatal("o worker de voz continuou ativo após o encerramento")
	}
}

func TestPiperWarmFaberRuntimeAndReuse(t *testing.T) {
	archive := os.Getenv("JARVIS_PIPER_ARCHIVE")
	if archive == "" {
		t.Skip("runtime Piper fornecido pelo GitHub Actions")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	dir := filepath.Join(t.TempDir(), "Piper Faber")
	if err := unpackPiper(ctx, archive, dir); err != nil {
		t.Fatal(err)
	}
	if !piperInstalled(dir) {
		t.Fatal("pacote Faber incompleto")
	}
	if _, err := os.Stat(filepath.Join(dir, "pt_BR-edresson-low.onnx")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("o arquivo da voz Edresson não deveria estar no pacote")
	}
	w := newPiperWarmWithInstaller(ctx, dir, func(context.Context) (string, error) {
		return dir, nil
	})
	started := time.Now()
	waitPiperWarm(t, w)
	t.Logf("Faber carregado antecipadamente em %v", time.Since(started))
	for n, text := range []string{
		"Olá! Eu sou o Jarvis. Este áudio usa somente a voz Faber.",
		"Esta é a segunda frase, utilizando a mesma sessão ONNX.",
	} {
		wav := filepath.Join(dir, "fala-"+string(rune('0'+n))+".wav")
		begin := time.Now()
		if err := w.synthesize(ctx, text, wav); err != nil {
			t.Fatalf("frase %d: %v", n, err)
		}
		latency := time.Since(begin)
		audio, err := os.ReadFile(wav)
		if err != nil {
			t.Fatal(err)
		}
		if len(audio) <= 2048 || string(audio[:4]) != "RIFF" || string(audio[8:12]) != "WAVE" {
			t.Fatalf("frase %d: WAV inválido", n)
		}
		if rate := binary.LittleEndian.Uint32(audio[24:28]); rate != 22050 {
			t.Fatalf("sample rate incorreta: %d", rate)
		}
		t.Logf("Faber pré-carregada: frase %d em %v", n+1, latency)
	}
	if got := w.starts.Load(); got != 1 {
		t.Fatalf("o modelo foi carregado %d vezes; esperado 1", got)
	}
	cancel()
	select {
	case <-w.stopped:
	case <-time.After(4*time.Second):
		t.Fatal("o processo Piper não encerrou")
	}
}

func TestPiperWarmCancelledSynthesis(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	w := newPiperWarmWithInstaller(ctx, t.TempDir(), func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})
	requestCtx, stop := context.WithCancel(ctx)
	stop()
	if err := w.synthesize(requestCtx, "Teste.", filepath.Join(t.TempDir(), "unused.wav")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelamento incorreto: %v", err)
	}
	cancel()
	<-w.stopped
}
