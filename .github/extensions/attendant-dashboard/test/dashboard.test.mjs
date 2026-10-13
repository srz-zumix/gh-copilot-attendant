import { test } from "node:test";
import assert from "node:assert/strict";
import { Dashboard } from "../lib/dashboard.mjs";
import { normalizeQuery } from "../lib/query.mjs";
import { vscodeDataset } from "./fixtures/vscode.mjs";

function deferred() {
    let resolve;
    let reject;
    const promise = new Promise((res, rej) => {
        resolve = res;
        reject = rej;
    });
    return { promise, resolve, reject };
}

function makeDashboard(runStats, extra = {}) {
    return new Dashboard({
        query: normalizeQuery(),
        cwd: "/repo",
        deps: {
            resolveRunner: async () => ({ cmd: "fake", prefix: [], label: "fake" }),
            runStats,
        },
        ...extra,
    });
}

test("a superseded run cannot overwrite the newer result", async () => {
    const calls = [];
    const dashboard = makeDashboard((runner, argv, { signal }) => {
        const d = deferred();
        signal.addEventListener("abort", () => d.reject(Object.assign(new Error("cancelled"), { aborted: true })));
        calls.push({ argv, d, signal });
        return d.promise;
    });

    const first = dashboard.refresh("session");
    await new Promise((r) => setImmediate(r));
    dashboard.applyQuery({ top: 5 });
    const second = dashboard.refresh("session");
    await new Promise((r) => setImmediate(r));

    assert.equal(calls.length, 2);
    assert.ok(calls[0].signal.aborted, "the first run is aborted");
    calls[1].d.resolve({ data: { Sessions: 2 }, warnings: [], durationMs: 1 });

    const [a, b] = await Promise.all([first, second]);
    assert.equal(a.data.Sessions, 2, "the first caller receives the latest result");
    assert.equal(b.data.Sessions, 2);
    assert.equal(dashboard.results.session.status, "ok");
    assert.ok(dashboard.isFresh("session"));
});

test("stale completions after abort are ignored even if they resolve", async () => {
    const pending = [];
    const dashboard = makeDashboard(() => {
        const d = deferred();
        pending.push(d);
        return d.promise;
    });
    const first = dashboard.refresh("session");
    await new Promise((r) => setImmediate(r));
    const second = dashboard.refresh("session");
    await new Promise((r) => setImmediate(r));
    pending[1].resolve({ data: { Sessions: 2 }, warnings: [], durationMs: 1 });
    await second;
    pending[0].resolve({ data: { Sessions: 1 }, warnings: [], durationMs: 1 });
    await first;
    assert.equal(dashboard.results.session.data.Sessions, 2);
});

test("errors keep the previous data and report the message", async () => {
    let fail = false;
    const dashboard = makeDashboard(async () => {
        if (fail) throw new Error("boom");
        return { data: { Sessions: 3 }, warnings: ["w"], durationMs: 1 };
    });
    await dashboard.refresh("session");
    fail = true;
    const result = await dashboard.refresh("session");
    assert.equal(result.status, "error");
    assert.equal(result.error, "boom");
    assert.equal(result.data.Sessions, 3);
});

test("ensureFresh skips re-running when the result matches the query", async () => {
    let runs = 0;
    const dashboard = makeDashboard(async () => {
        runs += 1;
        return { data: {}, warnings: [], durationMs: 1 };
    });
    await dashboard.ensureFresh();
    await dashboard.ensureFresh();
    assert.equal(runs, 1);
    await dashboard.setQuery({ top: 3 });
    assert.equal(runs, 2);
    await dashboard.ensureFresh({ force: true });
    assert.equal(runs, 3);
});

test("switching source only runs the newly active source", async () => {
    const sources = [];
    const dashboard = makeDashboard(async (runner, argv) => {
        sources.push(argv[0]);
        return { data: argv[0] === "vscode" ? vscodeDataset() : {}, warnings: [], durationMs: 1 };
    });
    await dashboard.setQuery({ source: "vscode" });
    await dashboard.setQuery({ source: "session" });
    await dashboard.setQuery({ source: "vscode" });
    assert.deepEqual(sources, ["vscode", "session"]);
});

