package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const kokoroAsset = "JarvisKokoro-win-x64.zip"
const kokoroDownloadLimit = 256 << 20
const kokoroExtractionLimit = 512 << 20

// Set by the Windows release workflow after testing the runtime.
var kokoroPackageDigest string

const playKokoroScript = `$ErrorActionPreference='Stop'
[Console]::InputEncoding=[System.Text.UTF8Encoding]::new($false)
$player=[System.Media.SoundPlayer]::new([Console]::In.ReadToEnd())
try { $player.PlaySync() } finally { $player.Dispose() }`

func kokoroVoice(profileDir string) func(context.Context, string, voiceProfile) error {
	return func(ctx context.Context, text string, voice voiceProfile) error {
		if strings.TrimSpace(text) == "" {
			return nil
		}
		dir, err := prepareKokoro(ctx, profileDir)
		if err != nil {
			return fmt.Errorf("preparar vozes Kokoro: %w", err)
		}
		wav, err := os.CreateTemp(profileDir, ".kokoro-*.wav")
		if err != nil {
			return err
		}
		defer os.Remove(wav.Name())
		if err := wav.Close(); err != nil {
			return err
		}
		cmd, err := kokoroCommand(ctx, dir, wav.Name(), text, voice.ID)
		if err != nil {
			return err
		}
		if err := cmd.Run(); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("não foi possível gerar a voz %s: %w", voice.Name, err)
		}
		info, err := os.Stat(wav.Name())
		if err != nil {
			return err
		}
		if info.Size() <= 44 {
			return nil
		}
		if _, err := speechCommand(ctx, playKokoroScript, wav.Name()); err != nil {
			return fmt.Errorf("não foi possível reproduzir o áudio de %s: %w", voice.Name, err)
		}
		return nil
	}
}

func kokoroCommand(ctx context.Context, dir, output, text, voice string) (*exec.Cmd, error) {
	input, err := json.Marshal(map[string]string{"text": text, "voice": voice})
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, filepath.Join(dir, "python.exe"), "-B", filepath.Join(dir, "synthesize.py"), output)
	configureProjectProcess(cmd)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(string(input))
	return cmd, nil
}

func prepareKokoro(ctx context.Context, profileDir string) (string, error) {
	digest, err := hex.DecodeString(kokoroPackageDigest)
	if err != nil || len(digest) != sha256.Size {
		return "", errors.New("esta compilação precisa do pacote de voz verificado da versão oficial")
	}
	root := filepath.Join(profileDir, "Kokoro")
	dir := filepath.Join(root, kokoroPackageDigest[:16])
	if kokoroInstalled(dir) {
		return dir, nil
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(root, ".instalando-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	archive := filepath.Join(stage, kokoroAsset)
	if err := downloadKokoro(ctx, archive); err != nil {
		return "", err
	}
	payload := filepath.Join(stage, "runtime")
	if err := unpackKokoro(ctx, archive, payload); err != nil {
		return "", err
	}
	if !kokoroInstalled(payload) {
		return "", errors.New("pacote de vozes incompleto")
	}
	if err := os.Rename(payload, dir); err != nil && !kokoroInstalled(dir) {
		return "", err
	}
	return dir, nil
}

func kokoroInstalled(dir string) bool {
	for _, name := range []string{"python.exe", "synthesize.py", "kokoro.onnx", "voices.bin"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return false
		}
	}
	return true
}

func downloadKokoro(ctx context.Context, path string) error {
	client := publicClient(officialHost)
	// Use the user's proxy settings for the official release download.
	client.Transport = http.DefaultTransport
	client.Timeout = 6 * time.Minute
	url := "https://github.com/" + repository + "/releases/download/" + Version + "/" + kokoroAsset
	resp, err := request(ctx, client, url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.ContentLength > kokoroDownloadLimit {
		return errors.New("pacote de voz excede o limite de tamanho")
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(resp.Body, kokoroDownloadLimit+1))
	if err := errors.Join(copyErr, file.Close()); err != nil {
		return err
	}
	if size > kokoroDownloadLimit || hex.EncodeToString(hash.Sum(nil)) != kokoroPackageDigest {
		return errors.New("SHA-256 do pacote Kokoro não confere")
	}
	return nil
}

func unpackKokoro(ctx context.Context, archive, dir string) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer reader.Close()
	var total uint64
	for _, entry := range reader.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		name, err := filepath.Localize(entry.Name)
		if err != nil || !entry.Mode().IsRegular() {
			return fmt.Errorf("entrada inválida no pacote Kokoro: %q", entry.Name)
		}
		if entry.UncompressedSize64 > kokoroExtractionLimit-total {
			return errors.New("pacote Kokoro excede o limite de extração")
		}
		total += entry.UncompressedSize64
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		if err := extractKokoroFile(entry, path); err != nil {
			return err
		}
	}
	return nil
}

func extractKokoroFile(entry *zip.File, path string) error {
	source, err := entry.Open()
	if err != nil {
		return err
	}
	defer source.Close()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, source)
	return errors.Join(copyErr, file.Close())
}
