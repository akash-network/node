import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { once } from "node:events";
import { mkdtemp, writeFile } from "node:fs/promises";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

test(
  "maintained testnet server serves configured portal and fails unavailable state without diagnostics",
  { timeout: 20000 },
  async (t) => {
    const dist = await mkdtemp(join(tmpdir(), "sink-v2-portal-"));
    await writeFile(
      join(dist, "index.html"),
      "<!doctype html><title>separately built portal</title>",
    );
    const reservation = createServer();
    reservation.listen(0, "127.0.0.1");
    await once(reservation, "listening");
    const address = reservation.address();
    assert.ok(address && typeof address !== "string");
    await new Promise<void>((resolve, reject) =>
      reservation.close((error) => (error ? reject(error) : resolve())),
    );
    const origin = `http://127.0.0.1:${address.port}`;
    const child = spawn(
      process.execPath,
      ["--import", "tsx", "local/team.ts"],
      {
        cwd: process.cwd(),
        env: {
          ...process.env,
          AKT_TEAM_TESTNET: "1",
          AKT_TEAM_HOST: "127.0.0.1",
          AKT_TEAM_PORT: String(address.port),
          AKT_TEAM_ORIGIN: origin,
          AKT_PORTAL_DIST: dist,
          AKT_LOCAL_RUN: `receipt-test-v2-${process.pid}-${address.port}`,
        },
        stdio: ["ignore", "pipe", "pipe"],
      },
    );
    t.after(async () => {
      if (child.exitCode === null && child.signalCode === null) {
        child.kill("SIGTERM");
        await once(child, "exit");
      }
    });
    await new Promise<void>((resolve, reject) => {
      const timeout = setTimeout(
        () => reject(new Error("testnet server startup timeout")),
        12000,
      );
      child.once("exit", (code) => {
        clearTimeout(timeout);
        reject(new Error(`testnet server exited ${code}`));
      });
      child.stdout.on("data", (data) => {
        if (String(data).includes("testnet-receipt-adapter")) {
          clearTimeout(timeout);
          resolve();
        }
      });
    });
    const home = await fetch(origin);
    assert.equal(home.status, 200);
    assert.match(await home.text(), /separately built portal/);
    assert.match(
      home.headers.get("content-security-policy") ?? "",
      /frame-ancestors 'none'/,
    );
    for (const query of ["id=7", "id=bad", "tx=bad", "id=bad&tx=bad"]) {
      const response = await fetch(`${origin}/api/receipt?${query}`);
      assert.equal(response.status, 400);
    }
    const unavailable = await fetch(`${origin}/api/config`);
    assert.equal(unavailable.status, 503);
    const text = await unavailable.text();
    assert.equal(
      text,
      '{"error":"Network verification is temporarily unavailable. Try again shortly."}',
    );
    assert.ok(
      !text.includes(dist) &&
        !text.includes("stack") &&
        !text.includes("reporter1.json"),
    );
    const forbidden = await fetch(`${origin}/rpc/akash`, {
      method: "POST",
      headers: {
        "content-type": "application/json",
        origin: "https://attacker.invalid",
      },
      body: JSON.stringify({ jsonrpc: "2.0", id: 1, method: "status" }),
    });
    assert.equal(forbidden.status, 400);
  },
);