test("Agent selection, union, removal, and Top project locally without any new runner calls", async () => {
    const calls = [];
    const dashboard = makeDashboard(async (runner, argv) => {
        calls.push(argv);
        return { data: vscodeDataset(), warnings: ["missing log"], durationMs: 12 };
    });
    await dashboard.setQuery({ source: "vscode" });
    const ranAt = dashboard.results.vscode.ranAt;
    assert.ok(calls[0].includes("--include-log-stats"));
    assert.ok(calls[0].includes("--top=0"));
    await dashboard.setQuery({ filters: { vscode: { agent: ["^Explore$"] } } });
    assert.equal(dashboard.results.vscode.data.LLMRequests, 4);
    assert.equal(dashboard.results.vscode.ranAt, ranAt);
    assert.equal(dashboard.isFresh("vscode"), true);
    assert.equal(dashboard.summary().summary.llmRequests, 4);
    assert.equal(dashboard.summary().summary.byModel[0].requests, 4);
    assert.equal(dashboard.summary().localFiltering, true);
    assert.equal(dashboard.results.vscode.data.LogDetails, undefined);
    await dashboard.setQuery({ filters: { vscode: { agent: ["Explore", "executionSubagent"] } }, top: 1 });
    assert.equal(dashboard.results.vscode.data.LLMRequests, 5);
    assert.equal(dashboard.results.vscode.data.ByModel.length, 1);
    await dashboard.setQuery({ filters: { vscode: { agent: [] } }, top: 0 });
    assert.equal(dashboard.results.vscode.data.LLMRequests, 7);
    assert.equal(calls.length, 1);
    await dashboard.setQuery({ period: "7d" });
    await dashboard.setQuery({ filters: { vscode: { model: ["a"] } } });
    await dashboard.ensureFresh({ force: true });
    assert.equal(calls.length, 4);
});

test("Agent changes during initial and forced fetching do not restart it and publish the latest projection", async () => {
    const calls = [];
    const dashboard = makeDashboard(() => {
        const d = deferred();
        calls.push(d);
        return d.promise;
    });
    const first = dashboard.setQuery({ source: "vscode" });
    await new Promise((r) => setImmediate(r));
    const filtered = dashboard.setQuery({ filters: { vscode: { agent: ["Explore"] } } });
    assert.equal(calls.length, 1);
    calls[0].resolve({ data: vscodeDataset(), warnings: [], durationMs: 2 });
    await Promise.all([first, filtered]);
    assert.equal(dashboard.results.vscode.data.LLMRequests, 4);
    const refresh = dashboard.ensureFresh({ force: true });
    await new Promise((r) => setImmediate(r));
    const union = dashboard.setQuery({ filters: { vscode: { agent: ["Explore", "executionSubagent"] } } });
    assert.equal(dashboard.results.vscode.data.LLMRequests, 5);
    assert.equal(dashboard.results.vscode.status, "loading");
    assert.equal(calls.length, 2);
    calls[1].resolve({ data: vscodeDataset(), warnings: [], durationMs: 3 });
    await Promise.all([refresh, union]);
    assert.equal(dashboard.results.vscode.data.LLMRequests, 5);
    assert.equal(dashboard.results.vscode.status, "ok");
    assert.equal(dashboard.isFresh("vscode"), true);
});

