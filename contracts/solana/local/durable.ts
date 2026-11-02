import { open, rename, mkdir, readFile } from "node:fs/promises";
import { dirname } from "node:path";
import { randomUUID } from "node:crypto";
import { spawn } from "node:child_process";

export async function atomicJson(path: string, value: unknown) {
  await mkdir(dirname(path), { recursive: true, mode: 0o700 });
  const temporary = `${path}.${randomUUID()}.tmp`;
  const file = await open(temporary, "wx", 0o600);
  try {
    await file.writeFile(JSON.stringify(value) + "\n");
    await file.sync();
  } finally {
    await file.close();
  }
  await rename(temporary, path);
  const directory = await open(dirname(path), "r");
  try {
    await directory.sync();
  } finally {
    await directory.close();
  }
}
export async function optionalJson(path: string): Promise<unknown | undefined> {
  try {
    return JSON.parse(await readFile(path, "utf8"));
  } catch (error) {
    if (error instanceof Error && "code" in error && error.code === "ENOENT")
      return undefined;
    throw error;
  }
}
// flock is held by a tiny Python standard-library helper. Parent death closes
// stdin, which exits the helper and releases the OS lock. Never unlink the lock
// file: two different inodes would allow two simultaneous holders.
export async function withUploadLock<T>(
  path: string,
  action: () => Promise<T>,
  waitMs = 0,
) {
  await mkdir(dirname(path), { recursive: true, mode: 0o700 });
  const child = spawn(
    "python3",
    [
      "-c",
      `import fcntl, os, sys, time
fd = os.open(sys.argv[1], os.O_CREAT | os.O_RDWR, 0o600)
deadline = time.monotonic() + float(sys.argv[2]) / 1000
while True:
    try:
        fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        break
    except BlockingIOError:
        if time.monotonic() >= deadline:
            sys.exit(73)
        time.sleep(0.05)
sys.stdout.write("locked\\n")
sys.stdout.flush()
sys.stdin.buffer.read()
os.close(fd)
`,
      path,
      String(waitMs),
    ],
    { stdio: ["pipe", "pipe", "pipe"] },
  );
  const exited = new Promise<number | null>((resolve, reject) => {
    child.once("error", reject);
    child.once("exit", resolve);
  });
  try {
    await Promise.race([
      new Promise<void>((resolve) =>
        child.stdout.once("data", () => resolve()),
      ),
      exited.then((code) => {
        throw new Error(`header uploader lock unavailable (${code})`);
      }),
    ]);
    return await action();
  } finally {
    child.stdin.end();
    await exited;
  }
}
