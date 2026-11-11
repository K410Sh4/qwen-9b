"""Generate a 24 kHz mono WAV with one of Jarvis Local's Kokoro voices."""

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


def synthesize(text, voice, output):
    if voice not in VOICES or not isinstance(text, str) or not 0 < len(text) <= 1200:
        raise ValueError("Solicitação de síntese inválida")

    # Use only the bundled phonemizer and the CPU provider.
    os.environ.pop("PHONEMIZER_ESPEAK_LIBRARY", None)
    os.environ["ONNX_PROVIDER"] = "CPUExecutionProvider"
    root = Path(__file__).resolve().parent
    model = Kokoro(str(root / "kokoro.onnx"), str(root / "voices.bin"))
    phonemes = model.tokenizer.phonemize(text, lang="pt-br")

    with wave.open(str(output), "wb") as wav:
        wav.setparams((1, 2, 24000, 0, "NONE", "not compressed"))
        for chunk in textwrap.wrap(phonemes, width=MAX_PHONEME_LENGTH - 16):
            audio, rate = model.create(chunk, voice, is_phonemes=True)
            if rate != wav.getframerate():
                raise ValueError(f"Frequência de áudio inesperada: {rate}")
            pcm = (np.clip(audio, -1.0, 1.0) * 32767).astype("<i2")
            wav.writeframesraw(pcm.tobytes())


if __name__ == "__main__":
    request = json.loads(sys.stdin.buffer.read())
    synthesize(request["text"], request["voice"], Path(sys.argv[1]))
