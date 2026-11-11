package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestKokoroWindowsRuntime(t *testing.T) {
	archive := os.Getenv("JARVIS_KOKORO_ARCHIVE")
	if archive == "" {
		t.Skip("pacote Kokoro disponível na compilação de release")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	dir := filepath.Join(t.TempDir(), "Kokoro Vozes")
	if err := unpackKokoro(ctx, archive, dir); err != nil {
		t.Fatal(err)
	}
	if !kokoroInstalled(dir) {
		t.Fatal("runtime incompleto")
	}
	var previous []byte
	for _, voice := range voiceProfiles {
		output := filepath.Join(dir, voice.ID+".wav")
		cmd, err := kokoroCommand(ctx, dir, output, "Olá! Eu sou o Jarvis. Hoje vamos testar a pronúncia em português brasileiro.", voice.ID)
		if err != nil {
			t.Fatal(err)
		}
		if log, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", voice.Name, err, log)
		}
		audio, err := os.ReadFile(output)
		if err != nil {
			t.Fatal(err)
		}
		if len(audio) < 48044 || string(audio[:4]) != "RIFF" || string(audio[8:12]) != "WAVE" {
			t.Fatalf("%s: áudio inválido (%d bytes)", voice.Name, len(audio))
		}
		if bytes.Equal(previous, audio) {
			t.Fatalf("%s: amostras iguais às da outra voz", voice.Name)
		}
		t.Logf("%s: WAV gerado no Windows (%d bytes)", voice.Name, len(audio))
		previous = audio
	}
}


func TestKokoroPersistentSentenceSynthesis(t *testing.T) {
    archive := os.Getenv("JARVIS_KOKORO_ARCHIVE")
    if archive == "" {
        t.Skip("pacote Kokoro disponível na compilação de release")
    }
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    defer cancel()
    dir := filepath.Join(t.TempDir(), "runtime")
    if err := unpackKokoro(ctx, archive, dir); err != nil {
        t.Fatal(err)
    }
    clips := t.TempDir()
    phrases := make(chan string, 1)
    phrases <- "Esta é a segunda frase."
    close(phrases)
    ready := make(chan string, 2)
    result := make(chan error, 1)
    go func() {
        result <- synthesizeKokoroSequence(ctx, dir, clips, "pf_dora", "Esta é a primeira frase.", phrases, ready)
        close(ready)
    }()
    count := 0
    for path := range ready {
        audio, err := os.ReadFile(path)
        if err != nil {
            t.Fatal(err)
        }
        if len(audio) <= 44 || string(audio[:4]) != "RIFF" {
            t.Fatalf("WAV da frase %d inválido", count)
        }
        count++
    }
    if err := <-result; err != nil {
        t.Fatal(err)
    }
    if count != 2 {
        t.Fatalf("esperava duas frases renderizadas no mesmo processo; recebeu %d", count)
    }
}
