package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	user          = syscall.NewLazyDLL("user32.dll")
	kernel        = syscall.NewLazyDLL("kernel32.dll")
	gdi           = syscall.NewLazyDLL("gdi32.dll")
	shell         = syscall.NewLazyDLL("shell32.dll")
	createWindow  = user.NewProc("CreateWindowExW")
	register      = user.NewProc("RegisterClassExW")
	defProc       = user.NewProc("DefWindowProcW")
	setText       = user.NewProc("SetWindowTextW")
	getText       = user.NewProc("GetWindowTextW")
	getTextLength = user.NewProc("GetWindowTextLengthW")
	show          = user.NewProc("ShowWindow")
	enable        = user.NewProc("EnableWindow")
	move          = user.NewProc("MoveWindow")
	send          = user.NewProc("SendMessageW")
	setTimer      = user.NewProc("SetTimer")
	killTimer     = user.NewProc("KillTimer")
	messageBox    = user.NewProc("MessageBoxW")
	closeWindow   = user.NewProc("DestroyWindow")
	postQuit      = user.NewProc("PostQuitMessage")
	getMessage    = user.NewProc("GetMessageW")
	translate     = user.NewProc("TranslateMessage")
	dispatch      = user.NewProc("DispatchMessageW")
	loadCursor    = user.NewProc("LoadCursorW")
	getModule     = kernel.NewProc("GetModuleHandleW")
	setLast       = kernel.NewProc("SetLastError")
	getStock      = gdi.NewProc("GetStockObject")
	shellOpen     = shell.NewProc("ShellExecuteW")
	openProcess   = kernel.NewProc("OpenProcess")
	waitObject    = kernel.NewProc("WaitForSingleObject")
	closeHandle   = kernel.NewProc("CloseHandle")
	createMutex   = kernel.NewProc("CreateMutexW")
)

const (
	startID = 100 + iota
	pauseID
	approveID
	discardID
	previewID
	folderID
	otaID
)

type point struct{ X, Y int32 }
type message struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Position       point
	Private        uint32
}
type windowClass struct {
	Size, Style                        uint32
	Proc                               uintptr
	ExtraClass, ExtraWindow            int32
	Instance, Icon, Cursor, Background uintptr
	Menu, Name                         *uint16
	SmallIcon                          uintptr
}
type otaResult struct {
	Update  *Update
	Err     error
	Applied bool
}
type Window struct {
	engine                          *Engine
	handle, instance, font          uintptr
	heading, objective, status, log uintptr
	buttons                         []uintptr
	lastRevision                    uint64
	lastRunning, refresh, updating  bool
	updates                         chan otaResult
}

var mainWindow *Window

func wide(s string) *uint16 {
	value, _ := syscall.UTF16PtrFromString(strings.ReplaceAll(s, "\x00", ""))
	return value
}
func pointer(p *uint16) uintptr { return uintptr(unsafe.Pointer(p)) }
func text(hwnd uintptr, s string) {
	p := wide(s)
	setText.Call(hwnd, pointer(p))
	runtime.KeepAlive(p)
}
func errorDialog(hwnd uintptr, s string) { dialog(hwnd, s, 0x10) }
func dialog(hwnd uintptr, s string, mode uintptr) {
	title, p := wide("Jarvis Local"), wide(s)
	messageBox.Call(hwnd, pointer(p), pointer(title), mode)
	runtime.KeepAlive(title)
	runtime.KeepAlive(p)
}
func confirm(hwnd uintptr, s string) bool {
	title, p := wide("Jarvis Local"), wide(s)
	result, _, _ := messageBox.Call(hwnd, pointer(p), pointer(title), 0x24|0x100)
	runtime.KeepAlive(title)
	runtime.KeepAlive(p)
	return result == 6
}

func controlText(hwnd uintptr) string {
	n, _, _ := getTextLength.Call(hwnd)
	value := make([]uint16, n+1)
	getText.Call(hwnd, uintptr(unsafe.Pointer(&value[0])), n+1)
	return syscall.UTF16ToString(value)
}

