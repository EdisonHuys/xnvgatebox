"""Download a pinned official test client; never installed into the VPS package."""
import gzip
import hashlib
import json
from pathlib import Path
import urllib.request

root=Path(__file__).resolve().parents[1]
pin=json.loads((root/'tests/mihomo-release.json').read_text())
url='https://github.com/MetaCubeX/mihomo/releases/download/'+pin['version']+'/'+pin['asset']
with urllib.request.urlopen(url,timeout=120) as response: data=response.read(50_000_001)
assert len(data)<50_000_001 and hashlib.sha256(data).hexdigest()==pin['sha256']
dest=root/'tools/bin/mihomo';dest.parent.mkdir(parents=True,exist_ok=True)
dest.write_bytes(gzip.decompress(data));dest.chmod(0o755)
print('Pinned official Mihomo test client installed')
