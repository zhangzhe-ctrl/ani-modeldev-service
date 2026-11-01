#!/usr/bin/env python3
"""Fedora-only focused integration using the existing actual-business fixture."""
import json
import pathlib
import subprocess
import tempfile

HERE = pathlib.Path(__file__).resolve().parent
ROOT = next(p for p in HERE.parents if (p / 'go.mod').is_file())
with tempfile.TemporaryDirectory(prefix='originalabe-overlay-') as d:
    overlay = pathlib.Path(d) / 'overlay.json'
    overlay.write_text(json.dumps({'Replace': {
        str(ROOT / 'internal/data/runtimeproof/original_abe_maintenance_test.go'):
        str(HERE / 'testdata/owner_path_test.go.txt')
    }}))
    result = subprocess.run(['go', 'test', '-count=1', '-race', '-overlay', str(overlay),
                             './internal/data/runtimeproof', '-run', '^TestOriginalABEMaintenance', '-v'],
                            cwd=ROOT, check=False)
    raise SystemExit(result.returncode)
