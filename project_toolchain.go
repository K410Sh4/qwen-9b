package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const projectGo = "go1.27.4"
const projectArchiveSize = 79077744
const projectArchiveHash = "178ab98950ac69ab25069307dc6ade79f3816e414cce95b4da166a115cde1e7e"

func goTool(root string) string {
	name := "go"
	if runtime.GOOS == "windows" {
		name = "go.exe"
	}
	return filepath.Join(root, "bin", name)
}

func buildWithLocalGo(ctx context.Context, mission string, files []ProjectFile, plan ProjectPlan) ([]string, string, error) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		return nil, "", errors.New("o instalador do compilador exige Windows x64")
	}
	cache := filepath.Join(filepath.Dir(filepath.Dir(mission)), "compilador")
	root, err := ensureProjectGo(ctx, cache)
	if err != nil {
		return nil, "", err
	}
	return compileProject(ctx, root, mission, files, plan)
}

func ensureProjectGo(ctx context.Context, cache string) (string, error) {
	target := filepath.Join(cache, projectGo)
	if marker, err := os.ReadFile(filepath.Join(target, "sha256.txt")); err == nil && string(marker) == projectArchiveHash {
		root := filepath.Join(target, "go")
		if info, err := os.Stat(goTool(root)); err == nil && info.Mode().IsRegular() {
			return root, nil
		}
	}
	if err := os.MkdirAll(cache, 0700); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(cache, "instalacao-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	archive := filepath.Join(stage, "go.zip")
	if err := downloadProjectGo(ctx, archive); err != nil {
		return "", err
	}
	if err := extractProjectGo(ctx, archive, stage); err != nil {
		return "", err
	}
	if err := os.Remove(archive); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(stage, "sha256.txt"), []byte(projectArchiveHash), 0600); err != nil {
		return "", err
	}
	if err := os.RemoveAll(target); err != nil {
		return "", err
	}
	if err := os.Rename(stage, target); err != nil {
		return "", err
	}
	return filepath.Join(target, "go"), nil
}

func downloadProjectGo(ctx context.Context, file string) error {
	client := publicClient(func(u *url.URL) bool { return u.Hostname() == "dl.google.com" })
	client.Timeout = 4 * time.Minute
	defer client.CloseIdleConnections()
	resp, err := request(ctx, client, "https://dl.google.com/go/"+projectGo+".windows-amd64.zip")
	if err != nil {
		return fmt.Errorf("download do compilador oficial: %w", err)
	}
	defer resp.Body.Close()
	f, err := os.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	digest := sha256.New()
	n, writeErr := io.Copy(io.MultiWriter(f, digest), io.LimitReader(resp.Body, projectArchiveSize+1))
	if err := errors.Join(writeErr, f.Close()); err != nil {
		return err
	}
	if n != projectArchiveSize || hex.EncodeToString(digest.Sum(nil)) != projectArchiveHash {
		return errors.New("o tamanho ou SHA-256 do compilador difere do publicado")
	}
	return nil
}

func extractProjectGo(ctx context.Context, archive, dest string) error {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer z.Close()
	var total uint64
	for _, entry := range z.File {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		name := strings.TrimSuffix(entry.Name, "/")
		if name != "go" && (!strings.HasPrefix(name, "go/") || name != path.Clean(name) || strings.ContainsAny(name, `\:`)) {
			return errors.New("caminho inesperado no arquivo do compilador")
		}
		if entry.UncompressedSize64 > 80<<20 || total+entry.UncompressedSize64 > 512<<20 {
			return errors.New("pacote do compilador excede o limite de extração")
		}
		total += entry.UncompressedSize64
		target := filepath.Join(dest, filepath.FromSlash(name))
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0700); err != nil {
				return err
			}
			continue
		}
		if !entry.Mode().IsRegular() {
			return errors.New("entrada não regular no arquivo do compilador")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		if err := extractProjectFile(entry, target); err != nil {
			return err
		}
	}
	if _, err := os.Stat(filepath.Join(dest, "go", "bin", "go.exe")); err != nil {
		return err
	}
	return nil
}

func extractProjectFile(entry *zip.File, target string) error {
	r, err := entry.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		return err
	}
	n, writeErr := io.Copy(f, io.LimitReader(r, int64(entry.UncompressedSize64)+1))
	if err = errors.Join(writeErr, f.Close()); err != nil {
		return err
	}
	if uint64(n) != entry.UncompressedSize64 {
		return errors.New("tamanho inesperado na extração do compilador")
	}
	return nil
}
