#!/usr/bin/env python3
"""Check project scratch paths and EXIT cleanup without downloads or installs."""

from concurrent.futures import ThreadPoolExecutor
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


SOURCE = Path(__file__).resolve().parents[1]
SCRATCH = SOURCE.parents[1] / ".cache/solana/tests"
ANCHOR = b'''#!/usr/bin/env bash
if [[ "${CHECK_FAIL:-}" == version ]]; then exit 31; fi
echo stub-anchor
# Complete download marker.
'''

STUB = r'''#!/usr/bin/env python3
import json, os, pathlib, shutil, subprocess, sys, tarfile, time
name = pathlib.Path(sys.argv[0]).name
args = sys.argv[1:]
cache = pathlib.Path(os.environ["CHECK_CACHE"])
source = pathlib.Path(os.environ["CHECK_SOURCE"])
home_bin = pathlib.Path.home() / ".cargo/bin"
with open(os.environ["CHECK_LOG"], "a") as log:
    log.write(json.dumps([name, args]) + "\n")
if name == "mktemp":
    template = pathlib.Path(args[-1])
    adjacent = template.parent == home_bin and template.name == ".anchor.XXXXXX"
    if not template.is_relative_to(cache) and not adjacent:
        sys.exit("temporary path outside repository cache: " + str(template))
    result = subprocess.run([os.environ["REAL_MKTEMP"], *args], capture_output=True, text=True)
    if result.returncode:
        sys.stderr.write(result.stderr)
        sys.exit(result.returncode)
    with open(os.environ["CHECK_TEMPS"], "a") as log:
        log.write(result.stdout)
    sys.stdout.write(result.stdout)
elif name == "curl":
    if os.environ.get("CHECK_FAIL") == "curl": sys.exit(23)
    shutil.copyfile(os.environ["CHECK_DOWNLOAD"], args[args.index("-o") + 1])
elif name == "python3":
    if args[0] == "-":
        sys.stdin.read()
        if os.environ.get("CHECK_FAIL") == "checksum": sys.exit(24)
    else:
        os.execv(os.environ["REAL_PYTHON"], [os.environ["REAL_PYTHON"], *args])
elif name == "install":
    dst = pathlib.Path(args[-1])
    if dst.parent == home_bin:
        assert dst.name.startswith(".anchor."), dst
        if os.environ.get("CHECK_FAIL") == "install":
            dst.write_bytes(b"partial copy")
            sys.exit(29)
        if os.environ.get("CHECK_FAIL") == "truncated":
            dst.write_bytes(pathlib.Path(args[-2]).read_bytes().split(b"# Complete")[0])
            dst.chmod(0o755)
            sys.exit(0)
    os.execv(os.environ["REAL_INSTALL"], ["install", *args])
elif name == "mv":
    if args[-1] == str(home_bin / "anchor"):
        src = pathlib.Path(args[-2])
        assert src.parent == home_bin, src
        assert src.stat().st_dev == home_bin.stat().st_dev
        assert src.read_bytes() == pathlib.Path(os.environ["CHECK_DOWNLOAD"]).read_bytes()
        if os.environ.get("CHECK_FAIL") == "rename": sys.exit(30)
        if os.environ.get("CHECK_BARRIER"):
            barrier = pathlib.Path(os.environ["CHECK_BARRIER"])
            (barrier / src.name).write_text(str(src))
            deadline = time.monotonic() + 10
            while len(list(barrier.iterdir())) < 2:
                if time.monotonic() > deadline: sys.exit("concurrent candidate timed out")
                time.sleep(0.01)
    os.execv(os.environ["REAL_MV"], ["mv", *args])
elif name == "tar":
    os.execv(os.environ["REAL_TAR"], ["tar", *args])
elif name == "solana":
    if os.environ.get("CHECK_FAIL") == "solana": sys.exit(27)
    if "dump" in args:
        pathlib.Path(args[-1]).write_bytes(b"program\0\0")
    else:
        print('{"authority":"none"}')
elif name == "git":
    if args[0] == "rev-parse":
        print("a" * 40)
    else:
        archive = pathlib.Path(args[args.index("-o") + 1])
        names = args[args.index("-o") + 3:]
        with tarfile.open(archive, "w") as out:
            for member in names:
                out.add(source / member, arcname=member)
elif name == "docker":
    if os.environ.get("CHECK_FAIL") == args[0]: sys.exit(28)
    if args[0] == "run":
        mounts = [args[i + 1] for i, a in enumerate(args) if a == "-v"]
        archive = pathlib.Path(next(m.split(":")[0] for m in mounts if ":/input/" in m))
        output = pathlib.Path(next(m.split(":")[0] for m in mounts if ":/output" in m))
        container = archive.parent / "container"
        solana = container / "contracts/solana"
        solana.mkdir(parents=True)
        with tarfile.open(archive) as source_tar:
            assert "scripts/workspace.sh" in source_tar.getnames(), "workspace helper missing from container archive"
            source_tar.extractall(solana, filter="data")
            assert "scripts/fetch-light-client.py" in source_tar.getnames()
        (output / "toolchain.txt").write_text("stub-toolchain\n")
        (output / "SHA256SUMS").write_text("stub-checksums\n")
    elif args[0] == "image": print("stub-image")
else:
    sys.exit("unexpected stub: " + name)
'''


