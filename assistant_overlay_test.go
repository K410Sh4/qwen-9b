package main

import (
    "encoding/base64"
    "crypto/sha256"
    "fmt"
    "strings"
    "testing"
)

func TestOverlayAvatarIsEmbeddedOffline(t *testing.T) {
    data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(avatarBase64))
    if err != nil { t.Fatal(err) }
    if len(data)<2000 || string(data[:4])!="RIFF" || string(data[8:12])!="WEBP" {
        t.Fatal("avatar WebP não foi incorporado corretamente")
    }
    hash := fmt.Sprintf("%x", sha256.Sum256(data))
    if hash!="a0c9b85371162bc86520fdcdc9b318ce1a9f691c29f248475b07884bd4dd9cbf" {
        t.Fatalf("avatar alterado no transporte: SHA-256 %s", hash)
    }
    for _, required := range []string{"__JARVIS_AVATAR__", "__JARVIS_BRIDGE_TOKEN__", "id=\"avatarButton\"", "id=\"chat\"", "id=\"mic\"", "id=\"quit\""} {
        if !strings.Contains(assistantHTML,required) { t.Errorf("overlay sem %s",required) }
    }
}

func TestOverlayDoesNotExposeOldWorkspace(t *testing.T) {
    if strings.Contains(assistantHTML, "Iniciar sessão") || strings.Contains(assistantHTML, "Aprovar missão") || strings.Contains(assistantHTML,"id=\"newProject\"") {
        t.Fatal("a interface antiga ainda está no painel padrão")
    }
}
