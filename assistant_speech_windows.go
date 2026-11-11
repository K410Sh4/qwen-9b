package main

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

const speakScript = `$ErrorActionPreference='Stop'
[Console]::InputEncoding=[System.Text.Encoding]::UTF8
Add-Type -AssemblyName System.Speech
$s=New-Object System.Speech.Synthesis.SpeechSynthesizer
try {
 $voices=@($s.GetInstalledVoices() | Where-Object {$_.Enabled -and $_.VoiceInfo.Culture.TwoLetterISOLanguageName -eq 'pt'})
 if ($voices.Count -eq 0) { exit 11 }
 $br=@($voices | Where-Object {$_.VoiceInfo.Culture.Name -eq 'pt-BR'})
 if ($br.Count -gt 0) { $s.SelectVoice($br[0].VoiceInfo.Name) } else {$s.SelectVoice($voices[0].VoiceInfo.Name)}
 $s.Speak([Console]::In.ReadToEnd())
} finally {$s.Dispose()}`

const listenScript = `$ErrorActionPreference='Stop'
[Console]::OutputEncoding=[System.Text.UTF8Encoding]::new($false)
Add-Type -AssemblyName System.Speech
$recognizers=@([System.Speech.Recognition.SpeechRecognitionEngine]::InstalledRecognizers() | Where-Object {$_.Culture.TwoLetterISOLanguageName -eq 'pt'})
if ($recognizers.Count -eq 0) { exit 11 }
$br=@($recognizers | Where-Object {$_.Culture.Name -eq 'pt-BR'})
if ($br.Count -gt 0) { $recognizer=$br[0] } else {$recognizer=$recognizers[0]}
$r=[System.Speech.Recognition.SpeechRecognitionEngine]::new($recognizer)
try {
 $r.LoadGrammar((New-Object System.Speech.Recognition.DictationGrammar))
 $r.SetInputToDefaultAudioDevice()
 $result=$r.Recognize([TimeSpan]::FromSeconds(8))
 if ($null -ne $result) {[Console]::Write($result.Text)}
} finally {$r.Dispose()}`

func speechCommand(ctx context.Context, script, input string) (string, error) {
	root, err := windowsSystemDirectory()
	if err != nil {
		return "", err
	}
	cmdPath := filepath.Join(root, "WindowsPowerShell", "v1.0", "powershell.exe")
	encoded := utf16.Encode([]rune(script))
	data := make([]byte, 2*len(encoded))
	for i, c := range encoded {
		binary.LittleEndian.PutUint16(data[2*i:], c)
	}
	cmd := exec.CommandContext(ctx, cmdPath, "-NoLogo", "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(data))
	configureProjectProcess(cmd)
	cmd.Dir = root
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", errors.New("tempo da operação de voz esgotado")
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 11 {
			return "", errors.New("instale os componentes de fala em português nas configurações de idioma do Windows")
		}
		return "", errors.New("a fala do Windows está indisponível. Confira idioma e dispositivo de áudio")
	}
	return strings.TrimSpace(string(out)), nil
}
func speakWindows(ctx context.Context, text string) error {
	_, err := speechCommand(ctx, speakScript, text)
	return err
}
func listenWindows(ctx context.Context) (string, error) {
	text, err := speechCommand(ctx, listenScript, "")
	if err == nil && text == "" {
		return "", errors.New("não consegui entender a fala. Tente novamente perto do microfone")
	}
	return text, err
}
