package main

import (
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"github.com/jchv/go-webview2/pkg/edge"
	"github.com/jchv/go-webview2/webviewloader"
)

//go:embed assistant.html
var assistantHTML string

//go:embed avatar-mesh.json
var faceMeshJSON string

//go:embed avatar-mesh-renderer.js
var faceMeshRenderer string

var (
	notifyIcon         = shell.NewProc("Shell_NotifyIconW")
	registerHotkey     = user.NewProc("RegisterHotKey")
	unregisterHotkey   = user.NewProc("UnregisterHotKey")
	setForeground      = user.NewProc("SetForegroundWindow")
	isVisible          = user.NewProc("IsWindowVisible")
	getCursorPos       = user.NewProc("GetCursorPos")
	getWindowRect      = user.NewProc("GetWindowRect")
	loadIcon           = user.NewProc("LoadIconW")
	systemParameters   = user.NewProc("SystemParametersInfoW")
	makeMenu           = user.NewProc("CreatePopupMenu")
	appendMenu         = user.NewProc("AppendMenuW")
	trackMenu          = user.NewProc("TrackPopupMenu")
	destroyMenu        = user.NewProc("DestroyMenu")
	releaseCapture     = user.NewProc("ReleaseCapture")
	postMessage        = user.NewProc("PostMessageW")
	getSystemDirectory = kernel.NewProc("GetSystemDirectoryW")
	roundWindow        = syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
	getKeyState        = user.NewProc("GetKeyState")
	setLayeredAttrs    = user.NewProc("SetLayeredWindowAttributes")
	setWindowPos       = user.NewProc("SetWindowPos")
)

const (
	vkF4             = 0x73
	shellTrayMessage = 0x8011
	shellDragMessage = 0x8013
	shellHotkeyID    = 0x4a
	overlaySmallW    = 278
	overlaySmallH    = 374
	overlayWideW     = 716
	overlayWideH     = 548
)

type winRect struct{ Left, Top, Right, Bottom int32 }
type notifyData struct {
	Size               uint32
	Window             uintptr
	ID, Flags, Message uint32
	Icon               uintptr
	Tip                [128]uint16
	State, StateMask   uint32
	Info               [256]uint16
	Version            uint32
	InfoTitle          [64]uint16
	InfoFlags          uint32
	GUID               [16]byte
	BalloonIcon        uintptr
}

type assistantShell struct {
	handle       uintptr
	work         *Window
	browser      *edge.Chromium
	desk         *assistantDesk
	dev          *devAgent
	token, smoke string
	tray, hotkey bool
	expanded     bool
}

var activeShell *assistantShell

