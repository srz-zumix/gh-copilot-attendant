// Per-instance dashboard controller: owns the current query, runs the stats
// commands, and notifies subscribers (the SSE stream) when state changes.

import { buildArgv, normalizeQuery, SOURCES } from "./query.mjs";
import { resolveRunner, runStats, RUNNERS } from "./runner.mjs";
import { summarize } from "./summary.mjs";
import { projectFusion } from "./fusion-data.mjs";
import { projectVscode, vscodeFetchArgv } from "./vscode-data.mjs";

function idleResult() {
    return { status: "idle", data: null, argv: null, ranAt: null, durationMs: null, warnings: [], error: null };
}

export class Dashboard {
    /**
     * @param {object} options
     * @param {object} options.query Initial, already normalized query.
     * @param {string} [options.runnerMode]
     * @param {string} options.cwd Working directory the commands run in.
     * @param {(query: object) => unknown} [options.persist]
     * @param {(message: string, level?: string) => void} [options.log]
     * @param {{resolveRunner?: Function, runStats?: Function}} [options.deps] Test seams.
     */
    constructor({ query, runnerMode = "auto", cwd, persist, log, deps = {} }) {
        this.query = normalizeQuery({}, query);
        this.runnerMode = RUNNERS.includes(runnerMode) ? runnerMode : "auto";
        this.cwd = cwd;
        this.persist = persist ?? (() => {});
        this.log = log ?? (() => {});
        this.resolveRunner = deps.resolveRunner ?? resolveRunner;
        this.runStats = deps.runStats ?? runStats;
        this.runnerPromise = null;
        this.runnerInfo = { mode: this.runnerMode, label: null, error: null };
        this.results = Object.fromEntries(SOURCES.map((s) => [s, idleResult()]));
        this.generation = Object.fromEntries(SOURCES.map((s) => [s, 0]));
        this.aborters = {};
        this.pending = {};
        this.listeners = new Set();
        this.closed = false;
        this.sessionCache = null;
        this.vscodeCache = null;
    }

    subscribe(listener) {
        this.listeners.add(listener);
        return () => this.listeners.delete(listener);
    }

    emit() {
        const snapshot = this.snapshot();
        for (const listener of this.listeners) {
            try {
                listener(snapshot);
            } catch {
                // A broken subscriber must not stop the others.
            }
        }
    }

    /** Whether the stored result was produced by the current query. */
    isFresh(source) {
        const result = this.results[source];
        if (!result.argv || result.argv.join("\0") !== buildArgv(this.query, source).join("\0")) return false;
        if (source === "vscode") return this.hasVscodeCache();
        return result.runnerMode === this.runnerMode && result.projection === (this.query.fusionOnly ? "fusion" : "all");
    }

    hasSessionCache() {
        return this.sessionCache?.runnerMode === this.runnerMode &&
            this.sessionCache.argv.join("\0") === buildArgv(this.query, "session").join("\0");
    }

    projectCachedSession() {
        const previous = this.results.session;
        const projection = this.query.fusionOnly ? "fusion" : "all";
        if (!this.hasSessionCache()) {
            if (projection === "fusion" || previous.projection !== projection) {
                this.results.session = {
                    ...previous, data: null, projection, localFiltering: projection === "fusion",
                    fetchArgv: undefined,
                    ...(previous.errorKind === "projection" ? { status: "idle", error: null, errorKind: null } : {}),
                };
            }
            return;
        }
        const cache = this.sessionCache;
        const common = {
            argv: cache.argv, runnerMode: cache.runnerMode, projection,
            ranAt: cache.ranAt, durationMs: cache.durationMs, warnings: cache.warnings,
            fetchArgv: projection === "fusion" ? cache.argv : undefined,
            localFiltering: projection === "fusion",
        };
        const pending = previous.status === "loading" && previous.loadingRunnerMode === this.runnerMode &&
            previous.loadingArgv?.join("\0") === this.fetchArgv("session").join("\0");
        try {
            const data = projection === "fusion" ? projectFusion(cache.data) : cache.data;
            this.results.session = {
                ...previous, ...common, data,
                ...(!pending && cache.fetchError !== undefined
                    ? { status: "error", error: cache.fetchError, errorKind: "fetch" }
                    : previous.errorKind === "projection" ? { status: "ok", error: null, errorKind: null } : {}),
            };
        } catch (error) {
            this.results.session = {
                ...previous, ...common, data: null,
                status: pending ? "loading" : "error",
                error: pending ? null : cache.fetchError !== undefined
                    ? `${error.message}; last refresh failed: ${cache.fetchError}` : error.message,
                errorKind: pending ? null : "projection",
            };
        }
    }

