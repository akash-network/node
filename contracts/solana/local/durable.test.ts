import assert from "node:assert/strict";
import { test } from "node:test";
import { spawn } from "node:child_process";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { once } from "node:events";
import { withUploadLock } from "./durable";

test(
  "payer lock excludes another process and releases after SIGKILL",
  { timeout: 10000 },
  async () => {
    const directory = await mkdtemp(join(tmpdir(), "akt-lock-"));
    const path = join(directory, "payer.lock");
    const child = spawn(
      process.execPath,
      [
        "--import",
        "tsx",
        "--input-type=module",
        "-e",
        `
    import { withUploadLock } from './local/durable.ts';
    await withUploadLock(process.argv[1], async () => {
      process.stdout.write('ready');
      await new Promise(() => {});
    });
  `,
        path,
      ],
      { stdio: ["ignore", "pipe", "inherit"] },
    );
    const exited = once(child, "exit");
    try {
      await once(child.stdout, "data");
      await assert.rejects(
        () => withUploadLock(path, async () => {}),
        /lock unavailable/,
      );
      child.kill("SIGKILL");
      await exited;
      const deadline = Date.now() + 3000;
      for (;;) {
        try {
          assert.equal(
            await withUploadLock(path, async () => "recovered"),
            "recovered",
          );
          break;
        } catch (error) {
          if (Date.now() >= deadline) throw error;
          await new Promise((resolve) => setTimeout(resolve, 20));
        }
      }
    } finally {
      child.kill("SIGKILL");
      await exited;
      await rm(directory, { recursive: true });
    }
  },
);