func newAssistantShell(w *Window, smoke string) (*assistantShell, error) {
	version, err := webviewloader.GetInstalledVersion()
	if err != nil || version == "" {
		return nil, errors.New("Microsoft Edge WebView2 indisponível")
	}
	data := make([]byte, 16)
	if _, err = rand.Read(data); err != nil {
		return nil, err
	}
	// Warm the single Piper model once at startup. Download and model load happen
	// asynchronously; the WebView2 and text chat stay responsive meanwhile.
	voiceEngine := newPiperWarmEngine(w.ctx, w.missions.profile)
	s := &assistantShell{work: w, desk: newAssistant(voiceEngine.speak, listenWindows, filepath.Join(w.missions.profile, "voz.txt")), token: hex.EncodeToString(data), smoke: smoke}
	s.desk.speakStream = voiceEngine.speakStream
	if smoke == "" {
		s.dev = newDevAgent(w.ctx, filepath.Join(w.missions.profile, "DevAgent"))
	}
	name, title := wide("JarvisAssistantPanel"), wide("Jarvis Local "+Version)
	cursor, _, _ := loadCursor.Call(0, 32512)
	class := windowClass{Proc: syscall.NewCallback(shellProc), Instance: w.instance, Cursor: cursor, Background: 0, Name: name}
	class.Size = uint32(unsafe.Sizeof(class))
	if n, _, e := register.Call(uintptr(unsafe.Pointer(&class))); n == 0 {
		return nil, e
	}
	var area winRect
	if ok, _, _ := systemParameters.Call(0x30, 0, uintptr(unsafe.Pointer(&area)), 0); ok == 0 {
		area = winRect{Right: 1280, Bottom: 800}
	}
	width, height := min(overlaySmallW, int(area.Right-area.Left)-24), min(overlaySmallH, int(area.Bottom-area.Top)-24)
	x, y := int(area.Right)-width-12, int(area.Bottom)-height-12
	activeShell = s
	// Tool-window + layered + topmost; the compact host covers only the avatar, not the desktop.
	hwnd, _, e := createWindow.Call(0x00080088, pointer(name), pointer(title), 0x80000000, uintptr(x), uintptr(y), uintptr(width), uintptr(height), 0, 0, w.instance, 0)
	runtime.KeepAlive(name)
	runtime.KeepAlive(title)
	if hwnd == 0 {
		activeShell = nil
		return nil, e
	}
	s.handle = hwnd
	if ok, _, last := setLayeredAttrs.Call(hwnd, 0, 255, 0x2); ok == 0 {
		activeShell = nil
		closeWindow.Call(hwnd)
		return nil, fmt.Errorf("não foi possível configurar a transparência: %v", last)
	}
	// Setting alpha before WebView2 creation also prevents a white loading flash.
	_ = os.Setenv("WEBVIEW2_DEFAULT_BACKGROUND_COLOR", "00000000")
	s.browser = edge.NewChromium()
	s.browser.DataPath = filepath.Join(w.missions.profile, "PainelWeb")
	s.browser.SetGlobalPermission(edge.CoreWebView2PermissionStateDeny)
	s.browser.MessageCallback = s.handleMessage
	s.browser.AcceleratorKeyCallback = func(key uint) bool {
		if key != vkF4 {
			return false
		}
		alt, _, _ := getKeyState.Call(0x12)
		if alt&0x8000 == 0 {
			return false
		}
		postMessage.Call(hwnd, wmClose, 0, 0)
		return true
	}
	if !s.browser.Embed(hwnd) {
		activeShell = nil
		closeWindow.Call(hwnd)
		return nil, errors.New("o WebView2 não iniciou")
	}
	settings, err := s.browser.GetSettings()
	if err != nil {
		activeShell = nil
		closeWindow.Call(hwnd)
		return nil, err
	}
	_ = settings.PutAreDevToolsEnabled(false)
	_ = settings.PutAreDefaultContextMenusEnabled(false)
	_ = settings.PutAreHostObjectsAllowed(false)
	_ = settings.PutIsStatusBarEnabled(false)
	_ = settings.PutAreBrowserAcceleratorKeysEnabled(false)
	_ = settings.PutIsSwipeNavigationEnabled(false)
	_ = settings.PutAreDefaultScriptDialogsEnabled(false)
	if roundWindow.Find() == nil {
		corner := uint32(2)
		_, _, _ = roundWindow.Call(hwnd, 33, uintptr(unsafe.Pointer(&corner)), 4)
	}
	if smoke == "" {
		s.tray = s.icon(0)
		hit, _, _ := registerHotkey.Call(hwnd, shellHotkeyID, 0x4003, 'J')
		s.hotkey = hit != 0
	}
	s.browser.Resize()
	markup := strings.Replace(assistantHTML, "__JARVIS_BRIDGE_TOKEN__", s.token, 1)
	markup = strings.Replace(markup, "__JARVIS_FACE_DATA__", faceMeshJSON, 1)
	markup = strings.Replace(markup, "__JARVIS_FACE_RENDERER__", faceMeshRenderer, 1)
	s.browser.NavigateToString(markup)
	s.show()
	return s, nil
}

func (s *assistantShell) setExpanded(expand bool) error {
    if s.handle == 0 || s.expanded == expand {
        return nil
    }
    var current winRect
    if ok, _, _ := getWindowRect.Call(s.handle, uintptr(unsafe.Pointer(&current))); ok == 0 {
        return errors.New("não foi possível posicionar o avatar")
    }
    var workArea winRect
    if ok, _, _ := systemParameters.Call(0x30, 0, uintptr(unsafe.Pointer(&workArea)), 0); ok == 0 {
        workArea = winRect{Left: 0, Top: 0, Right: 1280, Bottom: 800}
    }
    width, height := overlaySmallW, overlaySmallH
    if expand {
        width, height = overlayWideW, overlayWideH
    }
    width = min(width, int(workArea.Right-workArea.Left)-12)
    height = min(height, int(workArea.Bottom-workArea.Top)-12)
    x, y := int(current.Right)-width, int(current.Bottom)-height
    x = max(int(workArea.Left)+4, min(x, int(workArea.Right)-width-4))
    y = max(int(workArea.Top)+4, min(y, int(workArea.Bottom)-height-4))
    if ok, _, err := setWindowPos.Call(s.handle, 0, uintptr(x), uintptr(y), uintptr(width), uintptr(height), 0x14); ok == 0 {
        return fmt.Errorf("não foi possível redimensionar o avatar: %v", err)
    }
    s.expanded = expand
    s.browser.Resize()
    return nil
}

