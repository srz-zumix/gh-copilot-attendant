import { test } from "node:test";
import assert from "node:assert/strict";
import { Dashboard } from "../lib/dashboard.mjs";
import { normalizeQuery } from "../lib/query.mjs";

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
        return { data: {}, warnings: [], durationMs: 1 };
    });
    await dashboard.setQuery({ source: "vscode" });
    await dashboard.setQuery({ source: "session" });
    await dashboard.setQuery({ source: "vscode" });
    assert.deepEqual(sources, ["vscode", "session"]);
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
