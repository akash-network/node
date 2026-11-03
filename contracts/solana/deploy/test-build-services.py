#!/usr/bin/env python3
"""Exercise packaging/cleanup with fixture compiler and package manager."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import unittest

SOURCE = Path(__file__).resolve().parents[1]
CACHE = SOURCE.parents[1] / ".cache/solana/tests"
COMMIT = "a" * 40

STUB = r'''#!/usr/bin/env python3
import json, os, pathlib, shutil, sys
name = pathlib.Path(sys.argv[0]).name
args = sys.argv[1:]
if name == "uname":
    print("Linux" if args == ["-s"] else "Linux fixture-architecture")
elif name == "git":
    if "rev-parse" in args: print("a" * 40)
    elif "archive" in args: sys.stdout.buffer.write(pathlib.Path(os.environ["FIXTURE_SOURCE"]).read_bytes())
elif name == "node":
    if args[:2] != ["--import", "tsx"]:
        os.execv(os.environ["REAL_NODE"], [os.environ["REAL_NODE"], *args])
elif name == "cargo":
    target = pathlib.Path(os.environ["CARGO_TARGET_DIR"]) / "release"
    target.mkdir(parents=True)
    (target / "akt-header-encoder").write_text("fixture-encoder")
elif name == "npm":
    if os.environ.get("FAIL_INSTALL") == "yes": sys.exit(37)
    if args[0] == "ci":
        modules = pathlib.Path("node_modules")
        modules.mkdir()
        (modules / "fixture").write_text("locked-dependency")
        (modules / "development-only").write_text("compiler/test tooling")
    elif args == ["prune", "--omit=dev"]:
        pathlib.Path("node_modules/development-only").unlink()
    else: print("fixture npm: " + " ".join(args))
'''


class ServiceArchiveTests(unittest.TestCase):
    def setUp(self):
        CACHE.mkdir(parents=True, exist_ok=True)
        self.temp = tempfile.TemporaryDirectory(prefix="service-package-", dir=CACHE)
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.source = self.root / "contracts/solana"
        self.source.mkdir(parents=True)
        for name in ("deploy", "scripts", "tools/verifiable-build", "idls"):
            (self.source / name).mkdir(parents=True)
        shutil.copy2(SOURCE / "deploy/build-services.sh", self.source / "deploy")
        shutil.copy2(SOURCE / "scripts/workspace.sh", self.source / "scripts")
        self.cache = self.root / ".cache/solana"
        self.cache.mkdir(parents=True)
        (self.cache / "chain-sdk.tgz").write_text("fixture-sdk")
        build = self.root / "build"
        build.mkdir()
        for name in ("akt_solana", "ics07_tendermint", "access_manager"):
            (build / f"{name}.json").write_text("{}\n")
            (build / f"{name}.ts").write_text("// fixture type\n")
            (self.source / f"idls/{name}.json").write_text("{}\n")
        (build / "build-info.txt").write_text(f"commit {COMMIT}\nupstream {COMMIT}\n")
        self.manifest = self.root / "manifest.json"
        self.manifest.write_text(json.dumps({"sourceCommit": COMMIT, "buildDirectory": "build"}))
        archive = self.root / "source.tar"
        with tarfile.open(archive, "w") as output:
            output.add(self.source, arcname="contracts/solana")
        bin_dir = self.root / "bin"
        bin_dir.mkdir()
        for name in ("uname", "git", "node", "cargo", "npm"):
            path = bin_dir / name
            path.write_text(STUB.replace("#!/usr/bin/env python3", "#!" + os.path.realpath(os.sys.executable), 1))
            path.chmod(0o755)
        self.env = {k: v for k, v in os.environ.items() if k not in (
            "NODE_SINK_DIR", "CHAIN_SDK_DIR", "SOLANA_CACHE", "SOLANA_DEPS", "CARGO_TARGET_DIR")}
        self.env.update(PATH=str(bin_dir) + os.pathsep + os.environ["PATH"],
                        REAL_NODE=shutil.which("node"), FIXTURE_SOURCE=str(archive))
        self.output = self.root / "release/services.tar.gz"

    def run_build(self, fail=False):
        return subprocess.run(["bash", str(self.source / "deploy/build-services.sh"),
                               str(self.manifest), str(self.output)],
                              env={**self.env, "FAIL_INSTALL": "yes" if fail else "no"},
                              capture_output=True, text=True, timeout=30)

    def test_package_layout_and_refusal_to_replace(self):
        result = self.run_build()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        with tarfile.open(self.output) as archive:
            files = set(archive.getnames())
        for name in ("bin/akt-header-encoder", ".cache/solana/chain-sdk.tgz",
                     ".cache/solana/target/idl/akt_solana.json",
                     ".cache/solana/target/types/akt_solana.ts",
                     ".cache/solana/deps/ibc-contracts/ibc-solana/target/types/ics07_tendermint.ts",
                     "contracts/solana/node_modules/fixture", "service-build-info.txt", "launch-manifest.json"):
            self.assertIn("./" + name, files)
        self.assertNotIn("./contracts/solana/node_modules/development-only", files)
        self.assertNotIn("./.cache/solana/target/release", files)
        self.assertNotIn("./.cache/solana/deps/ibc-contracts/README.md", files)
        original = self.output.read_bytes()
        self.assertNotEqual(self.run_build().returncode, 0)
        self.assertEqual(self.output.read_bytes(), original)
        self.assertEqual(list(self.cache.glob("services.*")), [])

    def test_install_failure_leaves_no_release_or_staging_tree(self):
        result = self.run_build(fail=True)
        self.assertEqual(result.returncode, 37, result.stdout + result.stderr)
        self.assertFalse(self.output.exists())
        self.assertEqual(list(self.cache.glob("services.*")), [])


if __name__ == "__main__":
    unittest.main()
