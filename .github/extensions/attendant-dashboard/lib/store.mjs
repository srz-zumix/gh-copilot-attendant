// Persists the last dashboard query per (working directory, profile).
//
// Queries are user preferences, so they live under
// `$COPILOT_HOME/extensions/attendant-dashboard/artifacts/` rather than in the
// repository. Keying by working directory keeps a path-specific scope chosen
// in one repository from leaking into another.

import { mkdir, readFile, rename, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

export const EXTENSION_NAME = "attendant-dashboard";

export function artifactsDir(env = process.env) {
    const home = env.COPILOT_HOME || path.join(os.homedir(), ".copilot");
    return path.join(home, "extensions", EXTENSION_NAME, "artifacts");
}

export function storeKey(cwd, profile = "default") {
    return `${path.resolve(cwd)}::${profile}`;
}

export class QueryStore {
    constructor(file = path.join(artifactsDir(), "queries.json")) {
        this.file = file;
        this.writing = Promise.resolve();
    }

    async readAll() {
        try {
            const parsed = JSON.parse(await readFile(this.file, "utf-8"));
            return parsed && typeof parsed === "object" && !Array.isArray(parsed) ? parsed : {};
        } catch {
            // Missing or corrupt files fall back to defaults instead of failing to open.
            return {};
        }
    }

    async load(key) {
        const all = await this.readAll();
        return all[key]?.query ?? null;
    }

    /** Writes are serialized and atomic (temp file + rename). */
    save(key, query) {
        this.writing = this.writing.then(async () => {
            const all = await this.readAll();
            all[key] = { query, updatedAt: new Date().toISOString() };
            await mkdir(path.dirname(this.file), { recursive: true });
            const tmp = `${this.file}.${process.pid}.tmp`;
            await writeFile(tmp, `${JSON.stringify(all, null, 2)}\n`, "utf-8");
            await rename(tmp, this.file);
        }).catch(() => {});
        return this.writing;
    }
}
