"""Package the Windows runtime with exactly the voice data Jarvis uses."""

from pathlib import Path
import shutil
import sys
import zipfile

import numpy as np

from synthesize import VOICES


root = Path(sys.argv[1]).resolve()
upstream_voices = Path(sys.argv[2])
archive = Path(sys.argv[3])
with np.load(upstream_voices, allow_pickle=False) as source:
    with (root / "voices.bin").open("wb") as target:
        np.savez(target, **{voice: source[voice] for voice in VOICES})

for name in ("synthesize.py", "NOTICES.txt"):
    shutil.copy2(Path(__file__).with_name(name), root / name)

with zipfile.ZipFile(archive, "w", zipfile.ZIP_DEFLATED, compresslevel=4) as bundle:
    for path in sorted(root.rglob("*")):
        if path.is_file():
            bundle.write(path, path.relative_to(root).as_posix())
