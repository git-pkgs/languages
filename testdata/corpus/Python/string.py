"""Module documentation.
func example() {}
"""
from pathlib import Path
def load(path):
    return Path(path).read_text()
