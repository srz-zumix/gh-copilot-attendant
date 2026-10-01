// Resolves and runs the `gh-copilot-attendant` executable.
//
// Resolution order for the "auto" runner:
//   1. `$COPILOT_ATTENDANT_BIN`, when set, is executed directly.
//   2. The installed gh extension (`gh copilot-attendant`).
//   3. A binary built from this repository's sources, used only when the gh
//      extension is not installed (or `gh` itself is missing).
// The "gh" and "source" runners force options 2 and 3 respectively.

import { spawn } from "node:child_process";
import { access, mkdir, readFile } from "node:fs/promises";
import { createHash } from "node:crypto";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

export const RUNNERS = ["auto", "gh", "source"];
export const MODULE_PATH = "github.com/srz-zumix/gh-copilot-attendant";

const EXTENSION_DIR = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
/** Repository root, assuming the extension lives at `.github/extensions/<name>/`. */
export const REPO_ROOT = path.resolve(EXTENSION_DIR, "..", "..", "..");

const MAX_OUTPUT = 64 * 1024 * 1024;

export class RunError extends Error {
    constructor(message, { stderr = "", code = null, aborted = false } = {}) {
        super(message);
        this.name = "RunError";
        this.stderr = stderr;
        this.code = code;
        this.aborted = aborted;
    }
}

/**
 * Spawns a command in its own process group and collects its output.
 *
 * Aborting `signal` (or hitting `timeoutMs`) terminates the whole group, so
 * grandchildren such as the binary `gh` launches do not outlive the request.
 * The group first receives SIGTERM; if it has not exited after `killGraceMs`
 * it is killed forcibly, and after another `killGraceMs` the promise rejects
 * even if some process still holds the output pipes open.
 *
 * @param {string} cmd
 * @param {string[]} args
 * @param {{cwd?: string, signal?: AbortSignal, timeoutMs?: number, killGraceMs?: number, env?: object}} [options]
 * @returns {Promise<{stdout: string, stderr: string, code: number}>}
 */
export function execCapture(cmd, args, { cwd, signal, timeoutMs = 120_000, killGraceMs = 3_000, env } = {}) {
    return new Promise((resolve, reject) => {
        if (signal?.aborted) {
            reject(new RunError("run was cancelled", { aborted: true }));
            return;
        }
        let child;
        try {
            child = spawn(cmd, args, {
                cwd,
                env: env ?? process.env,
                stdio: ["ignore", "pipe", "pipe"],
                detached: process.platform !== "win32",
            });
        } catch (error) {
            reject(new RunError(`failed to start ${cmd}: ${error.message}`));
            return;
        }
        const stdout = [];
        const stderr = [];
        let size = 0;
        let settled = false;
        let reason = null;

        let killTimer = null;
        let deadlineTimer = null;

        // The group is signalled even when the direct child has already exited,
        // because a grandchild may still be running and holding the pipes open.
        const signalTree = (sig) => {
            if (!child.pid) return;
            try {
                if (process.platform !== "win32") {
                    process.kill(-child.pid, sig);
                } else if (sig === "SIGKILL") {
                    spawn("taskkill", ["/pid", String(child.pid), "/T", "/F"], { stdio: "ignore", windowsHide: true }).on(
                        "error",
                        () => {},
                    );
                } else {
                    child.kill(sig);
                }
            } catch {
                // The group may already be gone.
            }
        };
        const killTree = () => {
            if (settled || killTimer) return;
            signalTree("SIGTERM");
            killTimer = setTimeout(() => {
                signalTree("SIGKILL");
                deadlineTimer = setTimeout(() => {
                    child.stdout.destroy();
                    child.stderr.destroy();
                    settle(null);
                }, killGraceMs);
            }, killGraceMs);
        };
        const onAbort = () => {
            reason = "cancelled";
            killTree();
        };
        const timer = setTimeout(() => {
            reason = "timeout";
            killTree();
        }, timeoutMs);
        signal?.addEventListener("abort", onAbort, { once: true });

        const finish = (fn) => {
            if (settled) return;
            settled = true;
            clearTimeout(timer);
            clearTimeout(killTimer);
            clearTimeout(deadlineTimer);
            signal?.removeEventListener("abort", onAbort);
            fn();
        };
        const settle = (code) => {
            const out = Buffer.concat(stdout).toString("utf-8");
            const err = Buffer.concat(stderr).toString("utf-8");
            finish(() => {
                if (reason === "cancelled") reject(new RunError("run was cancelled", { stderr: err, aborted: true }));
                else if (reason) reject(new RunError(`run aborted: ${reason}`, { stderr: err }));
                else resolve({ stdout: out, stderr: err, code: code ?? -1 });
            });
        };

        child.stdout.on("data", (chunk) => {
            size += chunk.length;
            if (size > MAX_OUTPUT) {
                reason = "output too large";
                killTree();
                return;
            }
            stdout.push(chunk);
        });
        child.stderr.on("data", (chunk) => {
            if (stderr.reduce((n, c) => n + c.length, 0) < 1024 * 1024) stderr.push(chunk);
        });
        child.on("error", (error) => {
            finish(() => reject(new RunError(`failed to start ${cmd}: ${error.message}`, { code: error.code })));
        });
        child.on("close", (code) => settle(code));
    });
}

