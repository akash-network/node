---
name: upgrade-changelog
description: "Generates CHANGELOG.md entries for upgrade versions found in upgrades/software/ by parsing init.go and upgrade.go"
---
# Instructions

Generate changelog entries in `upgrades/CHANGELOG.md` for any upgrade version under `upgrades/software/` that doesn't already have an entry.

## Steps

1. **Find semver directories** — list all directories under `upgrades/software/` whose names match `v<major>.<minor>.<patch>` (e.g., `v2.0.0`, `v1.0.0`).

2. **Read `upgrades/CHANGELOG.md`** — identify which versions already have entries by scanning for `##### vX.Y.Z` headings. If a version already has a heading, skip it entirely.

3. **For each new version** (no existing heading), gather data from two files:

   ### 3a. Parse `upgrades/software/<version>/init.go`

   Find all `utypes.RegisterMigration(...)` calls. Each call has the form:
   ```go
   utypes.RegisterMigration(moduleName, version, handlerFn)
   ```
   - `moduleName` is a Go constant (e.g., `dv1.ModuleName`). Resolve it:
     1. Find the import alias (e.g., `dv1 "pkg.akt.dev/go/node/deployment/v1"`)
     2. Search the imported package for `ModuleName` constant definition to get the actual string value
   - `version` is a `uint64` — this is the **from** version. The **to** version is `version + 1`.
   - Record each migration as: `moduleName version -> version+1`

   ### 3b. Parse `upgrades/software/<version>/upgrade.go`

   Find the `StoreLoader()` method. It returns a `*storetypes.StoreUpgrades` struct with optional fields:
   - `Added: []string{...}` — new stores
   - `Renamed: []storetypes.StoreRename{...}` — renamed stores
   - `Deleted: []string{...}` — removed stores

   If `StoreLoader()` returns `nil`, there are no store changes.

   For each store key constant (e.g., `epochstypes.StoreKey`, `ttypes.ModuleName`):
   1. Find the import alias in the file
   2. Search the imported package for the constant definition to get the actual string value

4. **Insert the new entry** in `upgrades/CHANGELOG.md` immediately after the line:
   ```
   Add new upgrades after this line based on the template above
   ```
   followed by `-----`.

   Use this format (newest entries go first, right after the delimiter):

   ```markdown

   ##### vX.Y.Z

   ###### Description

   - Stores
       - added
           - `storeName`: brief description if available
       - renamed
           - `oldName` -> `newName`
       - deleted
           - `storeName`: brief description if available

   - Migrations
       - moduleName `from -> to`
   ```

   **Omission rules** (match existing CHANGELOG style):
   - Omit the entire `Stores` section if there are no added, renamed, or deleted stores
   - Omit `added`/`renamed`/`deleted` subsections individually if empty
   - Omit the entire `Migrations` section if there are no migrations
   - Always include the `###### Description` heading (leave it for the user to fill in)

5. **Report results** — list each version processed and what was added. For skipped versions (already in CHANGELOG), mention they were skipped.

## Important notes

- Store key constants may be named `StoreKey` or `ModuleName` — both are used as store identifiers. Resolve whichever constant appears in the code.
- When resolving Go constants from external packages, search under the Go module cache or use `go doc` if needed. The packages typically follow the pattern `pkg.akt.dev/go/node/<module>/<version>`.
- If a constant cannot be resolved, use the raw Go expression as a placeholder (e.g., `` `epochstypes.StoreKey` ``) and warn the user.
- Multiple versions may need entries — process them all in one run, inserting newest first after the delimiter.
