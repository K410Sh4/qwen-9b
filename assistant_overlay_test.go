package main

import (
    "encoding/base64"
    "strings"
    "testing"
)

func TestOverlayAvatarIsEmbeddedOffline(t *testing.T) {
    data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(avatarBase64))
    if err != nil { t.Fatal(err) }
    if len(data)<2000 || string(data[:4])!="RIFF" || string(data[8:12])!="WEBP" {
        t.Fatal("avatar WebP não foi incorporado corretamente")
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
