#!/usr/bin/env python3
"""Check out the same immutable IBC source that Cargo uses."""

from pathlib import Path
import subprocess
import tempfile
import tomllib


SOURCE = Path(__file__).resolve().parents[1]
CACHE = SOURCE.parents[1] / ".cache/solana"


def dependency(manifest, name):
    with (SOURCE / manifest).open("rb") as source:
        entry = tomllib.load(source)["dependencies"][name]
    return entry["git"], entry["rev"]


def git(path, *args):
    return subprocess.check_output(["git", "-C", str(path), *args], text=True).strip()


def main():
    repo, revision = dependency(
        "programs/akt/Cargo.toml", "tendermint-light-client-membership"
    )
    encoder = dependency("tools/header-encoder/Cargo.toml", "solana-ibc-borsh-header")
    if ((repo, revision) != encoder or len(revision) != 40 or
            any(c not in "0123456789abcdef" for c in revision)):
        raise SystemExit("Program and encoder must pin the same full IBC commit.")

    destination = CACHE / "deps/ibc-contracts"
    if destination.exists():
        if (not (destination / ".git").is_dir() or
                git(destination, "rev-parse", "HEAD") != revision):
            raise SystemExit(f"Preserve or move aside the old IBC checkout before setup: {destination}")
        if (git(destination, "remote", "get-url", "origin") != repo or
                git(destination, "status", "--porcelain")):
            raise SystemExit(f"IBC checkout differs from its pin; inspect it before rebuilding: {destination}")
    else:
        destination.parent.mkdir(parents=True, exist_ok=True)
        with tempfile.TemporaryDirectory(prefix="ibc-source-", dir=CACHE) as temporary:
            stage = Path(temporary) / "source"
            stage.mkdir()
            git(stage, "init", "--quiet")
            git(stage, "remote", "add", "origin", repo)
            git(stage, "fetch", "--quiet", "--depth=1", "origin", revision)
            git(stage, "checkout", "--quiet", "--detach", revision)
            stage.rename(destination)
    print(f"upstream-repository {repo}")
    print(f"upstream {revision}")


if __name__ == "__main__":
    main()
