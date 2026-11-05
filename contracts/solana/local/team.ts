import { web3 } from "@coral-xyz/anchor";
import tokenMetadata from "../metadata/akt.json";
import { ownerBytes } from "../shared/id";
import assert from "node:assert/strict";
import { existsSync } from "node:fs";
import { readFile, rename } from "node:fs/promises";
import { createServer, type IncomingMessage } from "node:http";
import { extname, join, resolve, sep } from "node:path";
import { AkashClient } from "../reporter/akash";
import {
  AKASH_GRPC_PORT,
  AKASH_REST_PORT,
  AKASH_RPC,
  AKASH_RPC_PORT,
  CHAIN,
  PROGRAM_ID,
  ROOT,
  SOLANA_RPC_PORT,
  cosmosSigner,
  fee,
  json,
  object,
  pda,
  provider,
  save,
} from "./common";
import { processes, running } from "./stack";
import {
  assertOrigin,
  extensionOrigins,
  logFailure,
  proxyRequest,
} from "./team-proxy";
import {
  ApiFailure,
  bridgeConfig,
  bridgeReceipt,
  receiptLookup,
  walletHistoryAddress,
  walletTransfers,
} from "./team-receipt";

const externalHost = process.env.AKT_TEAM_HOST;
assert.ok(
  externalHost && /^[a-zA-Z0-9.-]+$/.test(externalHost),
  "AKT_TEAM_HOST is required",
);
const host = externalHost;
const publicSolanaPort = Number(
  process.env.AKT_TEAM_SOLANA_RPC_PORT ?? SOLANA_RPC_PORT,
);
assert.ok(
  Number.isSafeInteger(publicSolanaPort) &&
    publicSolanaPort >= 1024 &&
    publicSolanaPort < 65535,
  "invalid AKT_TEAM_SOLANA_RPC_PORT",
);
const port = Number(process.env.AKT_TEAM_PORT ?? 18080);
assert.ok(
  Number.isSafeInteger(port) && port >= 1024 && port <= 65535,
  "invalid AKT_TEAM_PORT",
);
const configuredOrigin = new URL(
  process.env.AKT_TEAM_ORIGIN ?? `http://127.0.0.1:${port}`,
);
assert.ok(
  (configuredOrigin.protocol === "https:" ||
    (configuredOrigin.protocol === "http:" &&
      ["127.0.0.1", "localhost"].includes(configuredOrigin.hostname))) &&
    configuredOrigin.pathname === "/" &&
    !configuredOrigin.search &&
    !configuredOrigin.hash &&
    !configuredOrigin.username &&
    !configuredOrigin.password,
  "AKT_TEAM_ORIGIN must be HTTPS or loopback HTTP",
);
const webOrigins = new Set([configuredOrigin.origin, `http://${host}:8080`]);
const proxyOrigins = new Set([
  ...webOrigins,
  ...extensionOrigins(process.env.AKT_TEAM_EXTENSION_ORIGINS),
]);
let funding = false;

async function body(request: IncomingMessage): Promise<unknown> {
  assert.equal(
    request.headers["content-type"],
    "application/json",
    "application/json required",
  );
  assertOrigin(request.headers.origin, webOrigins);
  const chunks: Buffer[] = [];
  let length = 0;
  for await (const chunk of request) {
    const bytes = Buffer.from(chunk);
    length += bytes.length;
    assert.ok(length <= 2048, "request too large");
    chunks.push(bytes);
  }
  return JSON.parse(Buffer.concat(chunks).toString("utf8"));
}