test("failed refreshes stay visible while cached filters change; unsupported datasets never succeed", async () => {
    let calls = 0;
    const dashboard = makeDashboard(async () => {
        calls++;
        if (calls > 1) throw new Error("refresh failed");
        return { data: vscodeDataset(), warnings: [], durationMs: 1 };
    });
    await dashboard.setQuery({ source: "vscode" });
    await dashboard.ensureFresh({ force: true });
    await dashboard.setQuery({ filters: { vscode: { agent: ["Explore"] } } });
    assert.equal(calls, 2);
    assert.equal(dashboard.results.vscode.status, "error");
    assert.equal(dashboard.results.vscode.error, "refresh failed");
    assert.equal(dashboard.results.vscode.data.LLMRequests, 4);
    const unsupported = makeDashboard(async () => ({ data: {}, warnings: [], durationMs: 1 }));
    await unsupported.setQuery({ source: "vscode" });
    assert.equal(unsupported.results.vscode.status, "error");
    assert.match(unsupported.results.vscode.error, /update gh/);
    assert.equal(unsupported.vscodeCache, null);
});

test("runner resolution failures are reported and retried", async () => {
    let attempts = 0;
    const dashboard = new Dashboard({
        query: normalizeQuery(),
        cwd: "/repo",
        deps: {
            resolveRunner: async () => {
                attempts += 1;
                if (attempts === 1) throw new Error("not installed");
                return { cmd: "fake", prefix: [], label: "fake" };
            },
            runStats: async () => ({ data: {}, warnings: [], durationMs: 1 }),
        },
    });

    const failed = await dashboard.refresh();
    assert.equal(failed.status, "error");
    assert.equal(dashboard.snapshot().runner.error, "not installed");
    const ok = await dashboard.refresh();
    assert.equal(ok.status, "ok");
    assert.equal(dashboard.snapshot().runner.label, "fake");
});

test("VS Code base and runner changes invalidate the cache; superseded and closed runs cannot publish", async () => {
    const calls = [];
    const dashboard = makeDashboard((runner, argv, { signal }) => {
        const d = deferred();
        calls.push({ d, argv, signal });
        return d.promise;
    });
    const tick = () => new Promise((resolve) => setImmediate(resolve));
    const finish = (i) => calls[i].d.resolve({ data: vscodeDataset(), warnings: [], durationMs: 1 });
    const initial = dashboard.setQuery({ source: "vscode" });
    await tick();
    finish(0);
    await initial;

    const older = dashboard.setQuery({ scope: "all" });
    await tick();
    const latest = dashboard.setQuery({ filters: { vscode: { tool: ["read"] } } });
    await tick();
    assert.equal(calls.length, 3);
    assert.equal(calls[1].signal.aborted, true);
    assert.ok(calls[2].argv.includes("--tool=read"));
    finish(2);
    await latest;
    const latestData = dashboard.results.vscode.data;
    finish(1);
    await older;
    assert.equal(dashboard.results.vscode.data, latestData);
    assert.equal(dashboard.isFresh("vscode"), true);

    dashboard.setRunnerMode("source");
    assert.equal(dashboard.isFresh("vscode"), false);
    const changedRunner = dashboard.ensureFresh();
    await tick();
    assert.equal(calls.length, 4);
    finish(3);
    await changedRunner;
    assert.equal(dashboard.vscodeCache.runnerMode, "source");
    assert.equal(dashboard.isFresh("vscode"), true);

    const previous = dashboard.results.vscode.data;
    const closing = dashboard.ensureFresh({ force: true });
    await tick();
    dashboard.close();
    assert.equal(calls[4].signal.aborted, true);
    finish(4);
    await closing;
    assert.equal(dashboard.results.vscode.data, previous);
});

test("close aborts in-flight runs and suppresses their results", async () => {
    let signal;
    const dashboard = makeDashboard((runner, argv, options) => {
        signal = options.signal;
        return new Promise(() => {});
    });
    dashboard.refresh();
    await new Promise((r) => setImmediate(r));
    dashboard.close();
    assert.ok(signal.aborted);
});

test("persist receives every accepted query", async () => {
    const saved = [];
    const dashboard = makeDashboard(async () => ({ data: {}, warnings: [], durationMs: 1 }), { persist: (q) => saved.push(q) });
    dashboard.applyQuery({ top: 7 });
    assert.throws(() => dashboard.applyQuery({ top: -1 }));
    assert.equal(saved.length, 1);
    assert.equal(saved[0].top, 7);
    assert.equal(dashboard.query.top, 7);
});
