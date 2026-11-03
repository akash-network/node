import { mkdir, mkdtemp } from "node:fs/promises";
import { join } from "node:path";
import { CACHE } from "./common";

export async function temporaryDirectory(prefix: string) {
  const root = join(CACHE, "tests");
  await mkdir(root, { recursive: true });
  return mkdtemp(join(root, prefix));
}
