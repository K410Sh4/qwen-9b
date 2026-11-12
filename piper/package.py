"""Pack only the Faber Brazilian Portuguese voice and the portable Piper runtime."""
import json
from pathlib import Path
import shutil
import sys
import zipfile

root = Path(sys.argv[1]).resolve()
out = Path(sys.argv[2]).resolve()
base = Path(__file__).resolve().parent
for name in ("synthesize.py", "NOTICES.txt"):
    shutil.copy2(base / name, root / name)
name = "pt_BR-faber-medium"
model = root / (name + ".onnx")
config = root / (name + ".onnx.json")
card = root / (name + ".MODEL_CARD")
if model.stat().st_size < 15_000_000 or not card.is_file():
    raise ValueError("Modelo ou licença Faber ausente")
data = json.loads(config.read_text(encoding="utf-8"))
if data["language"]["code"] != "pt_BR" or data["audio"]["sample_rate"] != 22050:
    raise ValueError("Configuração de voz Faber inválida")
with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED, compresslevel=4) as bundle:
    for path in sorted(root.rglob("*")):
        if path.is_file():
            bundle.write(path, path.relative_to(root).as_posix())
