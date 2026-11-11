"""Validate both pinned Brazilian voices and bundle the portable Windows runtime."""
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
for name, sample_rate in (("pt_BR-edresson-low", 16000), ("pt_BR-faber-medium", 22050)):
    model = root / (name + ".onnx")
    config = root / (name + ".onnx.json")
    if model.stat().st_size < 15_000_000:
        raise ValueError("Modelo não foi baixado corretamente: " + name)
    data = json.loads(config.read_text(encoding="utf-8"))
    if data["language"]["code"] != "pt_BR" or data["audio"]["sample_rate"] != sample_rate:
        raise ValueError("Arquivo de configuração inválido: " + name)
with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED, compresslevel=4) as bundle:
    for path in sorted(root.rglob("*")):
        if path.is_file():
            bundle.write(path, path.relative_to(root).as_posix())