    hasVscodeCache() {
        return this.vscodeCache?.runnerMode === this.runnerMode &&
            this.vscodeCache.argv.join("\0") === vscodeFetchArgv(this.query).join("\0");
    }

    projectCachedVscode() {
        if (!this.hasVscodeCache()) return;
        const cache = this.vscodeCache;
        this.results.vscode = {
            ...this.results.vscode,
            data: projectVscode(cache.data, this.query),
            argv: buildArgv(this.query, "vscode"),
            fetchArgv: cache.argv,
            localFiltering: true,
            ranAt: cache.ranAt, durationMs: cache.durationMs, warnings: cache.warnings,
        };
    }

    snapshot() {
        return {
            query: this.query,
            cwd: this.cwd,
            runner: this.runnerInfo,
            results: Object.fromEntries(
                SOURCES.map((s) => [s, { ...this.results[s], fresh: this.isFresh(s), command: buildArgv(this.query, s) }]),
            ),
        };
    }

    getRunner() {
        if (!this.runnerPromise) {
            this.runnerPromise = this.resolveRunner(this.runnerMode, { cwd: this.cwd }).then(
                (runner) => {
                    this.runnerInfo = { mode: this.runnerMode, label: runner.label, error: null };
                    return runner;
                },
                (error) => {
                    // Allow a later refresh to retry, e.g. after installing the extension.
                    this.runnerPromise = null;
                    this.runnerInfo = { mode: this.runnerMode, label: null, error: error.message };
                    throw error;
                },
            );
        }
        return this.runnerPromise;
    }

    setRunnerMode(mode) {
        if (!RUNNERS.includes(mode) || mode === this.runnerMode) return;
        this.runnerMode = mode;
        this.runnerPromise = null;
        this.runnerInfo = { mode, label: null, error: null };
        this.projectCachedSession();
    }

    /**
     * Applies a query patch. When `refresh` is true the active source is
     * re-run unless its current result already matches the new query.
     */
    async setQuery(patch, { refresh = true, force = false } = {}) {
        this.applyQuery(patch);
        if (!refresh) return this.results[this.query.source];
        return this.ensureFresh({ force });
    }

    /** Validates and stores a query patch synchronously; throws QueryError. */
    applyQuery(patch) {
        this.query = normalizeQuery(patch, this.query);
        this.projectCachedSession();
        this.projectCachedVscode();
        this.persist(this.query);
        this.emit();
        return this.query;
    }

    /** Re-runs the active source unless its result already matches the query. */
    async ensureFresh({ force = false } = {}) {
        const source = this.query.source;
        if (!force && this.isFresh(source) && this.results[source].status === "ok") return this.results[source];
        if (!force && source === "vscode" && this.hasVscodeCache() && this.results[source].status === "error") {
            return this.results[source];
        }
        if (!force && source === "session" && this.hasSessionCache() && this.results[source].status === "error") {
            return this.results[source];
        }
        if (!force && this.results[source].status === "loading" && this.pending[source]) {
            const loading = this.results[source].loadingArgv?.join("\0");
            const sameRunner = this.results[source].loadingRunnerMode === this.runnerMode;
            if (sameRunner && loading === this.fetchArgv(source).join("\0")) return this.refreshing(source);
        }
        return this.refresh(source);
    }

    fetchArgv(source) {
        return source === "vscode" ? vscodeFetchArgv(this.query) : buildArgv(this.query, source);
    }

    async refreshing(source) {
        let current = this.pending[source];
        await current;
        while (this.pending[source] !== current) {
            current = this.pending[source];
            await current;
        }
        return this.results[source];
    }

