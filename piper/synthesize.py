"""Single-voice resident Faber Piper TTS process. All IPC uses JSON lines."""
import io
import json
from pathlib import Path
import sys
import wave

from piper import PiperVoice

VOICE = "pt_BR-faber-medium"
ROOT = Path(__file__).resolve().parent


def load_voice(profile):
    if profile != VOICE:
        raise ValueError("Somente a voz Faber está disponível")
    return PiperVoice.load(str(ROOT / (VOICE + ".onnx")), use_cuda=False)


def synthesize(text, voice, destination):
    if not isinstance(text, str) or not 0 < len(text) <= 1200:
        raise ValueError("Texto de fala inválido")
    with wave.open(str(destination), "wb") as wav:
        voice.synthesize_wav(text, wav)
    if destination.stat().st_size < 48:
        raise ValueError("A síntese não gerou áudio")


def serve():
    voice = load_voice(VOICE)
    # Execute one short offline inference to initialize lazy ONNX/phonemizer paths.
    # Discard the samples: warm-up must never be spoken to the user.
    with wave.open(io.BytesIO(), "wb") as sink:
        voice.synthesize_wav("Olá.", sink)
    # A startup handshake confirms the ONNX session is actually loaded.
    sys.stdout.write('{"ready":true}\n')
    sys.stdout.flush()
    for line in sys.stdin.buffer:
        try:
            request = json.loads(line)
            if request.get("voice") != VOICE:
                raise ValueError("Solicitação para voz diferente de Faber")
            synthesize(request["text"], voice, Path(request["output"]))
            result = {"ok": True}
        except Exception as exc:
            result = {"ok": False, "error": str(exc)[:250]}
        sys.stdout.write(json.dumps(result, ensure_ascii=False) + "\n")
        sys.stdout.flush()


if __name__ == "__main__":
    if len(sys.argv) == 2 and sys.argv[1] == "--serve":
        serve()
    elif len(sys.argv) == 2:
        req = json.loads(sys.stdin.buffer.read())
        synthesize(req["text"], load_voice(req["voice"]), Path(sys.argv[1]))
    else:
        raise SystemExit("Uso: synthesize.py --serve | <arquivo.wav>")
