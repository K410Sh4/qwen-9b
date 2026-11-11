package main

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

const speakScript = `$ErrorActionPreference='Stop'
[Console]::InputEncoding=[System.Text.UTF8Encoding]::new($false)
$request=ConvertFrom-Json -InputObject ([Console]::In.ReadToEnd())
Add-Type -AssemblyName System.Speech
$s=New-Object System.Speech.Synthesis.SpeechSynthesizer
try {
 $voices=@($s.GetInstalledVoices() | Where-Object {$_.Enabled -and $_.VoiceInfo.Culture.TwoLetterISOLanguageName -eq 'pt'} | ForEach-Object {$_.VoiceInfo} | Sort-Object @{Expression={$_.Culture.Name -ne 'pt-BR'}},Name)
 if ($voices.Count -eq 0) { exit 11 }
 $natural=$voices[0]
 $other=@($voices | Where-Object {$_.Name -ne $natural.Name})
 $grave=$natural
 if ($other.Count -gt 0) {
  $male=@($other | Where-Object {$_.Gender -eq 'Male'})
  if ($male.Count -gt 0) {$grave=$male[0]} else {$grave=$other[0]}
 }
 $clear=$natural
 $unused=@($other | Where-Object {$_.Name -ne $grave.Name})
 if ($unused.Count -gt 0) {
  $female=@($unused | Where-Object {$_.Gender -eq 'Female'})
  if ($female.Count -gt 0) {$clear=$female[0]} else {$clear=$unused[0]}
 }
 $selected=switch ($request.profile.id) {
  'grave' {$grave}
  'clara' {$clear}
  default {$natural}
 }
 $s.SelectVoice($selected.Name)
 $s.Rate=[int]$request.profile.rate
 $text=[string]$request.text
 if ($request.profile.pitch -eq 'medium') {
  $s.Speak($text)
 } else {
  $text=[System.Security.SecurityElement]::Escape($text)
  $ssml='<speak version="1.0" xmlns="http://www.w3.org/2001/10/synthesis" xml:lang="{0}"><prosody pitch="{1}">{2}</prosody></speak>' -f $selected.Culture.Name,$request.profile.pitch,$text
  $s.SpeakSsml($ssml)
 }
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
func speakWindows(ctx context.Context, text string, profile voiceProfile) error {
	data, err := json.Marshal(struct {
		Text    string       `json:"text"`
		Profile voiceProfile `json:"profile"`
	}{text, profile})
	if err != nil {
		return err
	}
	_, err = speechCommand(ctx, speakScript, string(data))
	return err
}
func listenWindows(ctx context.Context) (string, error) {
	text, err := speechCommand(ctx, listenScript, "")
	if err == nil && text == "" {
		return "", errors.New("não consegui entender a fala. Tente novamente perto do microfone")
	}
	return text, err
}
