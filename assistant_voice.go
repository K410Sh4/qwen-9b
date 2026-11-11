package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const defaultVoiceProfile = "natural"

type voiceProfile struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Detail string `json:"detail"`
	Rate   int    `json:"rate"`
	Pitch  string `json:"pitch"`
}

var voiceProfiles = [...]voiceProfile{
	{ID: "natural", Name: "Natural", Detail: "Ritmo equilibrado", Rate: 0, Pitch: "medium"},
	{ID: "grave", Name: "Grave", Detail: "Tom mais baixo, fala pausada", Rate: -2, Pitch: "low"},
	{ID: "clara", Name: "Clara", Detail: "Tom mais alto, fala ágil", Rate: 2, Pitch: "high"},
}

func findVoiceProfile(id string) (voiceProfile, bool) {
	for _, profile := range voiceProfiles {
		if profile.ID == id {
			return profile, true
		}
	}
	return voiceProfile{}, false
}

func readVoiceProfile(path string) string {
	if path != "" {
		if data, err := os.ReadFile(path); err == nil {
			if profile, ok := findVoiceProfile(strings.TrimSpace(string(data))); ok {
				return profile.ID
			}
		}
	}
	return defaultVoiceProfile
}

func saveVoiceProfile(path, id string) error {
	if path == "" {
		return nil
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".voz-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.WriteString(id + "\n")
	if err = errors.Join(writeErr, file.Close()); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func (a *assistantDesk) selectVoiceProfile(id string) error {
	profile, ok := findVoiceProfile(id)
	if !ok {
		return errors.New("perfil de voz desconhecido")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if profile.ID == a.state.VoiceProfile {
		return nil
	}
	if err := saveVoiceProfile(a.voiceFile, profile.ID); err != nil {
		return fmt.Errorf("não foi possível salvar a voz: %w", err)
	}
	a.state.VoiceProfile = profile.ID
	a.state.Revision++
	return nil
}

func (a *assistantDesk) previewVoice() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.speak == nil {
		return errors.New("a síntese de voz está indisponível")
	}
	if a.state.Activity != "ready" {
		return errors.New("aguarde o Jarvis terminar a atividade atual")
	}
	a.state.Error = ""
	profile, _ := findVoiceProfile(a.state.VoiceProfile)
	a.startSpeechLocked("Olá! Eu sou o Jarvis. Você está ouvindo a voz " + profile.Name + ".")
	return nil
}