func (s *assistantShell) show() {
	show.Call(s.handle, 9)
	setForeground.Call(s.handle)
	_ = s.browser.Show()
	s.browser.Focus()
}
func (s *assistantShell) hide() {
	if s.tray || s.hotkey {
		_ = s.browser.Hide()
		show.Call(s.handle, 0)
	} else {
		show.Call(s.handle, 6)
	}
}
func (s *assistantShell) toggle() {
	visible, _, _ := isVisible.Call(s.handle)
	if visible != 0 {
		s.hide()
	} else {
		s.show()
	}
}

func (s *assistantShell) icon(operation uintptr) bool {
	icon, _, _ := loadIcon.Call(0, 32512)
	data := notifyData{Window: s.handle, ID: 1, Flags: 7, Message: shellTrayMessage, Icon: icon}
	data.Size = uint32(unsafe.Sizeof(data))
	label, _ := syscall.UTF16FromString("Jarvis Local")
	copy(data.Tip[:], label)
	ok, _, _ := notifyIcon.Call(operation, uintptr(unsafe.Pointer(&data)))
	return ok != 0
}
func (s *assistantShell) trayMenu() {
	menu, _, _ := makeMenu.Call()
	if menu == 0 {
		return
	}
	defer destroyMenu.Call(menu)
	for i, label := range []string{"Abrir Jarvis", "Sair"} {
		p := wide(label)
		appendMenu.Call(menu, 0, uintptr(i+1), pointer(p))
		runtime.KeepAlive(p)
	}
	var pt point
	getCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	setForeground.Call(s.handle)
	choice, _, _ := trackMenu.Call(menu, 0x102, uintptr(pt.X), uintptr(pt.Y), 0, s.handle, 0)
	switch choice {
	case 1:
		s.show()
	case 2:
		postMessage.Call(s.handle, wmClose, 0, 0)
	}
}

func shellProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	s := activeShell
	if s != nil && s.handle == hwnd {
		switch msg {
		case 0x0083:
			if wp != 0 {
				return 0
			}
		case 0x0084:
			var r winRect
			getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
			x, y := int32(int16(lp)), int32(int16(lp>>16))
			left, right := x-r.Left < 6, r.Right-x <= 6
			top, bottom := y-r.Top < 6, r.Bottom-y <= 6
			switch {
			case top && left:
				return 13
			case top && right:
				return 14
			case bottom && left:
				return 16
			case bottom && right:
				return 17
			case left:
				return 10
			case right:
				return 11
			case top:
				return 12
			case bottom:
				return 15
			}
		case 0x0005:
			if s.browser != nil && s.browser.GetController() != nil {
				s.browser.Resize()
			}
		case 0x0003:
			if s.browser != nil && s.browser.GetController() != nil {
				_ = s.browser.NotifyParentWindowPositionChanged()
			}
		case 0x0006:
			if uint16(wp) != 0 && s.browser != nil && s.browser.GetController() != nil {
				s.browser.Focus()
			}
		case 0x0104:
			if wp == vkF4 && lp&(1<<29) != 0 {
				postMessage.Call(hwnd, wmClose, 0, 0)
				return 0
			}
		case wmClose:
			requestAppClose(s.work, hwnd)
			return 0
		case 0x0002:
			if s.tray {
				s.icon(2)
			}
			if s.hotkey {
				unregisterHotkey.Call(hwnd, shellHotkeyID)
			}
			s.handle = 0
			closeFromWork(s.work)
			return 0
		case 0x0312:
			if wp == shellHotkeyID {
				s.toggle()
				return 0
			}
		case shellTrayMessage:
			switch uint16(lp) {
			case 0x0202:
				s.show()
			case 0x0205:
				s.trayMenu()
			}
			return 0
		case shellDragMessage:
			releaseCapture.Call()
			send.Call(hwnd, 0x00a1, 2, 0)
			return 0
		}
	}
	ret, _, _ := defProc.Call(hwnd, uintptr(msg), wp, lp)
	return ret
}

