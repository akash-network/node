---
name: vanity
description: "Registers node major version vanity URL in the sibling vanity repo"
---
# Instructions

Register the current node major version as a vanity URL in the `github.com/com/akash-network/vanity` repo.

## Steps

1. **Read the major version** from `go.mod` in this repo. Extract the version suffix from the module path (e.g. `pkg.akt.dev/node/v2` → `v2`). If the module path has no version suffix (v0/v1), abort — no vanity entry is needed.

2. **Get the current git branch name** using `git rev-parse --abbrev-ref HEAD`.

3. **Open `vanity/vangen.json`** and check if a repository entry with `"prefix": "node/vN"` already exists (where `vN` is the version from step 1). If it exists, inform the user and abort.

4. **Add a new entry** to the `repositories` array in `vanity/vangen.json`:
```json
{
  "prefix": "node/vN",
  "type": "git",
  "main": true,
  "url": "https://github.com/akash-network/node",
  "source": {
    "home": "https://github.com/akash-network/node",
    "dir": "https://github.com/akash-network/node/tree/BRANCH{/dir}",
    "file": "https://github.com/akash-network/node/blob/BRANCH{/dir}/{file}#L{line}"
  },
  "website": {
    "url": "https://github.com/akash-network/node"
  }
}
```
Replace `vN` with the detected version and `BRANCH` with the detected branch name. Place the entry after the last existing `node` entry.

5. **Run `make vangen`** in `vanity` to regenerate HTML files.

6. **Verify** that `vanity/node/vN/index.html` was created and contains the correct `go-import` and `go-source` meta tags pointing to the detected branch. Also verify that existing `vanity/node/index.html` and any `vanity/node/vN/index.html` is unchanged.