func (w *Window) child(class, label string, style, id uintptr) uintptr {
	className, title := wide(class), wide(label)
	hwnd, _, _ := createWindow.Call(0, pointer(className), pointer(title), style|0x50000000, 0, 0, 1, 1, w.handle, id, w.instance, 0)
	runtime.KeepAlive(className)
	runtime.KeepAlive(title)
	if hwnd != 0 {
		send.Call(hwnd, 0x30, w.font, 1)
	}
	return hwnd
}

func (w *Window) setup() error {
	w.heading = w.child("STATIC", "Objetivo permanente", 0, 0)
	w.objective = w.child("EDIT", "", 0x00810000|0x00000044, 0)
	send.Call(w.objective, 0x00c5, 1200, 0)
	w.status = w.child("STATIC", "", 0, 0)
	w.log = w.child("EDIT", "", 0x00a00000|0x00000844, 0)
	w.buttons = make([]uintptr, 7)
	labels := []string{"Iniciar sessão", "Pausar", "Aprovar missão", "Descartar", "Abrir protótipo", "Abrir pasta", "Atualizar"}
	for i, label := range labels {
		w.buttons[i] = w.child("BUTTON", label, 0x00010000, uintptr(startID+i))
	}
	for _, h := range append([]uintptr{w.heading, w.objective, w.status, w.log}, w.buttons...) {
		if h == 0 {
			return errors.New("não foi possível criar os controles da janela")
		}
	}
	state, _ := w.engine.snapshot()
	goal := state.Goal
	if goal == "" {
		goal = "Encontre uma oportunidade de mercado e construa um protótipo que teste a hipótese."
	}
	text(w.objective, goal)
	w.refresh = true
	w.paint()
	return nil
}

func (w *Window) layout(width, height int) {
	if w.objective == 0 {
		return
	}
	set := func(h uintptr, x, y, a, b int) {
		move.Call(h, uintptr(x), uintptr(y), uintptr(max(1, a)), uintptr(max(1, b)), 1)
	}
	set(w.heading, 16, 14, width-32, 22)
	set(w.objective, 16, 39, width-32, 66)
	var x, y int = 16, 118
	widths := []int{104, 75, 116, 80, 115, 93, 90}
	for i, h := range w.buttons {
		if x+widths[i]+16 > width && x > 16 {
			x = 16
			y += 36
		}
		set(h, x, y, widths[i], 29)
		x += widths[i] + 7
	}
	set(w.status, 16, y+40, width-32, 24)
	set(w.log, 16, y+68, width-32, height-y-83)
}

func can(hwnd uintptr, v bool) {
	flag := uintptr(0)
	if v {
		flag = 1
	}
	enable.Call(hwnd, flag)
}

func (w *Window) paint() {
	state, running := w.engine.snapshot()
	if !w.refresh && w.lastRevision == state.Revision && w.lastRunning == running {
		return
	}
	w.refresh, w.lastRevision, w.lastRunning = false, state.Revision, running
	var mission Mission
	if len(state.Missions) > 0 {
		mission = state.Missions[len(state.Missions)-1]
	}
	phase := mission.Phase
	inProgress := phase != "" && phase != "concluída" && phase != "descartada"
	can(w.objective, !running && !w.updating && !inProgress)
	can(w.buttons[0], !running && !w.updating && phase != "aprovação")
	can(w.buttons[1], running)
	can(w.buttons[2], !running && !w.updating && phase == "aprovação")
	can(w.buttons[3], !running && !w.updating && inProgress)
	can(w.buttons[4], !running && mission.Tool.Name != "")
	can(w.buttons[5], !running)
	can(w.buttons[6], !running && !w.updating)
	status := "Aguardando início da sessão."
	if phase != "" {
		status = mission.ID + " · " + phase
	}
	if running {
		status += " · sessão em andamento"
	}
	if w.updating {
		status += " · verificando atualização"
	}
	text(w.status, status)
	var lines []string
	lines = append(lines, "Estratégia ativa: "+state.Strategy, fmt.Sprintf("Missões: %d  |  Ferramentas testadas: %d", len(state.Missions), len(state.Tools)))
	if state.Candidate != "" {
		lines = append(lines, "Próximo experimento: "+state.Candidate)
	}
	if m := mission.Opportunity; m.Title != "" {
		lines = append(lines, "", "Oportunidade: "+m.Title, "Público: "+m.Audience, "Problema: "+m.Problem, "Hipótese: "+m.Hypothesis, "O que derrubaria a hipótese: "+m.Contradiction, "Próxima tarefa escolhida: "+m.NextTask, "", "Fontes")
		for _, id := range m.EvidenceIDs {
			for _, e := range mission.Evidence {
				if e.ID == id {
					lines = append(lines, e.Title+" — "+e.Source, e.Link, e.Summary)
				}
			}
		}
	}
	if len(mission.Tests) > 0 {
		lines = append(lines, "", "Testes de aceite")
		lines = append(lines, mission.Tests...)
	}
	if m := mission.Review; m.Reason != "" {
		lines = append(lines, "", "Autocrítica: "+m.Reason, "Lição: "+m.Lesson)
	}
	if mission.Failure != "" {
		lines = append(lines, "", "Pendência: "+mission.Failure)
	}
	if len(mission.Log) > 0 {
		lines = append(lines, "", "Histórico")
		lines = append(lines, mission.Log...)
	}
	text(w.log, strings.Join(lines, "\r\n"))
}

