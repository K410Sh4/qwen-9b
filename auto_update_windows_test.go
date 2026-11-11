package main

import (
    "crypto/sha256"
    "encoding/hex"
    "testing"
)

func TestAutomaticUpdateWaitsForSafeIdle(t *testing.T) {
    cases := []struct {
        name string
        closing, updating, work, chat bool
        activity string
        want bool
    }{
        {"idle and collapsed",false,false,false,false,"ready",true},
        {"chat editing",false,false,false,true,"ready",false},
        {"speaking",false,false,false,false,"speaking",false},
        {"thinking",false,false,false,false,"thinking",false},
        {"microphone active",false,false,false,false,"listening",false},
        {"project active",false,false,true,false,"ready",false},
        {"update in progress",false,true,false,false,"ready",false},
        {"closing",true,false,false,false,"ready",false},
    }
    for _, tc := range cases {
        t.Run(tc.name,func(t *testing.T) {
            got := automaticUpdateIdle(tc.closing,tc.updating,tc.work,tc.chat,tc.activity)
            if got != tc.want {
                t.Fatalf("automaticUpdateIdle = %v, want %v",got,tc.want)
            }
        })
    }
}

func TestUpdateBinaryValidation(t *testing.T) {
    exe := append([]byte("MZ"),make([]byte,256)...)
    sum := sha256.Sum256(exe)
    good := Update{Version:"v0.4.2",Hash:hex.EncodeToString(sum[:]),Size:int64(len(exe))}
    if err := validateUpdateBinary(good,exe); err != nil {
        t.Fatalf("valid binary rejected: %v",err)
    }
    altered := append([]byte(nil),exe...)
    altered[5] = 1
    if err := validateUpdateBinary(good,altered); err == nil {
        t.Fatal("tampered executable accepted")
    }
    changedSize := good
    changedSize.Size++
    if err := validateUpdateBinary(changedSize,exe); err == nil {
        t.Fatal("wrong content length accepted")
    }
    notPE := append([]byte("NO"),exe[2:]...)
    sum = sha256.Sum256(notPE)
    wrongFormat := Update{Version:"v0.4.2",Hash:hex.EncodeToString(sum[:]),Size:int64(len(notPE))}
    if err := validateUpdateBinary(wrongFormat,notPE); err == nil {
        t.Fatal("wrong binary signature accepted")
    }
    if err := validateUpdateBinary(good,nil); err == nil {
        t.Fatal("empty response accepted")
    }
}