/** Whether a failed `gh <ext>` invocation means the extension is not installed. */
export function isMissingExtension(result) {
    return /unknown command "copilot-attendant"/.test(result?.stderr ?? "");
}

async function isRepoCheckout(root) {
    try {
        const gomod = await readFile(path.join(root, "go.mod"), "utf-8");
        return new RegExp(`^module\\s+${MODULE_PATH.replace(/[.]/g, "\\.")}\\s*$`, "m").test(gomod);
    } catch {
        return false;
    }
}

async function buildFromSource(root) {
    if (!(await isRepoCheckout(root))) {
        throw new RunError(`cannot build from source: ${root} is not a ${MODULE_PATH} checkout`);
    }
    const hash = createHash("sha256").update(root).digest("hex").slice(0, 12);
    const dir = path.join(os.tmpdir(), `attendant-dashboard-${hash}`);
    await mkdir(dir, { recursive: true });
    const bin = path.join(dir, process.platform === "win32" ? "gh-copilot-attendant.exe" : "gh-copilot-attendant");
    const result = await execCapture("go", ["build", "-o", bin, "."], { cwd: root, timeoutMs: 300_000 });
    if (result.code !== 0) {
        throw new RunError(`go build failed (exit ${result.code})`, { stderr: result.stderr, code: result.code });
    }
    return { cmd: bin, prefix: [], label: "source build" };
}

async function probeGh() {
    try {
        const result = await execCapture("gh", ["copilot-attendant", "--version"], { timeoutMs: 30_000 });
        if (result.code === 0) {
            return { ok: true, version: result.stdout.trim() };
        }
        return { ok: false, missing: isMissingExtension(result), stderr: result.stderr };
    } catch (error) {
        return { ok: false, missing: error.code === "ENOENT", stderr: error.message };
    }
}

/**
 * Resolves the executable for the requested runner mode.
 *
 * @param {"auto"|"gh"|"source"} [mode]
 * @param {{repoRoot?: string, env?: object}} [options]
 * @returns {Promise<{cmd: string, prefix: string[], label: string}>}
 */
export async function resolveRunner(mode = "auto", { repoRoot = REPO_ROOT, env = process.env } = {}) {
    if (!RUNNERS.includes(mode)) throw new RunError(`runner must be one of ${RUNNERS.join(", ")}`);
    if (mode === "source") return buildFromSource(repoRoot);
    if (mode === "auto" && env.COPILOT_ATTENDANT_BIN) {
        const bin = env.COPILOT_ATTENDANT_BIN;
        await access(bin).catch(() => {
            throw new RunError(`COPILOT_ATTENDANT_BIN does not exist: ${bin}`);
        });
        return { cmd: bin, prefix: [], label: bin };
    }
    const probe = await probeGh();
    if (probe.ok) return { cmd: "gh", prefix: ["copilot-attendant"], label: `gh ${probe.version}` };
    if (mode === "auto" && probe.missing) return buildFromSource(repoRoot);
    throw new RunError("gh copilot-attendant is not available", { stderr: probe.stderr });
}

/** Extracts structured-log warnings (slog text format) from stderr. */
export function parseWarnings(stderr) {
    const warnings = [];
    for (const line of String(stderr).split(/\r?\n/)) {
        if (!/\blevel=WARN\b/.test(line)) continue;
        const msg = line.match(/\bmsg="((?:[^"\\]|\\.)*)"/)?.[1] ?? line;
        warnings.push(msg);
    }
    return warnings;
}

/** Returns the last few non-empty stderr lines that are not warnings. */
function errorTail(stderr) {
    return String(stderr)
        .split(/\r?\n/)
        .filter((l) => l.trim() && !/\blevel=WARN\b/.test(l))
        .slice(-8)
        .join("\n");
}

/**
 * Runs one `stats` command and parses its JSON output.
 *
 * @param {{cmd: string, prefix: string[]}} runner
 * @param {string[]} argv
 * @param {{cwd?: string, signal?: AbortSignal, timeoutMs?: number}} [options]
 */
export async function runStats(runner, argv, options = {}) {
    const started = Date.now();
    const result = await execCapture(runner.cmd, [...runner.prefix, ...argv], options);
    const warnings = parseWarnings(result.stderr);
    if (result.code !== 0) {
        const tail = errorTail(result.stderr);
        throw new RunError(tail || `exited with code ${result.code}`, { stderr: result.stderr, code: result.code });
    }
    let data;
    try {
        data = JSON.parse(result.stdout);
    } catch (error) {
        throw new RunError(`could not parse JSON output: ${error.message}`, { stderr: result.stderr });
    }
    return { data, warnings, durationMs: Date.now() - started };
}