class TemporaryPaths(unittest.TestCase):
    def setUp(self):
        SCRATCH.mkdir(parents=True, exist_ok=True)
        self.temporary = tempfile.TemporaryDirectory(prefix="paths-", dir=SCRATCH)
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.source = self.root / "node/contracts/solana"
        self.cache = self.root / "node/.cache/solana"
        for directory in ("scripts", "tools/verifiable-build", "tools/header-encoder"):
            shutil.copytree(SOURCE / directory, self.source / directory)
        (self.source / "deploy").mkdir()
        shutil.copy2(SOURCE / "deploy/verify-program.sh", self.source / "deploy")
        (self.source / "idls").mkdir()
        for name in ("ics07_tendermint", "access_manager"):
            (self.source / f"idls/{name}.json").write_text("{}\n")
        for name in ("Anchor.toml", "Cargo.toml", "Cargo.lock", "rust-toolchain.toml"):
            (self.source / name).touch()
        (self.source / "programs").mkdir()
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self.home = self.root / "home"
        self.home.mkdir()
        self.home_bin = self.home / ".cargo/bin"
        self.anchor = self.home_bin / "anchor"
        self.download = self.root / "anchor-download"
        self.download.write_bytes(ANCHOR)
        self.log = self.root / "calls.jsonl"
        self.temps = self.root / "temps.txt"
        self.env = dict(os.environ)
        for name in ("NODE_SINK_DIR", "CHAIN_SDK_DIR", "SOLANA_CACHE", "SOLANA_DEPS", "SOLANA_RUN", "CARGO_TARGET_DIR"):
            self.env.pop(name, None)
        for name in ("mktemp", "python3", "install", "mv", "tar"):
            self.env["REAL_" + ("PYTHON" if name == "python3" else name.upper())] = shutil.which(name)
        # An absolute interpreter avoids recursively invoking the python3 stub.
        stub = STUB.replace("#!/usr/bin/env python3", "#!" + self.env["REAL_PYTHON"], 1)
        for name in ("mktemp", "curl", "python3", "install", "mv", "tar", "solana", "git", "docker"):
            path = self.bin / name
            path.write_text(stub)
            path.chmod(0o755)
        self.env.update(PATH=str(self.bin) + os.pathsep + os.environ["PATH"],
                        HOME=str(self.home), CHECK_DOWNLOAD=str(self.download),
                        CHECK_CACHE=str(self.cache), CHECK_SOURCE=str(self.source),
                        CHECK_LOG=str(self.log), CHECK_TEMPS=str(self.temps))

    def assert_cleaned(self, paths):
        for path in paths:
            temporary = Path(path)
            self.assertTrue(temporary.is_relative_to(self.cache) or
                            (temporary.parent == self.home_bin and temporary.name.startswith(".anchor.")), path)
            self.assertFalse(temporary.exists(), "scratch survived EXIT: " + path)
        self.assertEqual(list(self.home_bin.glob(".anchor.*")), [])

    def entry_identity(self, path):
        info = path.lstat()
        return info.st_ino, info.st_mode, info.st_size, info.st_mtime_ns, info.st_ctime_ns

    def run_script(self, script, *args, failure=""):
        previous = self.temps.read_text().splitlines() if self.temps.exists() else []
        result = subprocess.run(["bash", str(self.source / script), *args],
                                cwd=self.root, env={**self.env, "CHECK_FAIL": failure},
                                capture_output=True, text=True, timeout=30)
        codes = {"": 0, "curl": 23, "checksum": 24,
                 "solana": 27, "build": 28, "run": 28, "install": 29,
                 "truncated": 1, "rename": 30, "version": 31}
        self.assertEqual(result.returncode, codes[failure], result.stdout + result.stderr)
        self.assertTrue(self.temps.exists(), "script did not create scratch")
        created = self.temps.read_text().splitlines()[len(previous):]
        self.assertTrue(created, "script reused scratch")
        self.assert_cleaned(created)
        return result

    def test_anchor(self):
        for failure in ("curl", "checksum", ""):
            with self.subTest(failure=failure):
                self.run_script("scripts/install-anchor.sh", failure=failure)
                if failure:
                    self.assertFalse(self.anchor.exists())
                else:
                    self.assertEqual(self.anchor.read_bytes(), ANCHOR)
                    self.assertEqual(self.anchor.stat().st_mode & 0o777, 0o755)

    def test_anchor_replaces_avm_symlink(self):
        avm = self.root / "avm"
        avm.write_bytes(b"old shared AVM executable\n")
        avm.chmod(0o755)
        before = self.entry_identity(avm)
        self.home_bin.mkdir(parents=True)
        self.anchor.symlink_to(avm)
        self.run_script("scripts/install-anchor.sh")
        self.assertFalse(self.anchor.is_symlink())
        self.assertEqual(self.anchor.read_bytes(), ANCHOR)
        self.assertEqual(self.anchor.stat().st_mode & 0o777, 0o755)
        self.assertEqual(avm.read_bytes(), b"old shared AVM executable\n")
        self.assertEqual(self.entry_identity(avm), before)

    def test_anchor_failures_preserve_installed_entry(self):
        avm = self.root / "avm"
        avm.write_bytes(b"old shared AVM executable\n")
        avm.chmod(0o755)
        self.home_bin.mkdir(parents=True)
        for symlink in (False, True):
            for failure in ("curl", "checksum", "install", "truncated", "version", "rename"):
                with self.subTest(symlink=symlink, failure=failure):
                    self.anchor.unlink(missing_ok=True)
                    if symlink:
                        self.anchor.symlink_to(avm)
                    else:
                        self.anchor.write_bytes(b"old standalone Anchor\n")
                        self.anchor.chmod(0o755)
                    entry = self.entry_identity(self.anchor)
                    contents = self.anchor.read_bytes()
                    target = self.entry_identity(avm)
                    self.log.write_text("")
                    self.run_script("scripts/install-anchor.sh", failure=failure)
                    self.assertEqual(self.anchor.is_symlink(), symlink)
                    self.assertEqual(self.entry_identity(self.anchor), entry)
                    self.assertEqual(self.anchor.read_bytes(), contents)
                    self.assertEqual(avm.read_bytes(), b"old shared AVM executable\n")
                    self.assertEqual(self.entry_identity(avm), target)
                    if symlink:
                        self.assertEqual(self.anchor.readlink(), avm)
                    calls = [json.loads(line) for line in self.log.read_text().splitlines()]
                    moves = [args for name, args in calls if name == "mv"]
                    self.assertEqual(len(moves), 1 if failure == "rename" else 0)

    def test_concurrent_anchor_installation(self):
        barrier = self.root / "candidate-barrier"
        barrier.mkdir()
        def install():
            return subprocess.run(["bash", str(self.source / "scripts/install-anchor.sh")],
                                  cwd=self.root, env={**self.env, "CHECK_BARRIER": str(barrier)},
                                  capture_output=True, text=True, timeout=30)
        with ThreadPoolExecutor(max_workers=2) as pool:
            results = list(pool.map(lambda _: install(), range(2)))
        for result in results:
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        paths = self.temps.read_text().splitlines()
        self.assertEqual(len(paths), 4)
        self.assertEqual(len(set(paths)), 4)
        candidates = {path for path in paths if Path(path).parent == self.home_bin}
        self.assertEqual(len(candidates), 2)
        self.assertEqual({path.read_text() for path in barrier.iterdir()}, candidates)
        self.assert_cleaned(paths)
        self.assertEqual(self.anchor.read_bytes(), ANCHOR)
        self.assertEqual(self.anchor.stat().st_mode & 0o777, 0o755)

    def test_verify_program(self):
        binary = self.root / "program.so"
        binary.write_bytes(b"program")
        for failure in ("solana", ""):
            with self.subTest(failure=failure):
                self.run_script("deploy/verify-program.sh", "--url", "stub-rpc", "stub-id", str(binary), failure=failure)

    def test_verifiable_build(self):
        for failure in ("build", "run", ""):
            with self.subTest(failure=failure):
                self.run_script("scripts/verifiable-build.sh", str(self.cache / ("output-" + failure)), failure=failure)

    def test_concurrent_verification(self):
        binary = self.root / "program.so"
        binary.write_bytes(b"program")
        def verify():
            return subprocess.run(["bash", str(self.source / "deploy/verify-program.sh"),
                                   "--url", "stub-rpc", "stub-id", str(binary)],
                                  cwd=self.root, env=self.env, capture_output=True, text=True, timeout=30)
        with ThreadPoolExecutor(max_workers=2) as pool:
            results = list(pool.map(lambda _: verify(), range(2)))
        for result in results:
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        paths = self.temps.read_text().splitlines()
        self.assertEqual(len(paths), 2)
        self.assertEqual(len(set(paths)), 2)
        for path in paths:
            self.assertTrue(Path(path).is_relative_to(self.cache), path)
            self.assertFalse(Path(path).exists(), path)


if __name__ == "__main__":
    unittest.main()