type assistantRequest struct {
	Revision            uint64
	ID                  uint32
	Token, Method, Text string
	Value               bool
}

func (s *assistantShell) handleMessage(raw string) {
	if len(raw) > 18000 {
		return
	}
	var req assistantRequest
	if json.Unmarshal([]byte(raw), &req) != nil || subtle.ConstantTimeCompare([]byte(req.Token), []byte(s.token)) != 1 || req.ID == 0 {
		return
	}
	var result any
	var err error
	switch req.Method {
	case "state":
		result = s.desk.snapshotAfter(req.Revision)
	case "info":
		shortcut := ""
		if s.hotkey {
			shortcut = "Ctrl + Alt + J"
		}
		result = map[string]any{"version": Version, "shortcut": shortcut, "voiceProfiles": voiceProfiles, "smoke": s.smoke != ""}
	case "send":
		err = s.desk.submit(req.Text, s.runIntent)
	case "voice":
		s.desk.setVoice(req.Value)
	case "voiceProfile":
		err = s.desk.selectVoiceProfile(req.Text)
	case "voicePreview":
		err = s.desk.previewVoice()
	case "listen":
		err = s.desk.record()
	case "draft":
		result = s.desk.takeDraft()
	case "hide":
		s.hide()
	case "panel":
		err = s.setExpanded(req.Value)
	case "quit":
		postMessage.Call(s.handle, wmClose, 0, 0)
	case "drag":
		postMessage.Call(s.handle, shellDragMessage, 0, 0)
	case "ready":
		if s.smoke != "" {
			err = os.WriteFile(s.smoke, []byte("Painel WebView2 conectado · "+Version), 0600)
		}
	default:
		err = errors.New("ação desconhecida")
	}
	if err != nil {
		result = err.Error()
	}
	payload, marshalErr := json.Marshal(result)
	if marshalErr == nil {
		s.browser.Eval(fmt.Sprintf("window.jarvisResolve(%d,%t,%s)", req.ID, err == nil, payload))
	}
}

