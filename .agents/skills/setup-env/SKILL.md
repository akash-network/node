---
name: setup-env
description: "Set up development environment by loading direnv. Must be run before any make targets (tests, builds, linting)."
---
# Instructions

Before running any make target (`make test`, `make bins`, `make lint-go`, etc.), the direnv environment **must** be loaded.
Without it, make will fail with errors about `AKASH_DIRENV_SET` or `AKASH_ROOT` not being set.

## Steps

1. **Allow and load direnv** for the project root. This sets all required variables (`AKASH_ROOT`, `AKASH_DIRENV_SET`, `GOTOOLCHAIN`, `GOPATH`, `GOWORK`, cache paths) adds tool directories to `PATH`, and runs `make cache` to download build tools and wasmvm libraries:
   ```bash
   direnv allow
   ```

2. **Load environment into current shell**:
   ```bash
   eval "$(direnv export bash 2>&1 | grep -v '^direnv:')"
   ```

3. **Validate**:
   ```bash
   [[ "$AKASH_DIRENV_SET" == "1" ]] && echo "direnv loaded" || echo "ERROR: direnv not loaded"
   ```

## Notes

- If direnv fails, check that required system tools are installed: `make`, `unzip`, `wget`, `curl`, `npm`, `jq`, `readlink`, `pv`, `lz4`.
- On macOS, Homebrew `make` (v4+) must be on PATH: `export PATH="$(brew --prefix)/opt/make/libexec/gnubin:$PATH"`.
