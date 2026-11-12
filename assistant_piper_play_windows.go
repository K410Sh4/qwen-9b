package main

import (
 "context"
 "fmt"
 "runtime"
 "syscall"
 "time"
 "unsafe"
)

// Native Windows playback removes the PowerShell spawn for every sentence.
var piperPlaySound = syscall.NewLazyDLL("winmm.dll").NewProc("PlaySoundW")
const sndPiperFilename = 0x00020000
const sndPiperNoDefault = 0x00000002

func playPiperWAV(ctx context.Context, path string) error {
 if err := ctx.Err(); err != nil { return err }
 utf16name, err := syscall.UTF16PtrFromString(path)
 if err != nil { return err }
 done := make(chan error, 1)
 go func() {
  ok, _, callErr := piperPlaySound.Call(uintptr(unsafe.Pointer(utf16name)),0,sndPiperFilename|sndPiperNoDefault)
  runtime.KeepAlive(utf16name)
  if ok == 0 { done <- fmt.Errorf("reprodução WinMM indisponível: %v",callErr); return }
  done <- nil
 }()
 select {
 case err := <-done: return err
 case <-ctx.Done():
  piperPlaySound.Call(0,0,0) // Interrupt ongoing waveform playback.
  select { case <-done: case <-time.After(1500*time.Millisecond): }
  return ctx.Err()
 }
}