async function fund(input: unknown) {
  const request = object(input);
  assert.ok(
    request.chain === "akash" || request.chain === "solana",
    "chain must be akash or solana",
  );
  assert.ok(typeof request.address === "string", "address required");
  const { chain, address } = request;
  if (chain === "akash") {
    ownerBytes(address);
    const init = object(await json(join(ROOT, "initialized.json")));
    assert.notEqual(
      address,
      init.sinkAddress,
      "fund a wallet, then deposit with MsgSink",
    );
  } else {
    const key = new web3.PublicKey(address);
    assert.equal(key.toBase58(), address);
    assert.ok(
      web3.PublicKey.isOnCurve(key.toBytes()) &&
        !key.equals(web3.PublicKey.default),
      "wallet public key required",
    );
  }
  assert.ok(!funding, "another funding request is in progress; retry shortly");
  funding = true;
  try {
    const path = join(ROOT, "team-faucet.json");
    const today = new Date().toISOString().slice(0, 10);
    const previous = existsSync(path) ? object(await json(path)) : {};
    const counts = previous.day === today ? object(previous.counts) : {};
    const key = `${chain}:${address}`;
    const count = counts[key] ?? 0;
    assert.ok(
      typeof count === "number" && Number.isInteger(count) && count < 5,
      "daily wallet allowance reached",
    );
    const total = previous.day === today ? previous.total : 0;
    assert.ok(
      typeof total === "number" && Number.isInteger(total) && total < 100,
      "daily faucet allowance reached",
    );
    await save(`${path}.tmp`, {
      day: today,
      total: total + 1,
      counts: { ...counts, [key]: count + 1 },
    });
    await rename(`${path}.tmp`, path);
    if (chain === "akash") {
      const signer = await cosmosSigner("sender");
      try {
        const tx = await signer.client.sendTokens(
          signer.address,
          address,
          [{ denom: "uakt", amount: "100000000" }],
          fee,
          "AKT bridge team testnet faucet",
        );
        assert.equal(tx.code, 0, tx.rawLog);
        return {
          chain,
          address,
          amount: "100 AKT",
          transaction: tx.transactionHash,
        };
      } finally {
        signer.client.disconnect();
      }
    }
    const wallet = await provider();
    const transaction = await wallet.connection.requestAirdrop(
      new web3.PublicKey(address),
      2 * web3.LAMPORTS_PER_SOL,
    );
    await wallet.connection.confirmTransaction(transaction, "confirmed");
    return { chain, address, amount: "2 SOL", transaction };
  } finally {
    funding = false;
  }
}

async function status() {
  const akash = new AkashClient(AKASH_RPC, CHAIN, 60);
  try {
    const wallet = await provider();
    const state = await akash.state();
    const entries = (await processes()).filter((p) => p.name !== "testnetify");
    const services = await Promise.all(
      entries.map(async (p) => ({ name: p.name, running: await running(p) })),
    );
    const reconciliation = existsSync(join(ROOT, "reconciliation.json"))
      ? await json(join(ROOT, "reconciliation.json"))
      : null;
    const initialized = object(await json(join(ROOT, "initialized.json")));
    return {
      checkedAt: new Date().toISOString(),
      chainId: CHAIN,
      akashHeight: state.height,
      solanaSlot: await wallet.connection.getSlot("finalized"),
      solanaGenesisHash: await wallet.connection.getGenesisHash(),
      mint: pda("mint").toBase58(),
      program: PROGRAM_ID.toBase58(),
      sinkAddress: initialized.sinkAddress,
      endpoints: {
        solana: `http://${host}:${publicSolanaPort}`,
        solanaWs: `ws://${host}:${publicSolanaPort + 1}`,
        akash: `http://${host}:${AKASH_RPC_PORT}`,
        akashRest: `http://${host}:${AKASH_REST_PORT}`,
        akashGrpc: `${host}:${AKASH_GRPC_PORT}`,
      },
      services,
      totals: Object.fromEntries(
        Object.entries(state.totals).map(([key, value]) => [
          key,
          typeof value === "bigint" ? value.toString() : value,
        ]),
      ),
      reconciliation,
    };
  } finally {
    akash.close();
  }
}

assert.equal(
  process.env.AKT_TEAM_TESTNET,
  "1",
  "receipt adapter requires explicit AKT_TEAM_TESTNET=1",
);
assert.ok(
  process.env.AKT_PORTAL_DIST,
  "AKT_PORTAL_DIST must name the separately built portal dist",
);
const portalDist = resolve(process.env.AKT_PORTAL_DIST);
const contentTypes = new Map([
  [".html", "text/html; charset=utf-8"],
  [".js", "application/javascript"],
  [".css", "text/css"],
  [".svg", "image/svg+xml"],
  [".png", "image/png"],
  [".woff2", "font/woff2"],
  [".ico", "image/x-icon"],
]);