func (w *Window) command(id int) {
	var err error
	switch id {
	case startID:
		err = w.engine.start(controlText(w.objective))
	case pauseID:
		w.engine.pause()
	case approveID:
		if confirm(w.handle, "Você abriu o protótipo e confirmou que o fluxo principal funciona?") {
			err = w.engine.resolve(true)
		}
	case discardID:
		if confirm(w.handle, "Descartar esta missão? Os arquivos e o histórico continuarão disponíveis.") {
			err = w.engine.resolve(false)
		}
	case previewID, folderID:
		state, _ := w.engine.snapshot()
		path := w.engine.profile
		if id == previewID && len(state.Missions) > 0 {
			path = filepath.Join(path, "missoes", state.Missions[len(state.Missions)-1].ID, "prototipo.html")
		}
		if id == folderID && len(state.Missions) > 0 {
			if confirm(w.handle, "Abrir a pasta da missão? Escolha Não para abrir a pasta de configurações.") {
				path = filepath.Join(path, "missoes", state.Missions[len(state.Missions)-1].ID)
			}
		}
		err = openPath(w.handle, path)
	case otaID:
		if w.updating {
			return
		}
		w.updating = true
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
			defer cancel()
			u, err := findUpdate(ctx)
			w.updates <- otaResult{Update: u, Err: err}
		}()
	}
	if err != nil {
		errorDialog(w.handle, err.Error())
	}
	w.refresh = true
	w.paint()
}

func openPath(hwnd uintptr, path string) error {
	verb, p := wide("open"), wide(path)
	code, _, _ := shellOpen.Call(hwnd, pointer(verb), pointer(p), 0, 0, 1)
	runtime.KeepAlive(verb)
	runtime.KeepAlive(p)
	if code <= 32 {
		return fmt.Errorf("o Windows não abriu %s (código %d)", path, code)
	}
	return nil
}

func (w *Window) poll() {
	select {
	case result := <-w.updates:
		w.updating = false
		w.refresh = true
		if result.Err != nil {
			errorDialog(w.handle, result.Err.Error())
			break
		}
		if result.Applied {
			closeWindow.Call(w.handle)
			return
		}
		if result.Update == nil {
			dialog(w.handle, "Você já está na versão mais recente.", 0x40)
			break
		}
		if confirm(w.handle, "Instalar "+result.Update.Version+" e reiniciar o Jarvis?") {
			w.updating = true
			go func(u Update) {
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				err := stageUpdate(ctx, u)
				w.updates <- otaResult{Applied: err == nil, Err: err}
			}(*result.Update)
		}
	default:
	}
	w.paint()
}

