import assert from "node:assert/strict";
import { access, rm } from "node:fs/promises";
import { dirname, join } from "node:path";
import test from "node:test";
import { CACHE } from "./common";
import { temporaryDirectory } from "./test-workspace";

test("concurrent fixtures stay in the repository cache and clean up after failure", async () => {
  const directories = await Promise.all([
    temporaryDirectory("temp-path-"),
    temporaryDirectory("temp-path-"),
  ]);
  try {
    assert.notEqual(directories[0], directories[1]);
    for (const directory of directories) {
      assert.equal(dirname(directory), join(CACHE, "tests"));
      await access(directory);
    }
    await assert.rejects(async () => {
      const directory = await temporaryDirectory("temp-failure-");
      try {
        throw new Error("fixture failed");
      } finally {
        await rm(directory, { recursive: true, force: true });
        await assert.rejects(access(directory), { code: "ENOENT" });
      }
    }, /fixture failed/);
  } finally {
    await Promise.all(
      directories.map((directory) =>
        rm(directory, { recursive: true, force: true }),
      ),
    );
  }
  for (const directory of directories)
    await assert.rejects(access(directory), { code: "ENOENT" });
});
