package main

import (
    "encoding/json"
    "strings"
    "testing"
)

func TestOverlayUsesCanonicalFaceMeshOffline(t *testing.T) {
    var mesh struct {
        Positions []float64 `json:"positions"`
        Triangles []int `json:"triangles"`
    }
    if err := json.Unmarshal([]byte(faceMeshJSON), &mesh); err != nil {
        t.Fatal(err)
    }
    if len(mesh.Positions) != 468*3 || len(mesh.Triangles) != 898*3 {
        t.Fatalf("topologia facial inválida: %d posições, %d índices", len(mesh.Positions), len(mesh.Triangles))
    }
    for _, index := range mesh.Triangles {
        if index < 0 || index >= 468 {
            t.Fatalf("índice de vértice fora dos limites: %d", index)
        }
    }
    if len(faceMeshRenderer) < 3000 {
        t.Fatal("renderizador facial ausente")
    }
    for _, required := range []string{"__JARVIS_FACE_DATA__", "__JARVIS_FACE_RENDERER__",
        "__JARVIS_BRIDGE_TOKEN__", `id="faceMesh"`, `id="avatarButton"`,
        `id="chat"`, `id="mic"`, `id="quit"`, "jarvisFaceMeshReady"} {
        if !strings.Contains(assistantHTML, required) {
            t.Errorf("overlay sem %s", required)
        }
    }
    if strings.Contains(assistantHTML, "data:image/webp") || strings.Contains(assistantHTML, "__JARVIS_AVATAR__") {
        t.Fatal("avatar fotográfico antigo ainda está embutido no HTML")
    }
}

func TestOverlayDoesNotExposeOldWorkspace(t *testing.T) {
    if strings.Contains(assistantHTML, "Iniciar sessão") || strings.Contains(assistantHTML, "Aprovar missão") || strings.Contains(assistantHTML, `id="newProject"`) {
        t.Fatal("a interface antiga ainda está no painel padrão")
    }
}
