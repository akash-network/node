import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";

const result = spawnSync("npm", ["audit", "--omit=dev", "--json"], {
  encoding: "utf8",
  maxBuffer: 10 * 1024 * 1024,
});
assert.ok(!result.error && [0, 1].includes(result.status), "npm audit failed");
const cache = new URL("../../../.cache/solana/", import.meta.url);
mkdirSync(cache, { recursive: true });
writeFileSync(new URL("operator-npm-audit.json", cache), result.stdout);
const report = JSON.parse(result.stdout);
assert.ok(!report.error && report.metadata && report.vulnerabilities);
const reviewed = new Set(
  readFileSync(
    new URL("../docs/operator-releases.md", import.meta.url),
    "utf8",
  ).match(/GHSA-[\w-]+/g),
);
const unreviewed = new Set();
for (const dependency of Object.values(report.vulnerabilities)) {
  for (const advisory of dependency.via) {
    if (typeof advisory === "string") continue;
    const id = advisory.url.split("/").at(-1);
    if (!reviewed.has(id)) unreviewed.add(id);
  }
}
assert.equal(
  unreviewed.size,
  0,
  `Unreviewed operator dependency advisories: ${[...unreviewed].join(", ")}`,
);
console.log("Dependency advisories match the scoped operator review.");
