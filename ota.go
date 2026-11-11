package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const Version = "v0.1.1"
const repository = "K410Sh4/qwen-9b"
const executableName = "JarvisLocal.exe"

type Release struct {
	Tag    string `json:"tag_name"`
	Assets []struct {
		Name   string
		URL    string `json:"browser_download_url"`
		Digest string
		Size   int64
	}
}

type Update struct {
	Version, URL, Hash string
	Size               int64
}

var versionPattern = regexp.MustCompile(`^v(0|[1-9][0-9]{0,6})\.(0|[1-9][0-9]{0,6})\.(0|[1-9][0-9]{0,6})$`)

func newer(tag, installed string) bool {
	a, b := versionPattern.FindStringSubmatch(tag), versionPattern.FindStringSubmatch(installed)
	if a == nil || b == nil {
		return false
	}
	for i := 1; i <= 3; i++ {
		x, _ := strconv.Atoi(a[i])
		y, _ := strconv.Atoi(b[i])
		if x != y {
			return x > y
		}
	}
	return false
}

func officialHost(u *url.URL) bool {
	switch strings.ToLower(u.Hostname()) {
	case "api.github.com", "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com":
		return true
	}
	return false
}

func findUpdate(ctx context.Context) (*Update, error) {
	client := publicClient(officialHost)
	defer client.CloseIdleConnections()
	resp, err := request(ctx, client, "https://api.github.com/repos/"+repository+"/releases/latest")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := readLimited(resp.Body, 1<<20)
	if err != nil {
		return nil, err
	}
	var release Release
	if err = json.Unmarshal(body, &release); err != nil {
		return nil, err
	}
	if !newer(release.Tag, Version) {
		return nil, nil
	}
	expected := "https://github.com/" + repository + "/releases/download/" + release.Tag + "/" + executableName
	for _, asset := range release.Assets {
		if asset.Name != executableName || asset.URL != expected {
			continue
		}
		digest := strings.TrimPrefix(asset.Digest, "sha256:")
		if _, err = hex.DecodeString(digest); err != nil || len(digest) != 64 || !strings.HasPrefix(asset.Digest, "sha256:") || asset.Size < 1024 || asset.Size > 20<<20 {
			return nil, errors.New("arquivo da release sem tamanho ou digest válido")
		}
		return &Update{release.Tag, asset.URL, strings.ToLower(digest), asset.Size}, nil
	}
	return nil, errors.New("executável ausente na release")
}

func stageUpdate(ctx context.Context, u Update) error {
	client := publicClient(officialHost)
	defer client.CloseIdleConnections()
	resp, err := request(ctx, client, u.URL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := readLimited(resp.Body, 20<<20)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(body)
	if len(body) < 2 || int64(len(body)) != u.Size || hex.EncodeToString(sum[:]) != u.Hash || string(body[:2]) != "MZ" {
		return errors.New("o executável baixado falhou na verificação SHA-256 ou no formato")
	}
	target, err := os.Executable()
	if err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(target), "jarvis-ota-")
	if err != nil {
		return err
	}
	started := false
	defer func() {
		if !started {
			_ = os.RemoveAll(stage)
		}
	}()
	if err = os.WriteFile(filepath.Join(stage, "novo.exe"), body, 0700); err != nil {
		return err
	}
	old, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	helper := filepath.Join(stage, "aplicar.exe")
	if err = os.WriteFile(helper, old, 0700); err != nil {
		return err
	}
	cmd := exec.Command(helper, "--apply-update", strconv.Itoa(os.Getpid()), target, u.Hash)
	if err = cmd.Start(); err != nil {
		return err
	}
	started = true
	return cmd.Process.Release()
}

func applyUpdate(args []string, wait func(int) error) error {
	if len(args) != 3 {
		return errors.New("argumentos de atualização inválidos")
	}
	pid, err := strconv.Atoi(args[0])
	if err != nil || pid <= 0 {
		return errors.New("PID inválido")
	}
	target := filepath.Clean(args[1])
	self, err := os.Executable()
	if err != nil {
		return err
	}
	stage := filepath.Dir(self)
	if !strings.HasPrefix(filepath.Base(stage), "jarvis-ota-") || !strings.EqualFold(filepath.Dir(stage), filepath.Dir(target)) || strings.EqualFold(target, self) {
		return errors.New("pasta de atualização inválida")
	}
	pending := filepath.Join(stage, "novo.exe")
	backup := filepath.Join(stage, "anterior.exe")
	marker := filepath.Join(stage, "pronto.txt")
	body, err := os.ReadFile(pending)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != args[2] {
		return errors.New("a atualização mudou após o download")
	}
	if err = wait(pid); err != nil {
		return err
	}
	if err = os.Rename(target, backup); err != nil {
		return err
	}
	if err = os.Rename(pending, target); err != nil {
		return errors.Join(err, os.Rename(backup, target))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	probeErr := exec.CommandContext(ctx, target, "--smoke", marker).Run()
	probe, readErr := os.ReadFile(marker)
	if probeErr != nil || readErr != nil || !strings.HasPrefix(string(probe), "Janela Win32 criada") {
		_ = os.Remove(target)
		if err = os.Rename(backup, target); err != nil {
			return fmt.Errorf("falha no rollback. Versão anterior em %s: %w", backup, err)
		}
		_ = exec.Command(target).Start()
		return errors.New("a versão nova falhou no teste de inicialização. Versão anterior restaurada")
	}
	if err = exec.Command(target).Start(); err != nil {
		_ = os.Remove(target)
		if rollbackErr := os.Rename(backup, target); rollbackErr != nil {
			return errors.Join(err, rollbackErr)
		}
		return err
	}
	_ = os.Remove(backup)
	_ = os.Remove(marker)
	return nil
}
