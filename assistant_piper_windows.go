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

const piperAsset = "JarvisPiper-win-x64.zip"
const piperDownloadLimit = 300 << 20
const piperExtractionLimit = 700 << 20

// Set by the Windows release workflow after testing the runtime.
var piperPackageDigest string

func piperVoice(profileDir string) func(context.Context, string, voiceProfile) error {
	return func(ctx context.Context, text string, voice voiceProfile) error {
		if strings.TrimSpace(text) == "" {
			return nil
		}
		dir, err := preparePiper(ctx, profileDir)
		if err != nil {
			return fmt.Errorf("preparar vozes Piper: %w", err)
		}
		wav, err := os.CreateTemp(profileDir, ".piper-*.wav")
		if err != nil {
			return err
		}
		defer os.Remove(wav.Name())
		if err := wav.Close(); err != nil {
			return err
		}
		cmd, err := piperCommand(ctx, dir, wav.Name(), text, voice.ID)
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
		if err := playPiperWAV(ctx, wav.Name()); err != nil {
			return fmt.Errorf("não foi possível reproduzir o áudio de %s: %w", voice.Name, err)
		}
		return nil
	}
}

func piperCommand(ctx context.Context, dir, output, text, voice string) (*exec.Cmd, error) {
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

func preparePiper(ctx context.Context, profileDir string) (string, error) {
	digest, err := hex.DecodeString(piperPackageDigest)
	if err != nil || len(digest) != sha256.Size {
		return "", errors.New("esta compilação precisa do pacote de voz verificado da versão oficial")
	}
	root := filepath.Join(profileDir, "Piper")
	dir := filepath.Join(root, piperPackageDigest[:16])
	if piperInstalled(dir) {
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
	archive := filepath.Join(stage, piperAsset)
	if err := downloadPiper(ctx, archive); err != nil {
		return "", err
	}
	payload := filepath.Join(stage, "runtime")
	if err := unpackPiper(ctx, archive, payload); err != nil {
		return "", err
	}
	if !piperInstalled(payload) {
		return "", errors.New("pacote de vozes incompleto")
	}
	if err := os.Rename(payload, dir); err != nil && !piperInstalled(dir) {
		return "", err
	}
	return dir, nil
}

func piperInstalled(dir string) bool {
	for _, name := range []string{"python.exe", "synthesize.py", "pt_BR-edresson-low.onnx", "pt_BR-edresson-low.onnx.json", "pt_BR-faber-medium.onnx", "pt_BR-faber-medium.onnx.json"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return false
		}
	}
	return true
}

func downloadPiper(ctx context.Context, path string) error {
	client := publicClient(officialHost)
	// Use the user's proxy settings for the official release download.
	client.Transport = http.DefaultTransport
	client.Timeout = 6 * time.Minute
	url := "https://github.com/" + repository + "/releases/download/" + Version + "/" + piperAsset
	resp, err := request(ctx, client, url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.ContentLength > piperDownloadLimit {
		return errors.New("pacote de voz excede o limite de tamanho")
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(resp.Body, piperDownloadLimit+1))
	if err := errors.Join(copyErr, file.Close()); err != nil {
		return err
	}
	if size > piperDownloadLimit || hex.EncodeToString(hash.Sum(nil)) != piperPackageDigest {
		return errors.New("SHA-256 do pacote Piper não confere")
	}
	return nil
}

func unpackPiper(ctx context.Context, archive, dir string) error {
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
			return fmt.Errorf("entrada inválida no pacote Piper: %q", entry.Name)
		}
		if entry.UncompressedSize64 > piperExtractionLimit-total {
			return errors.New("pacote Piper excede o limite de extração")
		}
		total += entry.UncompressedSize64
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		if err := extractPiperFile(entry, path); err != nil {
			return err
		}
	}
	return nil
}

func extractPiperFile(entry *zip.File, path string) error {
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
