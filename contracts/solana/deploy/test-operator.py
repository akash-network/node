"""Exercise upload dispatch without signing or contacting a network."""

import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


REPO = Path(__file__).resolve().parents[1]
ROOT = REPO.parent.parent
NODE = shutil.which("node")
BIN = REPO / "deploy/operator-bin"


class OperatorCommands(unittest.TestCase):
    def setUp(self):
        cache = ROOT / ".cache/solana"
        cache.mkdir(parents=True, exist_ok=True)
        self.temporary = tempfile.TemporaryDirectory(prefix="operator-test.", dir=cache)
        self.addCleanup(self.temporary.cleanup)
        self.work = Path(self.temporary.name)
        (self.work / "bin").mkdir()
        shutil.copytree(REPO / "deployments/mainnet/build", self.work / "build")
        shutil.copyfile(REPO / "deployments/mainnet/manifest.json", self.work / "manifest.json")
        self.manifest = json.loads((self.work / "manifest.json").read_text())
        self.env = {
            **os.environ,
            "PATH": f"{self.work / 'bin'}:{BIN}:{os.environ['PATH']}",
            "MANIFEST": str(self.work / "manifest.json"),
            "BUILD_DIR": str(self.work / "build"),
            "RELEASE_DIR": str(self.work),
            "CEREMONY": str(self.work / "ceremony.json"),
            "PROGRAM_KEYS": str(self.work / "keys"),
            "CEREMONY_PUB": self.manifest["ceremonyAuthority"],
            "GENESIS": self.manifest["solanaGenesisHash"],
            "RPC": "http://127.0.0.1:1",
            "OPERATOR_TEST_NODE": NODE,
            "OPERATOR_TEST_MODE": "normal",
            "OPERATOR_TEST_TRACE": str(self.work / "trace.json"),
        }
        # Only transaction dispatch and preflight transport are substituted.
        # The release checker, argument construction and byte verifier are real.
        stub = '''#!/usr/bin/env python3
import json, os, pathlib, shutil, sys
name=pathlib.Path(sys.argv[0]).name
args=sys.argv[1:]
mode=os.environ['OPERATOR_TEST_MODE']
m=json.loads(pathlib.Path(os.environ['MANIFEST']).read_text())
if name=='node':
    if args[:4]==['--import','tsx','deploy/programs.ts','preflight']:
        print(json.dumps({'sourceCommit':m['sourceCommit'],'missing':[] if mode=='occupied' else list(m['programs'])}))
    else:
        os.execv(os.environ['OPERATOR_TEST_NODE'],[os.environ['OPERATOR_TEST_NODE'],*args])
elif name=='solana-keygen':
    p=pathlib.Path(args[-1]).name
    print('11111111111111111111111111111111' if mode=='wrong-key' else m['ceremonyAuthority'] if p=='ceremony.json' else m['programs'][p.removesuffix('-keypair.json')])
elif 'genesis-hash' in args:
    print('wrong-network' if mode=='wrong-network' else m['solanaGenesisHash'])
elif 'deploy' in args:
    pathlib.Path(os.environ['OPERATOR_TEST_TRACE']).write_text(json.dumps(args))
    print('PRIVATE_RECOVERY_TEST_SENTINEL')
    sys.exit(7 if mode=='upload-failure' else 0)
elif args[:2]==['program','dump']:
    program=next(k for k,v in m['programs'].items() if v in args)
    shutil.copyfile(pathlib.Path(os.environ['BUILD_DIR'])/(program+'.so'),args[-1])
elif args[:2]==['program','show']:
    print(json.dumps({'authority':m['ceremonyAuthority']}))
else:
    raise RuntimeError('unexpected command: '+repr(args))
'''
        for name in ["node", "solana", "solana-keygen"]:
            path = self.work / "bin" / name
            path.write_text(stub)
            path.chmod(0o700)

    def run_command(self, *args):
        return subprocess.run(
            [str(BIN / "deploy_one"), *args], env=self.env,
            cwd=self.work, capture_output=True, text=True, timeout=30,
        )

    def assert_no_upload(self, result):
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse((self.work / "trace.json").exists())

    def test_preview_never_uploads(self):
        result = self.run_command("--check", "access_manager")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("No transaction sent", result.stdout)
        self.assertFalse((self.work / "trace.json").exists())

    def test_existing_program_is_not_redeployed(self):
        self.env["OPERATOR_TEST_MODE"] = "occupied"
        result = self.run_command("akt")
        self.assert_no_upload(result)
        self.assertIn("already deployed", result.stderr)

    def test_wrong_network_is_rejected(self):
        self.env["OPERATOR_TEST_MODE"] = "wrong-network"
        self.assert_no_upload(self.run_command("akt"))

    def test_wrong_key_is_rejected(self):
        self.env["OPERATOR_TEST_MODE"] = "wrong-key"
        self.assert_no_upload(self.run_command("akt"))

    def test_unreviewed_build_directory_is_rejected(self):
        self.env["BUILD_DIR"] = str(REPO / "deployments/mainnet/build")
        self.assert_no_upload(self.run_command("akt"))

    def test_upload_arguments_and_private_log(self):
        result = self.run_command("akt")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        args = json.loads((self.work / "trace.json").read_text())
        self.assertEqual(args[args.index("--max-len") + 1], "1040040")
        self.assertEqual(args[args.index("--program-id") + 1], str(self.work / "keys/akt-keypair.json"))
        self.assertEqual(args[args.index("--upgrade-authority") + 1], self.env["CEREMONY"])
        self.assertEqual(args[args.index("--url") + 1], self.env["RPC"])
        self.assertIn(self.manifest["programs"]["akt"], result.stdout)
        self.assertNotIn("PRIVATE_RECOVERY_TEST_SENTINEL", result.stdout + result.stderr)
        logs = list((self.work / "logs").iterdir())
        self.assertEqual(len(logs), 1)
        self.assertEqual(logs[0].stat().st_mode & 0o777, 0o600)
        self.assertIn("PRIVATE_RECOVERY_TEST_SENTINEL", logs[0].read_text())

    def test_failed_upload_preserves_private_recovery_output(self):
        self.env["OPERATOR_TEST_MODE"] = "upload-failure"
        result = self.run_command("akt")
        self.assertEqual(result.returncode, 7)
        self.assertIn("Upload failed", result.stderr)
        self.assertNotIn("PRIVATE_RECOVERY_TEST_SENTINEL", result.stdout + result.stderr)
        self.assertIn("PRIVATE_RECOVERY_TEST_SENTINEL", next((self.work / "logs").iterdir()).read_text())


if __name__ == "__main__":
    unittest.main()