const server = createServer(async (request, response) => {
  response.setHeader("Cache-Control", "no-store");
  response.setHeader("X-Content-Type-Options", "nosniff");
  response.setHeader("Referrer-Policy", "same-origin");
  response.setHeader(
    "Content-Security-Policy",
    "default-src 'self'; script-src 'self'; style-src 'self'; font-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'",
  );
  try {
    const url = new URL(request.url ?? "/", configuredOrigin);
    if (
      request.method === "GET" &&
      url.pathname === "/token/akt.json" &&
      !url.search
    ) {
      response.setHeader("Content-Type", "application/json");
      response.setHeader("Access-Control-Allow-Origin", "*");
      response.end(
        JSON.stringify({
          ...tokenMetadata,
          description: "AKT on the private Akash testnet. No monetary value.",
          external_url: configuredOrigin.origin,
        }),
      );
      return;
    }
    if (
      url.pathname === "/rpc/akash" ||
      url.pathname === "/rpc/akash/status" ||
      url.pathname.startsWith("/rest/akash/")
    ) {
      await proxyRequest(request, response, url, {
        akashRpc: AKASH_RPC,
        akashRest: `http://127.0.0.1:${AKASH_REST_PORT}`,
        allowedOrigins: proxyOrigins,
      });
      return;
    }
    if (
      request.method === "GET" &&
      !url.search &&
      !url.pathname.startsWith("/api/")
    ) {
      const relative =
        url.pathname === "/"
          ? "index.html"
          : decodeURIComponent(url.pathname).slice(1);
      const asset = resolve(portalDist, relative);
      if (
        asset.startsWith(portalDist + sep) &&
        existsSync(asset) &&
        contentTypes.has(extname(asset))
      ) {
        response.setHeader(
          "Content-Type",
          contentTypes.get(extname(asset)) ?? "application/octet-stream",
        );
        response.end(await readFile(asset));
        return;
      }
    }
    let value: unknown;
    if (request.method === "GET" && request.url === "/api/status")
      value = await status();
    else if (request.method === "POST" && request.url === "/api/faucet")
      value = await fund(await body(request));
    else if (request.method === "GET" && request.url === "/api/config")
      value = await bridgeConfig();
    else if (request.method === "GET" && url.pathname === "/api/receipt") {
      value = await bridgeReceipt(receiptLookup(url.searchParams));
    } else if (request.method === "GET" && url.pathname === "/api/transfers") {
      value = await walletTransfers(walletHistoryAddress(url.searchParams));
    }
    response.setHeader("Content-Type", "application/json");
    if (value === undefined) {
      response.statusCode = 404;
      response.end('{"error":"not found"}');
    } else {
      const encoded = JSON.stringify(value, (_, v: unknown) =>
        typeof v === "bigint" ? v.toString() : v,
      );
      assert.ok(
        Buffer.byteLength(encoded) <= 2 * 1024 * 1024,
        "API response exceeds size bound",
      );
      response.end(encoded);
    }
  } catch (error) {
    const faucet = request.url === "/api/faucet";
    response.statusCode =
      error instanceof ApiFailure ? error.status : faucet ? 400 : 503;
    if (!(error instanceof ApiFailure))
      logFailure("portal-request-failed", error, {
        method: request.method ?? "",
        path: (request.url ?? "").split("?", 1)[0].slice(0, 200),
        status: response.statusCode,
      });
    response.setHeader("Content-Type", "application/json");
    response.end(
      JSON.stringify({
        error:
          error instanceof ApiFailure
            ? error.message
            : faucet
              ? "Faucet request failed. Check the address and daily allowance, then check the wallet before retrying."
              : "Network verification is temporarily unavailable. Try again shortly.",
      }),
    );
  }
});
server.requestTimeout = 15_000;
server.headersTimeout = 10_000;
server.listen(port, "127.0.0.1", () =>
  console.log(
    JSON.stringify({
      event: "testnet-receipt-adapter",
      address: `127.0.0.1:${port}`,
    }),
  ),
);
