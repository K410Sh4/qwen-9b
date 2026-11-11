package main

import (
	"path/filepath"
	"testing"
)

func TestLaunchWindowsProgramWithoutConsoleHandles(t *testing.T) {
	root, err := windowsSystemDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if err := launchWindowsProgram(filepath.Join(root, "whoami.exe"), root); err != nil {
		t.Fatalf("falha ao abrir um executável do Windows sem console: %v", err)
	}
}