func callback(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	w := mainWindow
	if w == nil {
		r, _, _ := defProc.Call(hwnd, uintptr(msg), wparam, lparam)
		return r
	}
	switch msg {
	case 0x0005:
		w.layout(int(uint16(lparam)), int(uint16(lparam>>16)))
	case 0x0111:
		if wparam>>16 == 0 {
			w.command(int(uint16(wparam)))
		}
	case 0x0113:
		w.poll()
	case 0x0010:
		if w.updating && !confirm(hwnd, "Fechar durante a atualização?") {
			return 0
		}
		closeWindow.Call(hwnd)
	case 0x0002:
		killTimer.Call(hwnd, 1)
		w.engine.pause()
		postQuit.Call(0)
	default:
		r, _, _ := defProc.Call(hwnd, uintptr(msg), wparam, lparam)
		return r
	}
	return 0
}

func waitParent(pid int) error {
	h, _, last := openProcess.Call(0x00100000, 0, uintptr(pid))
	if h == 0 {
		if last == syscall.Errno(87) {
			return nil
		}
		return last
	}
	defer closeHandle.Call(h)
	result, _, _ := waitObject.Call(h, 18000)
	if result != 0 {
		return errors.New("o Jarvis antigo não encerrou a tempo")
	}
	return nil
}

func createUI(e *Engine) (*Window, error) {
	instance, _, _ := getModule.Call(0)
	cursor, _, _ := loadCursor.Call(0, 32512)
	name, title := wide("JarvisLocalWindow"), wide("Jarvis Local "+Version)
	class := windowClass{Proc: syscall.NewCallback(callback), Instance: instance, Cursor: cursor, Background: 16, Name: name}
	class.Size = uint32(unsafe.Sizeof(class))
	if r, _, err := register.Call(uintptr(unsafe.Pointer(&class))); r == 0 {
		return nil, err
	}
	font, _, _ := getStock.Call(17)
	w := &Window{engine: e, instance: instance, font: font, updates: make(chan otaResult, 1)}
	mainWindow = w
	hwnd, _, err := createWindow.Call(0, pointer(name), pointer(title), 0x00cf0000, 0x80000000, 0x80000000, 850, 690, 0, 0, instance, 0)
	runtime.KeepAlive(name)
	runtime.KeepAlive(title)
	if hwnd == 0 {
		return nil, err
	}
	w.handle = hwnd
	if err := w.setup(); err != nil {
		return nil, err
	}
	w.layout(830, 640)
	return w, nil
}

func run() error {
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "--apply-update" {
		return applyUpdate(args[1:], waitParent)
	}
	smoke := len(args) == 2 && args[0] == "--smoke"
	var profile string
	if smoke {
		d, err := os.MkdirTemp("", "jarvis-smoke-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(d)
		profile = d
	} else {
		p := wide("Local\\JarvisLocal-qwen9b")
		setLast.Call(0)
		h, _, last := createMutex.Call(0, 0, pointer(p))
		runtime.KeepAlive(p)
		if h == 0 {
			return last
		}
		defer closeHandle.Call(h)
		if last == syscall.Errno(183) {
			return errors.New("o Jarvis já está aberto")
		}
		data, err := os.UserCacheDir()
		if err != nil {
			return err
		}
		profile = filepath.Join(data, "JarvisLocal")
	}
	e, err := openEngine(profile)
	if err != nil {
		return err
	}
	w, err := createUI(e)
	if err != nil {
		return err
	}
	if smoke {
		closeWindow.Call(w.handle)
		return os.WriteFile(args[1], []byte("Janela Win32 criada · "+Version), 0600)
	}
	setTimer.Call(w.handle, 1, 350, 0)
	show.Call(w.handle, 5)
	var msg message
	for {
		result, _, last := getMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) < 0 {
			return last
		}
		if result == 0 {
			return nil
		}
		translate.Call(uintptr(unsafe.Pointer(&msg)))
		dispatch.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func main() {
	runtime.LockOSThread()
	if err := run(); err != nil {
		if len(os.Args) > 1 && os.Args[1] == "--smoke" {
			os.Exit(1)
		}
		errorDialog(0, err.Error())
		os.Exit(1)
	}
}