    /**
     * Runs the stats command for `source` with the current query. A newer
     * refresh for the same source cancels the older one; only the latest
     * generation may publish its result.
     */
    async refresh(source = this.query.source) {
        if (!SOURCES.includes(source)) throw new Error(`unknown source "${source}"`);
        if (this.closed) return this.results[source];
        const generation = ++this.generation[source];
        this.aborters[source]?.abort();
        const controller = new AbortController();
        this.aborters[source] = controller;
        const query = this.query;
        const runnerMode = this.runnerMode;
        const argv = this.fetchArgv(source);
        const previous = this.results[source];
        this.results[source] = {
            ...previous,
            status: "loading",
            error: null,
            errorKind: null,
            loadingArgv: argv,
            loadingRunnerMode: runnerMode,
            startedAt: new Date().toISOString(),
        };
        this.emit();

        const run = (async () => {
            try {
                const runner = await this.getRunner();
                if (controller.signal.aborted) return;
                const { data, warnings, durationMs } = await this.runStats(runner, argv, {
                    cwd: this.cwd,
                    signal: controller.signal,
                });
                if (generation !== this.generation[source] || this.closed) return;
                const ranAt = new Date().toISOString();
                let projected = data;
                let equivalentArgv = argv;
                if (source === "session") {
                    this.sessionCache = { data, argv, ranAt, durationMs, warnings, runnerMode };
                    this.results.session = {
                        status: "ok",
                        data: this.query.fusionOnly ? null : data,
                        argv,
                        runnerMode,
                        projection: this.query.fusionOnly ? "fusion" : "all",
                        localFiltering: this.query.fusionOnly,
                        ranAt,
                        durationMs,
                        warnings,
                        error: null,
                    };
                    this.projectCachedSession();
                } else {
                    projectVscode(data, { ...query, filters: { ...query.filters, vscode: { ...query.filters.vscode, agent: [] } } });
                    const current = argv.join("\0") === this.fetchArgv(source).join("\0") ? this.query : query;
                    projected = projectVscode(data, current);
                    equivalentArgv = buildArgv(current, source);
                    this.vscodeCache = { data, argv, ranAt, durationMs, warnings, runnerMode };
                    this.results[source] = {
                        status: "ok",
                        data: projected,
                        argv: equivalentArgv,
                        fetchArgv: argv,
                        localFiltering: true,
                        ranAt,
                        durationMs,
                        warnings,
                        error: null,
                    };
                }
            } catch (error) {
                if (generation !== this.generation[source] || this.closed || error.aborted) return;
                this.log(`attendant-dashboard: ${source} stats failed: ${error.message}`, "warning");
                if (source === "session" && this.sessionCache?.runnerMode === runnerMode &&
                    this.sessionCache.argv.join("\0") === argv.join("\0")) {
                    this.sessionCache = { ...this.sessionCache, fetchError: error.message };
                }
                this.results[source] = {
                    ...this.results[source],
                    status: "error",
                    error: error.message,
                    errorKind: "fetch",
                };
                if (source === "session") this.projectCachedSession();
            } finally {
                if (generation === this.generation[source]) {
                    delete this.aborters[source];
                    if (!this.closed) this.emit();
                }
            }
        })();
        this.pending[source] = run;
        // A superseding refresh may start while this one runs; wait for the latest.
        return this.refreshing(source);
    }

    /** Compact summary of the latest result for `source`. */
    summary(source = this.query.source) {
        const result = this.results[source];
        return {
            source,
            status: result.status,
            fresh: this.isFresh(source),
            error: result.error,
            command: result.argv ? `gh copilot-attendant ${result.argv.join(" ")}` : null,
            ...(result.fetchArgv ? { fetchCommand: `gh copilot-attendant ${result.fetchArgv.join(" ")}`, localFiltering: true } : {}),
            ranAt: result.ranAt,
            durationMs: result.durationMs,
            warningCount: result.warnings?.length ?? 0,
            query: this.query,
            summary: summarize(source, result.data, 10, this.query),
        };
    }

    close() {
        this.closed = true;
        for (const controller of Object.values(this.aborters)) controller.abort();
        this.aborters = {};
        this.listeners.clear();
    }
}
