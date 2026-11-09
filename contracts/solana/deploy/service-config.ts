import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { dirname, isAbsolute, relative, resolve, sep } from "node:path";
import { z } from "zod";
import { parseRelayConfig } from "../local/relay-config";
import { parseConfig, validateMint } from "../reporter/config";
import { MAINNET_GENESIS } from "../shared/networks";

export function checkServiceConfig(
  role: string,
  raw: unknown,
  file: string,
  network: "mainnet" | "testnet" = "mainnet",
) {
  const service = z
    .enum(["relayer", "reporter-preview", "reporter-broadcast"])
    .parse(role);
  const config =
    service === "relayer"
      ? parseRelayConfig(raw, dirname(file))
      : parseConfig(raw);
  if ("mint" in config) validateMint(config);
  const genesis =
    "genesisHash" in config ? config.genesisHash : config.solanaGenesisHash;
  if (network === "mainnet") {
    assert.equal(
      config.akashChainId,
      "akashnet-2",
      "production requires akashnet-2",
    );
    assert.equal(
      genesis,
      MAINNET_GENESIS,
      "production requires Solana mainnet-beta",
    );
    assert.notEqual(
      config.programId.toBase58(),
      "FfFAPUKqjf68ggj1HvpyzMHFDRnT2cFAdZYYQb7mK9i5",
      "replace the development AKT program ID",
    );
  } else {
    assert.notEqual(
      config.akashChainId,
      "akashnet-2",
      "testnet mode refuses Akash mainnet",
    );
    assert.notEqual(
      genesis,
      MAINNET_GENESIS,
      "testnet mode refuses Solana mainnet",
    );
  }
  if (service === "relayer") {
    const paths = z
      .object({
        headerEncoder: z.string().refine(isAbsolute),
        payerKeyFile: z.string().refine(isAbsolute),
        stateDir: z.string().refine(isAbsolute),
      })
      .parse(raw);
    assert.ok(
      !resolve(paths.stateDir).split(sep).includes(".cache"),
      "production state must survive release/cache cleanup",
    );
    const fromRelease = relative(
      resolve(__dirname, "../../.."),
      resolve(paths.stateDir),
    );
    assert.ok(
      fromRelease.startsWith(`..${sep}`) || isAbsolute(fromRelease),
      "production state must live outside the release directory",
    );
  } else if (service === "reporter-broadcast") {
    assert.ok(
      "externalSigner" in config && (config.externalSigner || config.keyFile),
      "broadcast requires keyFile or externalSigner",
    );
  }
}

if (require.main === module) {
  void (async () => {
    const [role, file] = process.argv.slice(2);
    assert.ok(
      role && file && isAbsolute(file),
      "expected service and absolute config path",
    );
    checkServiceConfig(role, JSON.parse(await readFile(file, "utf8")), file);
  })().catch((error: unknown) => {
    process.stderr.write(
      `${error instanceof Error ? error.message : "invalid service configuration"}\n`,
    );
    process.exitCode = 1;
  });
}
