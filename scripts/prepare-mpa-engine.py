"""Build a relocatable, locked Python engine resource for MyGo packages."""

from pathlib import Path
import hashlib
import json
import os
import subprocess
import tarfile
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]
PROJECT = ROOT / "desktop/mpa-engine"
OUTPUT = ROOT / "desktop/build/mpa-engine.tar.gz"


def run(*args):
    return subprocess.check_output(args, text=True, cwd=ROOT).strip()


def main():
    subprocess.run(["uv", "sync", "--locked", "--no-dev", "--no-install-project", "--project", str(PROJECT)], check=True, cwd=ROOT)
    python = PROJECT / ".venv/bin/python"
    metadata = json.loads(run(str(python), "-c", "import sys,sysconfig,json,platform; print(json.dumps({'base':sys.base_prefix,'site':sysconfig.get_paths()['purelib'],'platform':platform.system(),'arch':platform.machine()}))"))
    sources = {name: (PROJECT / name).read_bytes() for name in ("pyproject.toml", "uv.lock", "engine.py")}
    digest = hashlib.sha256(Path(__file__).read_bytes())
    for content in sources.values():
        digest.update(content)
    digest.update(json.dumps(metadata, sort_keys=True).encode())
    stamp = OUTPUT.with_suffix(".sha256")
    if OUTPUT.is_file() and stamp.is_file() and stamp.read_text() == digest.hexdigest():
        print("MPA lifecycle engine is current")
        return
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="mpa-engine-", dir=OUTPUT.parent) as temporary:
        destination = Path(temporary)
        (destination / "engine.py").write_bytes(sources["engine.py"])
        (destination / "manifest.json").write_text(json.dumps({"veadk": "1.1.19", "python": "3.12", "platform": metadata["platform"], "architecture": metadata["arch"], "fingerprint": digest.hexdigest(), "sdkZip": True}))
        site = Path(metadata["site"])
        # Generated SDK service modules are pure Python with no package-relative
        # files. Keep every module, but import them from a zip to avoid creating
        # almost 29,000 tiny files on the user's first launch. Core/native code,
        # package metadata, licenses and all other dependencies remain ordinary
        # files. The system site .pth is honored by isolated Python and its runner.
        sdk_packages = sorted(p for p in site.glob("volcenginesdk*") if p.is_dir() and p.name != "volcenginesdkcore" and not p.name.endswith(".dist-info"))
        sdk_names = set()
        with zipfile.ZipFile(destination / "openmpa-sdk.zip", "w", compression=zipfile.ZIP_DEFLATED, compresslevel=1) as sdk:
            for package in sdk_packages:
                contents = []
                portable = True
                for base, directories, filenames in os.walk(package):
                    directories[:] = sorted(name for name in directories if name != "__pycache__")
                    for name in sorted(filenames):
                        if name.endswith(".pyc"):
                            continue
                        path = Path(base) / name
                        if path.suffix != ".py":
                            portable = False
                            break
                        data = path.read_bytes()
                        if b"__file__" in data or b"resource_filename" in data:
                            portable = False
                            break
                        contents.append((str(path.relative_to(site)), data))
                    if not portable:
                        break
                if portable:
                    sdk_names.add(package.name)
                    for name, data in contents:
                        sdk.writestr(name, data)
        (destination / "openmpa-sdk.pth").write_text("openmpa-sdk.zip\n")

        def filter_entry(entry):
            if "__pycache__" in entry.name.split("/") or entry.name.endswith(".pyc"):
                return None
            return entry

        def filter_base(entry):
            if entry.name == "python/lib/python3.12/site-packages":
                return None
            return filter_entry(entry)

        def filter_site(entry):
            parts = entry.name.split("/")
            if len(parts) > 4 and parts[4] in sdk_names:
                return None
            return filter_entry(entry)

        with tarfile.open(destination / "engine.tar.gz", "w:gz", compresslevel=1, dereference=True) as archive:
            archive.add(metadata["base"], arcname="python", filter=filter_base)
            archive.add(site, arcname="python/lib/python3.12/site-packages", filter=filter_site)
            for name in ("openmpa-sdk.zip", "openmpa-sdk.pth"):
                archive.add(destination / name, arcname="python/lib/python3.12/site-packages/" + name)
            archive.add(destination / "engine.py", arcname="engine.py")
            archive.add(destination / "manifest.json", arcname="manifest.json")
        os.replace(destination / "engine.tar.gz", OUTPUT)
    stamp.write_text(digest.hexdigest())
    print(f"Bundled MPA lifecycle engine: {OUTPUT.stat().st_size // (1024 * 1024)} MiB; {len(sdk_names)} generated SDK packages remain intact in zip")


if __name__ == "__main__":
    main()
