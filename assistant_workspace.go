package main

import (
    "context"
    "errors"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "regexp"
    "sort"
    "strings"
    "time"
    "unicode/utf8"
)

const workspaceMaxContent = 12000
const workspaceMaxHistoryItems = 50
var workspaceFilePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}\.(txt|md|json|go|py|html|css|js|csv)$`)

// Only flat, text-only filenames in Jarvis's application-owned workspace.
// There is no model-selected path, shell or access to the user's home folder.
func validateWorkspaceFile(file string) error {
    if !workspaceFilePattern.MatchString(file) || file=="." || file==".." {
        return errors.New("use um nome simples de arquivo .txt, .md, .json, .go, .py, .html, .css, .js ou .csv")
    }
    base:=strings.ToLower(strings.SplitN(file,".",2)[0])
    if base=="con"||base=="prn"||base=="aux"||base=="nul"||
       (len(base)==4 && (strings.HasPrefix(base,"com")||strings.HasPrefix(base,"lpt")) &&
        base[3]>='1' && base[3]<='9') {
        return errors.New("nome reservado pelo Windows")
    }
    return nil
}

func workspaceDirectory(profile string) string {
    return filepath.Join(profile,"Workspace")
}
func safeWorkspaceDir(profile string) (string,error) {
    dir:=workspaceDirectory(profile)
    if err:=os.MkdirAll(dir,0700);err!=nil{return "",err}
    info,err:=os.Lstat(dir)
    if err!=nil{return "",err}
    if !info.IsDir() || info.Mode()&os.ModeSymlink!=0 {
        return "",errors.New("workspace não é um diretório regular")
    }
    return dir,nil
}
func workspaceFileInfo(path string) (os.FileInfo,error) {
    info,err:=os.Lstat(path)
    if err!=nil {return nil,err}
    if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink!=0 {
        return nil,errors.New("arquivo não é um texto regular")
    }
    return info,nil
}

func listWorkspace(ctx context.Context, profile string)(string,error) {
    dir,err:=safeWorkspaceDir(profile)
    if err!=nil{return "",err}
    entries,err:=os.ReadDir(dir)
    if err!=nil{return "",err}
    lines:=make([]string,0,len(entries))
    for _,entry:=range entries {
        if err:=ctx.Err();err!=nil{return "",err}
        if validateWorkspaceFile(entry.Name())!=nil {continue}
        info,err:=workspaceFileInfo(filepath.Join(dir,entry.Name()))
        if err!=nil {continue}
        lines=append(lines,fmt.Sprintf("%s (%d bytes)",entry.Name(),info.Size()))
    }
    sort.Strings(lines)
    if len(lines)>50 {lines=append(lines[:50],"Outros arquivos omitidos.") }
    if len(lines)==0 {return "O workspace do Jarvis ainda não possui arquivos.",nil}
    return "Arquivos do workspace Jarvis:\n"+strings.Join(lines,"\n"),nil
}
func readWorkspace(ctx context.Context, profile,file string)(string,error) {
    if err:=validateWorkspaceFile(file);err!=nil{return "",err}
    if err:=ctx.Err();err!=nil{return "",err}
    dir,err:=safeWorkspaceDir(profile)
    if err!=nil{return "",err}
    name:=filepath.Join(dir,file)
    info,err:=workspaceFileInfo(name)
    if err!=nil{return "",err}
    if info.Size()>workspaceMaxContent {return "",errors.New("arquivo excede limite de leitura do assistente (12 KB)") }
    f,err:=os.Open(name)
    if err!=nil{return "",err}
    defer f.Close()
    raw,err:=io.ReadAll(io.LimitReader(f,workspaceMaxContent+1))
    if err!=nil{return "",err}
    if len(raw)>workspaceMaxContent || !utf8.Valid(raw) {return "",errors.New("arquivo não é texto UTF-8 compatível")}
    if err:=ctx.Err();err!=nil{return "",err}
    return fmt.Sprintf("Conteúdo de %s:\n%s",file,string(raw)),nil
}
func writeWorkspace(ctx context.Context, profile,file,content string)(string,error) {
    if err:=validateWorkspaceFile(file);err!=nil{return "",err}
    if len(content)==0 || len(content)>workspaceMaxContent || !utf8.ValidString(content) {
        return "",errors.New("o conteúdo precisa ser texto UTF-8 de até 12 KB")
    }
    if err:=ctx.Err();err!=nil{return "",err}
    dir,err:=safeWorkspaceDir(profile)
    if err!=nil{return "",err}
    dest:=filepath.Join(dir,file)
    var previous []byte
    info,err:=workspaceFileInfo(dest)
    if err==nil {
        if info.Size()>workspaceMaxContent {return "",errors.New("arquivo atual excede o limite de backup")}
        previous,err=os.ReadFile(dest)
        if err!=nil{return "",err}
        if !utf8.Valid(previous) {return "",errors.New("arquivo atual não é texto UTF-8")}
    } else if !errors.Is(err,os.ErrNotExist) {return "",err}
    backup:=""
    if previous!=nil {
        history:=filepath.Join(dir,"Historico")
        if err:=os.MkdirAll(history,0700);err!=nil{return "",err}
        hinfo,err:=os.Lstat(history)
        if err!=nil{return "",err}
        if !hinfo.IsDir() || hinfo.Mode()&os.ModeSymlink!=0 {return "",errors.New("histórico não é pasta regular")}
        items,err:=os.ReadDir(history)
        if err!=nil{return "",err}
        n:=0
        for _,item:=range items {if strings.HasPrefix(item.Name(),file+"."){n++}}
        if n>=workspaceMaxHistoryItems {return "",errors.New("limite de snapshots atingido; revise o histórico antes de continuar")}
        backup=filepath.Join(history,fmt.Sprintf("%s.%d.bak",file,time.Now().UnixNano()))
        f,err:=os.OpenFile(backup,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600)
        if err!=nil{return "",err}
        _,writeErr:=f.Write(previous)
        closeErr:=f.Close()
        if writeErr!=nil{return "",writeErr}
        if closeErr!=nil{return "",closeErr}
    }
    tmp,err:=os.CreateTemp(dir,".jarvis-write-*")
    if err!=nil{return "",err}
    defer os.Remove(tmp.Name())
    if err:=tmp.Chmod(0600);err!=nil{tmp.Close();return "",err}
    _,writeErr:=tmp.WriteString(content)
    syncErr:=tmp.Sync()
    closeErr:=tmp.Close()
    if err:=errors.Join(writeErr,syncErr,closeErr);err!=nil{return "",err}
    if err:=ctx.Err();err!=nil{return "",err}
    if err:=os.Rename(tmp.Name(),dest);err!=nil {
        return "",fmt.Errorf("falha ao substituir arquivo; cópia anterior preservada: %w",err)
    }
    if backup!="" {return fmt.Sprintf("Arquivo %s atualizado no workspace; versão anterior preservada em Histórico.",file),nil}
    return fmt.Sprintf("Arquivo %s criado no workspace do Jarvis (%d bytes).",file,len(content)),nil
}
