import { parseSinkId, ownerBytes } from "../shared/id";
import type { IncomingMessage, ServerResponse } from "node:http";
import { ApiFailure } from "./team-receipt";

const KEPLR_CHROME_ORIGIN =
  "chrome-extension://dmkamcknogkgcdfhhbddcghachkejeap";
const READ_METHODS = new Set([
  "status",
  "health",
  "abci_info",
  "abci_query",
  "block",
  "tx",
  "tx_search",
]);
const MAX_REQUEST_BYTES = 64 * 1024;
const MAX_RESPONSE_BYTES = 2 * 1024 * 1024;

export function logFailure(
  event: string,
  error: unknown,
  fields: Record<string, string | number> = {},
) {
  const describe = (value: unknown) =>
    value instanceof Error ? value.name : typeof value;
  const cause = error instanceof Error ? error.cause : undefined;
  console.error(
    JSON.stringify({
      event,
      ...fields,
      error: describe(error),
      ...(cause === undefined ? {} : { cause: describe(cause) }),
    }),
  );
}

export function extensionOrigins(input = "") {
  const values = [KEPLR_CHROME_ORIGIN, ...input.split(",").filter(Boolean)];
  for (const value of values) {
    if (
      !/^(?:chrome-extension:\/\/[a-p]{32}|moz-extension:\/\/[0-9a-f-]{36})$/.test(
        value,
      )
    )
      throw new Error(
        "AKT_TEAM_EXTENSION_ORIGINS contains an invalid extension origin",
      );
  }
  return new Set(values);
}

export function assertOrigin(
  origin: string | undefined,
  allowed: ReadonlySet<string>,
) {
  if (origin !== undefined && !allowed.has(origin))
    throw new ApiFailure(400, "This request origin is not allowed");
}

export async function readJson(
  request: IncomingMessage,
  limit = MAX_REQUEST_BYTES,
): Promise<unknown> {
  if (
    request.headers["content-type"]?.split(";", 1)[0].trim().toLowerCase() !==
    "application/json"
  )
    throw new ApiFailure(400, "application/json is required");
  const chunks: Buffer[] = [];
  let length = 0;
  for await (const chunk of request) {
    const bytes = Buffer.from(chunk);
    length += bytes.length;
    if (length > limit) throw new ApiFailure(400, "Request is too large");
    chunks.push(bytes);
  }
  try {
    return JSON.parse(Buffer.concat(chunks).toString("utf8"));
  } catch {
    throw new ApiFailure(400, "Invalid JSON request");
  }
}

function object(value: unknown): Record<string, unknown> {
  if (typeof value !== "object" || value === null || Array.isArray(value))
    throw new ApiFailure(400, "A JSON object is required");
  return Object.fromEntries(Object.entries(value));
}

export function rpcRequest(value: unknown) {
  const request = object(value);
  if (
    request.jsonrpc !== "2.0" ||
    !(
      typeof request.id === "string" ||
      (typeof request.id === "number" && Number.isSafeInteger(request.id))
    ) ||
    typeof request.method !== "string" ||
    !(
      READ_METHODS.has(request.method) || request.method === "broadcast_tx_sync"
    )
  )
    throw new ApiFailure(400, "Unsupported JSON-RPC request");
  const params = request.params === undefined ? {} : object(request.params);
  if (request.method === "broadcast_tx_sync") {
    if (
      typeof params.tx !== "string" ||
      params.tx.length === 0 ||
      params.tx.length > 48000 ||
      !/^[A-Za-z0-9+/]+={0,2}$/.test(params.tx) ||
      Object.keys(params).length !== 1
    )
      throw new ApiFailure(400, "Invalid signed transaction");
  }
  if (request.method === "tx_search") {
    if (
      typeof params.query !== "string" ||
      !/^tx\.hash='[A-Fa-f0-9]{64}'$/.test(params.query)
    )
      throw new ApiFailure(400, "Only lookup by transaction hash is supported");
  }
  return { jsonrpc: "2.0", id: request.id, method: request.method, params };
}

