package main

import (
    "bufio"
    "bytes"
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
)

// piperStreamVoice pre-generates upcoming sentence WAVs while Windows plays
// the preceding sentence. One Python interpreter and ONNX model are used per
// response, not per sentence. All temporary WAVs are deleted after playback.
func piperStreamVoice(profileDir string) func(context.Context, <-chan string, voiceProfile) error {
    return func(ctx context.Context, phrases <-chan string, voice voiceProfile) error {
        first, ok := <-phrases
        if !ok {
            return nil
        }
        dir, err := preparePiper(ctx, profileDir)
        if err != nil {
            return fmt.Errorf("preparar vozes Piper: %w", err)
        }
        clipsDir, err := os.MkdirTemp(profileDir, ".piper-frases-*")
        if err != nil {
            return err
        }
        defer os.RemoveAll(clipsDir)

        ready := make(chan string, 2)
        synthesized := make(chan error, 1)
        go func() {
            defer close(ready)
            synthesized <- synthesizePiperSequence(ctx, dir, clipsDir, voice.ID, first, phrases, ready)
        }()

        var playbackErr error
        for wav := range ready {
            if playbackErr == nil && ctx.Err() == nil {
                info, statErr := os.Stat(wav)
                if statErr != nil {
                    playbackErr = statErr
                } else if info.Size() > 44 {
                    if err := playPiperWAV(ctx, wav); err != nil {
                        playbackErr = fmt.Errorf("reproduzir voz %s: %w", voice.Name, err)
                    }
                }
            }
            _ = os.Remove(wav)
        }
        synthErr := <-synthesized
        if ctx.Err() != nil {
            return ctx.Err()
        }
        return errors.Join(synthErr, playbackErr)
    }
}

func synthesizePiperSequence(ctx context.Context, dir, clipsDir, voice, first string, phrases <-chan string, ready chan<- string) error {
    cmd := exec.CommandContext(ctx, filepath.Join(dir, "python.exe"), "-B", filepath.Join(dir, "synthesize.py"), "--serve", voice)
    configureProjectProcess(cmd)
    cmd.Dir = dir
    stdin, err := cmd.StdinPipe()
    if err != nil {
        return err
    }
    stdout, err := cmd.StdoutPipe()
    if err != nil {
        return err
    }
    var stderr bytes.Buffer
    cmd.Stderr = &stderr
    if err := cmd.Start(); err != nil {
        return err
    }
    finished := false
    defer func() {
        _ = stdin.Close()
        if !finished {
            _ = cmd.Process.Kill()
            _ = cmd.Wait()
        }
    }()
    encoder := json.NewEncoder(stdin)
    decoder := json.NewDecoder(bufio.NewReader(stdout))
    render := func(text string, index int) error {
        output := filepath.Join(clipsDir, fmt.Sprintf("clip-%05d.wav", index))
        if err := encoder.Encode(map[string]string{"text": text, "voice": voice, "output": output}); err != nil {
            return fmt.Errorf("enviar frase ao Piper: %w", err)
        }
        var response struct { OK bool; Error string }
        if err := decoder.Decode(&response); err != nil {
            if ctx.Err() != nil {
                return ctx.Err()
            }
            return fmt.Errorf("resposta inválida do Piper: %w", err)
        }
        if !response.OK {
            return fmt.Errorf("síntese Piper: %s", response.Error)
        }
        select {
        case ready <- output:
            return nil
        case <-ctx.Done():
            return ctx.Err()
        }
    }
    if err := render(first, 0); err != nil {
        return err
    }
    count := 1
    for phrase := range phrases {
        if ctx.Err() != nil {
            return ctx.Err()
        }
        if err := render(phrase, count); err != nil {
            return err
        }
        count++
    }
    _ = stdin.Close()
    err = cmd.Wait()
    finished = true
    if err != nil {
        return fmt.Errorf("Piper terminou com erro: %w: %s", err, shortened(stderr.String(), 300))
    }
    return nil
}