func (s *assistantShell) showWork() {
	show.Call(s.work.handle, 9)
	setForeground.Call(s.work.handle)
}
func (s *assistantShell) selectMode(project bool) error {
	w := s.work
	if w.inProjects() == project {
		return nil
	}
	_, running := w.engine.snapshot()
	if running || w.updating {
		return errors.New("pause o trabalho atual para trocar de área")
	}
	w.command(modeID)
	if w.inProjects() != project {
		return errors.New("não foi possível abrir essa área de trabalho")
	}
	return nil
}
func (s *assistantShell) runIntent(action desktopIntent) (string, error) {
	w := s.work
	switch action.Kind {
	case "dev_start":
		if s.dev == nil {
			return "", errors.New("Auto Melhorador indisponível no modo de teste")
		}
		return s.dev.start(action.Value, s.desk.note)
	case "dev_status":
		if s.dev == nil {
			return "", errors.New("Auto Melhorador indisponível")
		}
		return s.dev.statusText(), nil
	case "dev_review":
		if s.dev == nil {
			return "", errors.New("Auto Melhorador indisponível")
		}
		return s.dev.reviewText()
	case "dev_publish":
		if s.dev == nil {
			return "", errors.New("Auto Melhorador indisponível")
		}
		return s.dev.publishForReview(s.desk.note)
	case "dev_folder":
		if s.dev == nil {
			return "", errors.New("Auto Melhorador indisponível")
		}
		if _, err := os.Stat(filepath.Join(s.dev.root, "proposta.json")); err != nil {
			return "", errors.New("nenhuma proposta local foi preparada")
		}
		if err := openPath(s.handle, s.dev.root); err != nil {
			return "", err
		}
		return "Solicitei ao Windows a abertura da pasta de propostas.", nil
    case "capabilities":
        return assistantCapabilitiesAnswer(), nil
    case "capability_detail":
        return assistantCapabilityDetail(action.Value), nil
	case "status", "statusProject":
        if action.Kind == "statusProject" {
            return "Para consultar projetos anteriores, abra o Jarvis em --classic.", nil
        }
		state, running := w.engine.snapshot()
		if len(state.Missions) == 0 {
			return "A área atual ainda está vazia.", nil
		}
		m := state.Missions[len(state.Missions)-1]
		status := fmt.Sprintf("Trabalho atual: %s\nEtapa: %s", m.Goal, m.Phase)
		if running {
			status += " · em andamento"
		}
		if m.Failure != "" {
			status += "\nPendência: " + m.Failure
		}
		return status, nil
	case "projects", "missions":
		return "A janela antiga foi retirada da interface principal. Os projetos antigos continuam preservados. Para acessá-los, use o modo de compatibilidade --classic.", nil
	case "build":
		return "", errors.New("Projetos antigos estão disponíveis apenas no modo de compatibilidade --classic.")
	case "resume", "resumeProject":
		return "Sessões antigas estão disponíveis apenas no modo --classic.", nil
	case "pause":
		_, running := w.engine.snapshot()
		if !running {
			return "Não há trabalho ativo nesta área.", nil
		}
		w.engine.pause()
		return "Pausa solicitada. O Jarvis preserva a última etapa salva.", nil
	case "folder":
		state, _ := w.engine.snapshot()
		if len(state.Missions) == 0 {
			return "", errors.New("a área atual ainda não tem uma entrega")
		}
		mission := state.Missions[len(state.Missions)-1]
		if w.inProjects() && mission.Project.OutputDir == "" {
			return "", errors.New("o projeto ainda não concluiu a compilação")
		}
		path := filepath.Join(w.engine.profile, "missoes", mission.ID)
		if w.inProjects() {
			path = projectFolder(w.engine, mission)
		}
		if err := openPath(s.handle, path); err != nil {
			return "", err
		}
		return "Solicitei ao Windows a abertura da pasta no Explorador.", nil
	case "update":
		_, running := w.engine.snapshot()
		if running || w.updating {
			return "", errors.New("aguarde a tarefa ativa para verificar a atualização")
		}
		w.command(otaID)
		return "Solicitei a verificação de atualização; o resultado será mostrado quando estiver disponível.", nil
	case "open":
		if err := startWindowsApp(action.Value); err != nil {
			return "", err
		}
		app, _ := capabilityByID(action.Value)
		return "Solicitei ao Windows a abertura de " + app.Name + ".", nil
	}
	return "", errors.New("ação indisponível")
}
func windowsSystemDirectory() (string, error) {
	buf := make([]uint16, 32768)
	n, _, err := getSystemDirectory.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 || n >= uintptr(len(buf)) {
		return "", fmt.Errorf("pasta do Windows indisponível: %v", err)
	}
	return syscall.UTF16ToString(buf), nil
}

func startWindowsApp(app string) error {
	// Explicitly registered apps only; never accept model-supplied executable
	// paths, fuzzy matches or shell commands.
	definition, ok := capabilityByID(app)
	if !ok || definition.Scope != capabilityOverlay || definition.Executable == "" {
		return errors.New("aplicativo fora da lista de comandos")
	}
	systemDir, err := windowsSystemDirectory()
	if err != nil {
		return err
	}
	dir := systemDir
	switch definition.InstallRoot {
	case "system":
	case "windows":
		dir = filepath.Dir(systemDir)
	default:
		return errors.New("diretório de aplicativo inválido")
	}
	path := filepath.Join(dir, definition.Executable)
	if err := launchWindowsProgram(path, dir); err != nil {
		return fmt.Errorf("o Windows não iniciou %s: %w", definition.Name, err)
	}
	return nil
}

func launchWindowsProgram(path, dir string) error {
	cmd := exec.Command(path)
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func requestAppClose(w *Window, hwnd uintptr) {
	if w.updating && !confirm(hwnd, "Fechar durante a atualização?") {
		return
	}
	closeFromWork(w)
}

func closeFromWork(w *Window) {
	if w.closing {
		return
	}
	w.closing = true
	w.cancel()
	if activeShell != nil && activeShell.handle != 0 {
		closeWindow.Call(activeShell.handle)
	}
	closeWindow.Call(w.handle)
}