export function restPath(url: URL) {
  const path = url.pathname.replace(/^\/rest\/akash/, "");
  const account = "akash1[023456789acdefghjklmnpqrstuvwxyz]{38}";
  if (path.startsWith("/akash/sink/v1/")) {
    try {
      const base = "/akash/sink/v1/";
      const part = path.slice(base.length);
      if (part === "state") {
        if (url.search) throw new Error();
        return path;
      }
      if (part.startsWith("accounts/")) {
        const owner = part.slice(9);
        ownerBytes(owner);
        if (
          [...url.searchParams].some(
            ([key, value]) => key !== "owner" || value !== owner,
          ) ||
          url.searchParams.size > 1
        )
          throw new Error();
        return path;
      }
      if (part.startsWith("transfers/")) {
        const id = parseSinkId(part.slice(10)),
          [owner, height, sequence] = id.split("/");
        const expected = new Map([
          ["id.owner", owner],
          ["id.height", height],
          ["id.sequence", sequence],
        ]);
        const seen = new Set<string>();
        for (const [key, value] of url.searchParams) {
          if (seen.has(key) || expected.get(key) !== value) throw new Error();
          seen.add(key);
        }
        return path;
      }
      if (part !== "transfers" && part !== "disputes") throw new Error();
      const seen = new Set<string>();
      for (const [key, value] of url.searchParams) {
        if (seen.has(key)) throw new Error();
        seen.add(key);
        if (key === "owner" && part === "transfers") ownerBytes(value);
        else if (
          key === "status" &&
          part === "transfers" &&
          /^(0|1|2|4|5)$/.test(value)
        )
          continue;
        else if (
          key === "pagination.limit" &&
          /^[1-9][0-9]{0,2}$/.test(value) &&
          Number(value) <= 500
        )
          continue;
        else if (
          key === "pagination.key" &&
          value.length <= 128 &&
          /^[A-Za-z0-9+/]*={0,2}$/.test(value)
        )
          continue;
        else throw new Error();
      }
      return path + url.search;
    } catch {
      throw new ApiFailure(400, "Unsupported sink query parameter");
    }
  }
  const allowed = new RegExp(
    `^/cosmos/(?:base/tendermint/v1beta1/(?:node_info|syncing|blocks/latest)|base/node/v1beta1/config|auth/v1beta1/accounts/${account}|bank/v1beta1/(?:balances|spendable_balances)/${account}|staking/v1beta1/params)$`,
  );
  if (!allowed.test(path)) throw new ApiFailure(400, "Unsupported REST query");
  const seen = new Set<string>();
  for (const [key, value] of url.searchParams) {
    if (seen.has(key))
      throw new ApiFailure(400, "Duplicate REST query parameter");
    seen.add(key);
    if (!(
      (key === "pagination.limit" &&
        /^[1-9][0-9]{0,3}$/.test(value) &&
        Number(value) <= 1000) ||
      (key === "pagination.key" &&
        value.length <= 512 &&
        /^[A-Za-z0-9+/=]*$/.test(value)) ||
      (key === "denom" && value === "uakt")
    ))
      throw new ApiFailure(400, "Unsupported REST query parameter");
  }
  return path + url.search;
}

async function forward(response: ServerResponse, url: string, body?: string) {
  try {
    const upstream = await fetch(url, {
      method: body === undefined ? "GET" : "POST",
      ...(body === undefined
        ? {}
        : { body, headers: { "Content-Type": "application/json" } }),
      signal: AbortSignal.timeout(12_000),
      redirect: "error",
    });
    if (!upstream.body) throw new Error("empty upstream response");
    const chunks: Uint8Array[] = [];
    let length = 0;
    for await (const chunk of upstream.body) {
      length += chunk.length;
      if (length > MAX_RESPONSE_BYTES)
        throw new Error("upstream response is too large");
      chunks.push(chunk);
    }
    const output = Buffer.concat(chunks);
    JSON.parse(output.toString("utf8"));
    response.statusCode = upstream.status;
    response.setHeader("Content-Type", "application/json");
    response.end(output);
  } catch (error) {
    logFailure("portal-proxy-failed", error, {
      method: body === undefined ? "GET" : "POST",
      upstream: new URL(url).pathname,
    });
    throw new ApiFailure(503, "Akash endpoint is temporarily unavailable");
  }
}

export async function proxyRequest(
  request: IncomingMessage,
  response: ServerResponse,
  url: URL,
  options: {
    akashRpc: string;
    akashRest: string;
    allowedOrigins: ReadonlySet<string>;
  },
) {
  assertOrigin(request.headers.origin, options.allowedOrigins);
  if (request.headers.origin) {
    response.setHeader("Access-Control-Allow-Origin", request.headers.origin);
    response.setHeader("Vary", "Origin");
  }
  const rpc = url.pathname === "/rpc/akash";
  const rpcStatus = url.pathname === "/rpc/akash/status";
  if (rpcStatus && url.search)
    throw new ApiFailure(400, "Status does not accept query parameters");
  if (request.method === "OPTIONS") {
    if (!rpc && !rpcStatus) restPath(url);
    response.setHeader(
      "Access-Control-Allow-Methods",
      rpc ? "POST, OPTIONS" : "GET, OPTIONS",
    );
    response.setHeader("Access-Control-Allow-Headers", "Content-Type");
    response.statusCode = 204;
    response.end();
    return;
  }
  if (rpcStatus) {
    if (request.method !== "GET")
      throw new ApiFailure(400, "Status is read-only");
    await forward(response, `${options.akashRpc}/status`);
  } else if (rpc) {
    if (request.method !== "POST" || url.search)
      throw new ApiFailure(400, "RPC requires POST");
    const body = rpcRequest(await readJson(request));
    await forward(response, options.akashRpc, JSON.stringify(body));
  } else {
    if (request.method !== "GET")
      throw new ApiFailure(400, "REST is read-only");
    await forward(response, `${options.akashRest}${restPath(url)}`);
  }
}
