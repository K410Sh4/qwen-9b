package main

import (
 "bytes"
 "context"
 "encoding/binary"
 "os"
 "path/filepath"
 "testing"
 "time"
)

func TestPiperWindowsRuntimeAndTiming(t *testing.T) {
 archive := os.Getenv("JARVIS_PIPER_ARCHIVE")
 if archive == "" { t.Skip("pacote Piper disponível no workflow Windows") }
 ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
 defer cancel()
 dir := filepath.Join(t.TempDir(), "Runtime Piper")
 if err := unpackPiper(ctx, archive, dir); err != nil { t.Fatal(err) }
 if !piperInstalled(dir) { t.Fatal("runtime Piper incompleto") }
 var prev []byte
 for _, profile := range voiceProfiles {
  wav := filepath.Join(dir, profile.ID+".wav")
  started := time.Now()
  cmd,err := piperCommand(ctx,dir,wav,"Olá, eu sou o Jarvis. Esta frase é um teste da velocidade da voz brasileira.",profile.ID)
  if err != nil { t.Fatal(err) }
  if output,err := cmd.CombinedOutput(); err != nil {
   t.Fatalf("%s: %v: %s",profile.Name,err,output)
  }
  elapsed := time.Since(started)
  audio,err := os.ReadFile(wav)
  if err != nil { t.Fatal(err) }
  if len(audio) <= 2048 || string(audio[:4])!="RIFF" || string(audio[8:12])!="WAVE" {
   t.Fatalf("%s: WAV inválido (%d bytes)",profile.Name,len(audio))
  }
  if bytes.Equal(prev,audio) { t.Fatalf("voz %s não difere da anterior",profile.Name) }
  rate := binary.LittleEndian.Uint32(audio[24:28])
  want := uint32(22050)
  if profile.ID=="pt_BR-edresson-low" { want=16000 }
  if rate!=want { t.Fatalf("%s sample rate %d != %d",profile.Name,rate,want) }
  seconds := float64(len(audio)-44)/float64(rate*2)
  t.Logf("Piper %s: síntese inicial %v, duração áudio aprox %.2fs, RTF %.2f",profile.Name,elapsed,seconds,elapsed.Seconds()/seconds)
  prev=audio
 }
}

func TestPiperPersistentTwoSentences(t *testing.T) {
 archive := os.Getenv("JARVIS_PIPER_ARCHIVE")
 if archive == "" { t.Skip("runtime Piper disponível no workflow") }
 ctx,cancel := context.WithTimeout(context.Background(),3*time.Minute)
 defer cancel()
 dir := filepath.Join(t.TempDir(),"runtime")
 if err := unpackPiper(ctx,archive,dir);err!=nil { t.Fatal(err) }
 clips := t.TempDir()
 input := make(chan string,1)
 input <- "Esta é a segunda frase."
 close(input)
 ready := make(chan string,2)
 result := make(chan error,1)
 started := time.Now()
 go func(){
  result <- synthesizePiperSequence(ctx,dir,clips,"pt_BR-edresson-low","Esta é a primeira frase.",input,ready)
  close(ready)
 }()
 first := time.Duration(0)
 count := 0
 for file := range ready {
  if count==0 { first=time.Since(started) }
  wav,err := os.ReadFile(file)
  if err!=nil {t.Fatal(err)}
  if len(wav)<=2048 || string(wav[:4])!="RIFF" {t.Fatalf("frase %d inválida",count)}
  count++
 }
 if err:=<-result;err!=nil{t.Fatal(err)}
 if count!=2{t.Fatalf("expected 2 clips got %d",count)}
 t.Logf("Piper persistent: primeiro áudio em %v, 2 frases prontas em %v",first,time.Since(started))
}
