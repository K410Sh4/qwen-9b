"""Offline Portuguese Piper TTS; newline-delimited JSON protocol in server mode."""
import json
from pathlib import Path
import sys
import wave

from piper import PiperVoice

VOICES = ("pt_BR-edresson-low", "pt_BR-faber-medium")
ROOT = Path(__file__).resolve().parent

def load_voice(profile):
    if profile not in VOICES:
        raise ValueError("Voz desconhecida")
    return PiperVoice.load(str(ROOT / (profile + ".onnx")), use_cuda=False)

def render(text, voice, destination):
    if not isinstance(text, str) or not 0 < len(text) <= 1200:
        raise ValueError("Texto inválido")
    with wave.open(str(destination), "wb") as wav:
        voice.synthesize_wav(text, wav)
    if destination.stat().st_size < 48:
        raise ValueError("Áudio vazio")

def serve(profile):
    voice = load_voice(profile)
    for raw in sys.stdin.buffer:
        try:
            req = json.loads(raw)
            if req.get("voice") != profile:
                raise ValueError("Voz diferente da sessão em execução")
            render(req["text"], voice, Path(req["output"]))
            result = {"ok": True}
        except Exception as exc:
            result = {"ok": False, "error": str(exc)[:250]}
        sys.stdout.write(json.dumps(result, ensure_ascii=False) + "\n")
        sys.stdout.flush()

if __name__ == "__main__":
    if len(sys.argv) == 3 and sys.argv[1] == "--serve":
        serve(sys.argv[2])
    elif len(sys.argv) == 2:
        req = json.loads(sys.stdin.buffer.read())
        render(req["text"], load_voice(req["voice"]), Path(sys.argv[1]))
    else:
        raise SystemExit("Uso: synthesize.py <wav> | --serve <voice>")
