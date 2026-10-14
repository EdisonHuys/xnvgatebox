#!/usr/bin/env python3
"""Build a self-contained Linux amd64 package; no tokens or runtime state."""
import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import urllib.request
import zipfile

ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--version", default="v0.3.0")
    parser.add_argument("--manager", type=Path)
    parser.add_argument("--output", type=Path, default=ROOT / "dist")
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory() as tmp:
        stage = Path(tmp)
        (stage / "bin").mkdir()
        if args.manager:
            shutil.copy2(args.manager, stage / "bin/xnvgatebox-manager")
        else:
            subprocess.run(["go", "build", "-trimpath", "-ldflags", "-s -w -X main.version=" + args.version,
                            "-o", str(stage / "bin/xnvgatebox-manager"), "."],
                           cwd=ROOT / "vps/fanout", check=True,
                           env={**os.environ, "GOOS": "linux", "GOARCH": "amd64", "CGO_ENABLED": "0"})
        pin = json.loads((ROOT / "config/xray-release.json").read_text())
        asset = pin["assets"]["linux-x86_64"]
        url = "https://github.com/XTLS/Xray-core/releases/download/" + pin["version"] + "/" + asset["name"]
        with urllib.request.urlopen(url, timeout=120) as response:
            archive = response.read(60_000_001)
        if hashlib.sha256(archive).hexdigest() != asset["sha256"]:
            raise RuntimeError("Xray archive SHA256 mismatch")
        with zipfile.ZipFile(io.BytesIO(archive)) as z:
            (stage / "bin/xray").write_bytes(z.read("xray"))
            (stage / "LICENSE.xray").write_bytes(z.read("LICENSE"))
        for binary in (stage / "bin").iterdir():
            binary.chmod(0o755)
        shutil.copytree(ROOT / "src", stage / "src", ignore=shutil.ignore_patterns("__pycache__", "*.pyc"))
        shutil.copytree(ROOT / "config", stage / "config")
        (stage / "scripts").mkdir()
        shutil.copy2(ROOT / "scripts/toolbox.py", stage / "scripts/toolbox.py")
        shutil.copytree(ROOT / "vps/deploy", stage / "deploy")
        shutil.copy2(ROOT / "vps/install.sh", stage / "install.sh")
        shutil.copy2(ROOT / "LICENSE", stage / "LICENSE")
        shutil.copy2(ROOT / "vps/fanout/LICENSE", stage / "LICENSE.reference")
        (stage / "VERSION").write_text(args.version + "\n")
        sums = {str(p.relative_to(stage)): hashlib.sha256(p.read_bytes()).hexdigest()
                for p in stage.rglob("*") if p.is_file()}
        (stage / "package-sha256.json").write_text(json.dumps(sums, sort_keys=True, indent=2))
        dest = args.output / "xnvgatebox-linux-amd64.tar.gz"
        with tarfile.open(dest, "w:gz") as t:
            for p in sorted(stage.iterdir()):
                t.add(p, arcname=p.name)
        checksum = hashlib.sha256(dest.read_bytes()).hexdigest()
        (args.output / "checksums.txt").write_text(checksum + "  " + dest.name + "\n")
        print(dest)


if __name__ == "__main__":
    main()
