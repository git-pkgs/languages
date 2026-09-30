from pathlib import Path
import json

def load(path):
    return json.loads(Path(path).read_text())
