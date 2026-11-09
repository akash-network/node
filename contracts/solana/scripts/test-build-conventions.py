#!/usr/bin/env python3
"""Exercise dependency selection and source checkout without live chains."""

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


class BuildConventions(unittest.TestCase):
    def setUp(self):
        CACHE.mkdir(parents=True, exist_ok=True)
        self.temp = tempfile.TemporaryDirectory(prefix="conventions-", dir=CACHE)
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.node = self.root / "node"
        self.source = self.node / "contracts/solana"
        shutil.copytree(SOURCE / "scripts", self.source / "scripts")
        self.cache = self.node / ".cache/solana"
        self.cache.mkdir(parents=True)
        self.upstream = self.root / "upstream"
        self.upstream.mkdir()
        self.git("init", "--quiet")
        (self.upstream / "source.rs").write_text("reviewed source\n")
        self.git("add", ".")
        self.git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
                 "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "fixture")
        self.revision = self.git("rev-parse", "HEAD").strip()
        self.manifests(self.revision, self.revision)

    def git(self, *args):
        return subprocess.check_output(["git", "-C", str(self.upstream), *args], text=True)

    def manifests(self, program, encoder):
        for path, name, revision in (
            ("programs/akt/Cargo.toml", "tendermint-light-client-membership", program),
            ("tools/header-encoder/Cargo.toml", "solana-ibc-borsh-header", encoder),
        ):
            manifest = self.source / path
            manifest.parent.mkdir(parents=True, exist_ok=True)
            manifest.write_text(f'[dependencies]\n{name} = {{ git = "{self.upstream.as_uri()}", rev = "{revision}" }}\n')

    def fetch(self):
        return subprocess.run(["python3", str(self.source / "scripts/fetch-light-client.py")],
                              capture_output=True, text=True, timeout=30)

    def test_checkout_retries_and_refuses_dirty_source(self):
        for _ in range(2):
            result = self.fetch()
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn(self.revision, result.stdout)
        checkout = self.cache / "deps/ibc-contracts"
        self.assertEqual(subprocess.check_output(["git", "-C", str(checkout), "rev-parse", "HEAD"], text=True).strip(), self.revision)
        (checkout / "source.rs").write_text("operator edit\n")
        result = self.fetch()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("differs from its pin", result.stderr)
        self.assertEqual((checkout / "source.rs").read_text(), "operator edit\n")
        self.assertEqual(list(self.cache.glob("ibc-source-*")), [])

    def test_old_archive_is_preserved(self):
        checkout = self.cache / "deps/ibc-contracts"
        checkout.mkdir(parents=True)
        (checkout / ".akt-revision").write_text("old archive\n")
        result = self.fetch()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Preserve or move aside", result.stderr)
        self.assertEqual((checkout / ".akt-revision").read_text(), "old archive\n")

    def test_failed_fetch_cleans_staging(self):
        self.manifests("a" * 40, "a" * 40)
        self.assertNotEqual(self.fetch().returncode, 0)
        self.assertFalse((self.cache / "deps/ibc-contracts").exists())
        self.assertEqual(list(self.cache.glob("ibc-source-*")), [])

    def test_mismatched_program_and_encoder_pins_fail_before_fetch(self):
        self.manifests(self.revision, "a" * 40)
        self.assertIn("same full IBC commit", self.fetch().stderr)
        self.assertFalse((self.cache / "deps").exists())

    def test_sink_uses_selected_workspace_and_ignores_stale_modfile(self):
        stale = self.cache / "node.mod"
        stale.write_text("must never be used\n")
        workspace = self.node / "go.work"
        workspace.write_text("explicit developer workspace\n")
        bin_dir = self.root / "bin"
        bin_dir.mkdir()
        log = self.root / "calls.jsonl"
        stub = '''#!/usr/bin/env python3
import json, os, pathlib, sys
with open(os.environ["CALL_LOG"], "a") as output:
    output.write(json.dumps([pathlib.Path(sys.argv[0]).name, sys.argv[1:], os.getcwd(), os.environ["GOWORK"]]) + "\\n")
'''
        for name in ("go", "npm"):
            path = bin_dir / name
            path.write_text(stub)
            path.chmod(0o755)
        env = {k: v for k, v in os.environ.items() if k not in ("NODE_SINK_DIR",)}
        env.update(PATH=f"{bin_dir}:{os.environ['PATH']}", CALL_LOG=str(log),
                   GOWORK=str(workspace), AKT_NODE_MODFILE=str(stale))
        result = subprocess.run(["bash", str(self.source / "scripts/test-sink.sh")],
                                env=env, capture_output=True, text=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stderr)
        calls = [json.loads(line) for line in log.read_text().splitlines()]
        self.assertEqual([call[0] for call in calls], ["go", "npm", "go"])
        for _, args, cwd, selected in calls:
            self.assertEqual(selected, str(workspace))
            self.assertFalse(any("modfile" in arg for arg in args))
        self.assertEqual(stale.read_text(), "must never be used\n")

    def test_setup_uses_make_and_preserves_reviewed_sdk_archive(self):
        archive = self.cache / "chain-sdk.tgz"
        archive.write_bytes(b"reviewed SDK package")
        (self.source / "scripts/doctor.sh").write_text("exit 0\n")
        bin_dir = self.root / "bin"
        bin_dir.mkdir()
        log = self.root / "commands"
        for name in ("make", "npm"):
            path = bin_dir / name
            path.write_text('#!/usr/bin/env bash\nprintf "%s\\n" "$0 $*" >> "$CALL_LOG"\n')
            path.chmod(0o755)
        env = {k: v for k, v in os.environ.items() if k != "NODE_SINK_DIR"}
        env.update(PATH=f"{bin_dir}:{os.environ['PATH']}", CALL_LOG=str(log))
        result = subprocess.run(["bash", str(self.source / "scripts/setup-local.sh")],
                                env=env, capture_output=True, text=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stderr)
        commands = log.read_text().splitlines()
        self.assertIn(f"{bin_dir}/make -C {self.node} bins", commands)
        self.assertEqual(commands[0], f"{bin_dir}/npm ci")
        self.assertFalse(any("pack" in command or "--prefix" in command or "sinkdev" in command
                             for command in commands))
        self.assertEqual(archive.read_bytes(), b"reviewed SDK package")

    def sdk_fixture(self):
        fixture = self.root / "sdk-source/ts"
        fixture.mkdir(parents=True)
        (fixture / "package.json").write_text('{"name":"sdk-fixture","version":"1.0.0"}\n')
        (fixture / "package.json").chmod(0o644)
        download = self.root / "sdk-source.tar.gz"
        with tarfile.open(download, "w:gz") as archive:
            archive.add(fixture, arcname="chain-sdk/ts")
        self.sdk_lock("sha512-DxLINbtwj7/UafS6Hn5bm5oe8Ppi8vmltOXZ66WtqzC9OTBzKMke5ozL2tIbkUQER0bZWOYPbZgGIqdEDcPgrg==")
        bin_dir = self.root / "bin"
        bin_dir.mkdir()
        commands = {
            "curl": '#!/usr/bin/env bash\nset -eu\ncp "$SDK_FIXTURE" "${!#}"\n',
            "npm": '''#!/usr/bin/env bash
set -eu
test "${SDK_FAIL_BUILD:-0}" = 0
if [[ "$*" == "run build" ]]; then
  mkdir -p dist
  printf 'export const value = 1;\\n' > dist/index.js
  printf '%s' "$PWD" > dist/tsconfig.build.tsbuildinfo
elif [[ "$1" == pack ]]; then
  exec "$REAL_NPM" "$@"
fi
''',
        }
        for name, content in commands.items():
            path = bin_dir / name
            path.write_text(content)
            path.chmod(0o755)
        env = {k: v for k, v in os.environ.items() if k != "NODE_SINK_DIR"}
        env.update(PATH=f"{bin_dir}:{os.environ['PATH']}", SDK_FIXTURE=str(download),
                   REAL_NPM=shutil.which("npm"))
        return env

    def sdk_lock(self, integrity):
        (self.source / "package-lock.json").write_text(json.dumps({
            "packages": {"node_modules/@akashnetwork/chain-sdk": {"integrity": integrity}},
        }))

    def prepare_sdk(self, env):
        return subprocess.run(["bash", str(self.source / "scripts/prepare-sdk.sh")],
                              env=env, capture_output=True, text=True, timeout=30)

    def test_sdk_preparation_is_reproducible_without_incremental_cache(self):
        env = self.sdk_fixture()
        target = self.cache / "chain-sdk.tgz"
        result = self.prepare_sdk(env)
        self.assertEqual(result.returncode, 0, result.stderr)
        first = target.read_bytes()
        with tarfile.open(target) as archive:
            self.assertEqual(archive.getnames(), ["package/dist/index.js", "package/package.json"])
            self.assertEqual(archive.extractfile("package/dist/index.js").read(), b"export const value = 1;\n")
        result = self.prepare_sdk(env)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(first, target.read_bytes())
        self.assertEqual(list(self.cache.glob("sdk.*")), [])

    def test_sdk_failure_preserves_existing_input_and_cleans_staging(self):
        env = self.sdk_fixture()
        target = self.cache / "chain-sdk.tgz"
        target.write_bytes(b"previous input")
        self.sdk_lock("sha512-wrong")
        for fail_build in ("0", "1"):
            with self.subTest(fail_build=fail_build):
                result = self.prepare_sdk({**env, "SDK_FAIL_BUILD": fail_build})
                self.assertNotEqual(result.returncode, 0)
                if fail_build == "0":
                    self.assertIn("differs from package-lock.json", result.stderr)
                self.assertEqual(target.read_bytes(), b"previous input")
                self.assertEqual(list(self.cache.glob("sdk.*")), [])


if __name__ == "__main__":
    unittest.main()
