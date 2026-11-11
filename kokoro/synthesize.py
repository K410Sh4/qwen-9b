"""Generate Kokoro WAV clips in single-shot or persistent conversation mode."""

import json
import os
from pathlib import Path
import sys
import textwrap
import wave

import numpy as np
from kokoro_onnx import Kokoro
from kokoro_onnx.config import MAX_PHONEME_LENGTH

VOICES = ("pf_dora", "pm_alex")


def load_model():
    os.environ.pop("PHONEMIZER_ESPEAK_LIBRARY", None)
    os.environ["ONNX_PROVIDER"] = "CPUExecutionProvider"
    root = Path(__file__).resolve().parent
    return Kokoro(str(root / "kokoro.onnx"), str(root / "voices.bin"))


def synthesize(text, voice, output, model=None):
    if voice not in VOICES or not isinstance(text, str) or not 0 < len(text) <= 1200:
        raise ValueError("Solicitação de síntese inválida")
    if model is None:
        model = load_model()
    phonemes = model.tokenizer.phonemize(text, lang="pt-br")
    with wave.open(str(output), "wb") as wav:
        wav.setparams((1, 2, 24000, 0, "NONE", "not compressed"))
        for chunk in textwrap.wrap(phonemes, width=MAX_PHONEME_LENGTH - 16):
            audio, rate = model.create(chunk, voice, is_phonemes=True)
            if rate != wav.getframerate():
                raise ValueError(f"Frequência de áudio inesperada: {rate}")
            pcm = (np.clip(audio, -1.0, 1.0) * 32767).astype("<i2")
            wav.writeframesraw(pcm.tobytes())


def serve():
    model = load_model()
    for line in sys.stdin.buffer:
        try:
            request = json.loads(line)
            synthesize(request["text"], request["voice"], Path(request["output"]), model)
            response = {"ok": True}
        except Exception as exc:
            response = {"ok": False, "error": str(exc)[:300]}
        sys.stdout.write(json.dumps(response, ensure_ascii=False) + "\n")
        sys.stdout.flush()


if __name__ == "__main__":
    if len(sys.argv) == 2 and sys.argv[1] == "--serve":
        serve()
    elif len(sys.argv) == 2:
        request = json.loads(sys.stdin.buffer.read())
        synthesize(request["text"], request["voice"], Path(sys.argv[1]))
    else:
        raise SystemExit("Uso: synthesize.py <arquivo.wav> | --serve")
