import { test } from "node:test";
import assert from "node:assert/strict";
import { request } from "node:http";
import { Dashboard } from "../lib/dashboard.mjs";
import { normalizeQuery } from "../lib/query.mjs";
import { startServer } from "../lib/server.mjs";
import { fusionDataset } from "./fixtures/fusion.mjs";
import { vscodeDataset } from "./fixtures/vscode.mjs";

function call(url, { method = "GET", headers = {}, body } = {}) {
    return new Promise((resolve, reject) => {
        const req = request(url, { method, headers }, (res) => {
            const chunks = [];
            res.on("data", (c) => chunks.push(c));
            res.on("end", () => {
                const text = Buffer.concat(chunks).toString("utf-8");
                let json = null;
                try {
                    json = JSON.parse(text);
                } catch {
                    // Not JSON (e.g. HTML).
                }
                resolve({ status: res.statusCode, text, json, headers: res.headers });
            });
        });
        req.on("error", reject);
        if (body !== undefined) req.write(body);
        req.end();
    });
}

async function setup(runStats = async () => ({ data: { Sessions: 1, Requests: 2 }, warnings: [], durationMs: 1 })) {
    const asked = [];
    const dashboard = new Dashboard({
        query: normalizeQuery(),
        cwd: "/repo",
        deps: {
            resolveRunner: async () => ({ cmd: "fake", prefix: [], label: "fake" }),
            runStats,
        },
    });
    const server = await startServer({ dashboard, ask: async (a) => asked.push({ ...a, summary: dashboard.summary(a.source) }) });
    const auth = { "X-Dashboard-Token": server.token };
    const json = { ...auth, "Content-Type": "application/json" };
    return { dashboard, server, asked, auth, json };
}

test("index.html embeds the token and sets a CSP", async (t) => {
    const { server } = await setup();
    t.after(() => server.close());
    const res = await call(server.url);
    assert.equal(res.status, 200);
    assert.ok(res.text.includes(server.token));
    assert.ok(!res.text.includes("__DASHBOARD_TOKEN__"));
    assert.match(res.headers["content-security-policy"], /script-src 'self'/);
    assert.match(res.text, /<option value="this-month">This month<\/option>/);
    assert.match(res.text, /<option value="last-month">Last month<\/option>/);
});

test("index.html explains harness-based tab routing without implying a VS Code version cutoff", async (t) => {
    const { server } = await setup();
    t.after(() => server.close());
    const res = await call(server.url);
    const warning = res.text.match(/<div class="banner warn" id="vscode-harness-warning" role="note" hidden>([\s\S]*?)<\/div>/)?.[1];
    assert.ok(warning);
    assert.match(warning, /VS Code Local harness only/);
    assert.match(warning, /Copilot Chat debug logs from the Local harness/);
    assert.match(warning, /Locally recorded VS Code Copilot harness \(Agent Host\) sessions are included in the Copilot CLI tab/);
    assert.match(warning, /depends on the harness, not the VS Code version/);
    assert.doesNotMatch(warning, /legacy|older|newer/i);
});

test("table view modules are served as same-origin JavaScript", async (t) => {
    const { server } = await setup();
    t.after(() => server.close());
    for (const [file, exported] of [["model-view.mjs", "sortModels"], ["workspace-view.mjs", "sortWorkspaces"], ["table-view.mjs", "sortTableEntries"], ["filter-view.mjs", "panelClearPatch"]]) {
        const res = await call(`${server.origin}/${file}`);
        assert.equal(res.status, 200);
        assert.match(res.headers["content-type"], /^text\/javascript/);
        assert.ok(res.text.includes(`export function ${exported}`));
        assert.equal((await call(`${server.origin}/${file}`, { headers: { Origin: "http://evil.example" } })).status, 403);
    }
});

test("the token-bearing page is only served at the secret URL", async (t) => {
    const { server } = await setup();
    t.after(() => server.close());
    assert.notEqual(server.url, `${server.origin}/`);
    for (const p of ["/", "/index.html"]) {
        const res = await call(`${server.origin}${p}`);
        assert.equal(res.status, 404);
        assert.ok(!res.text.includes(server.token));
    }
    assert.equal((await call(`${server.origin}/app.js`)).status, 200);
});

test("API requests require the token", async (t) => {
    const { server, auth } = await setup();
    t.after(() => server.close());
    assert.equal((await call(`${server.origin}/api/state`)).status, 401);
    assert.equal((await call(`${server.origin}/api/state`, { headers: { "X-Dashboard-Token": "nope" } })).status, 401);
    assert.equal((await call(`${server.origin}/events`)).status, 401);
    const ok = await call(`${server.origin}/api/state`, { headers: auth });
    assert.equal(ok.status, 200);
    assert.equal(ok.json.query.source, "session");
    assert.ok(ok.json.meta.presets.length > 0);
});

test("foreign Host and Origin headers are rejected", async (t) => {
    const { server, auth } = await setup();
    t.after(() => server.close());
    assert.equal((await call(`${server.origin}/api/state`, { headers: { ...auth, Host: "evil.example" } })).status, 421);
    assert.equal((await call(`${server.origin}/api/state`, { headers: { ...auth, Origin: "http://evil.example" } })).status, 403);
    assert.equal((await call(server.url, { headers: { Host: "evil.example" } })).status, 421);
});

test("POST requires JSON and validates the query", async (t) => {
    const { server, auth, json, dashboard } = await setup();
    t.after(() => server.close());
    const url = `${server.origin}/api/query`;
    assert.equal((await call(url, { method: "POST", headers: { ...auth, "Content-Type": "text/plain" }, body: "{}" })).status, 415);
    assert.equal((await call(url, { method: "POST", headers: json, body: "{not json" })).status, 400);
    const invalid = await call(url, { method: "POST", headers: json, body: JSON.stringify({ query: { top: -1 } }) });
    assert.equal(invalid.status, 400);
    assert.match(invalid.json.error, /top/);
    assert.equal((await call(url, { method: "POST", headers: json, body: "x".repeat(70 * 1024) })).status, 413);

    const ok = await call(url, { method: "POST", headers: json, body: JSON.stringify({ query: { top: 4 }, refresh: false }) });
    assert.equal(ok.status, 200);
    assert.equal(dashboard.query.top, 4);
});

test("ask requires a fresh, successful result and forwards presets", async (t) => {
    const { server, json, dashboard, asked } = await setup();
    t.after(() => server.close());
    const url = `${server.origin}/api/ask`;
    assert.equal((await call(url, { method: "POST", headers: json, body: JSON.stringify({ text: "hi" }) })).status, 409);
    await dashboard.refresh("session");
    assert.equal((await call(url, { method: "POST", headers: json, body: JSON.stringify({}) })).status, 400);
    assert.equal((await call(url, { method: "POST", headers: json, body: JSON.stringify({ preset: "nope" }) })).status, 400);
    const ok = await call(url, { method: "POST", headers: json, body: JSON.stringify({ preset: "summarize", text: "focus on shell" }) });
    assert.equal(ok.status, 200);
    assert.equal(asked.length, 1);
    assert.match(asked[0].text, /focus on shell/);

    // Changing the query keeps the old data, which must not be sent with the new query.
    dashboard.applyQuery({ top: 5 });
    const stale = await call(url, { method: "POST", headers: json, body: JSON.stringify({ text: "hi" }) });
    assert.equal(stale.status, 409);
    assert.match(stale.json.error, /out of date/);

    // A failed refresh keeps the previous data but is not a successful result.
    await dashboard.refresh("session");
    dashboard.runStats = async () => {
        throw new Error("boom");
    };
    await dashboard.refresh("session");
    assert.equal(dashboard.results.session.status, "error");
    assert.equal((await call(url, { method: "POST", headers: json, body: JSON.stringify({ text: "hi" }) })).status, 409);
    assert.equal(asked.length, 1);
});

test("HTTP state and Ask agent share recorded Agent-mode outcomes", async (t) => {
    const modes = [
        { Key: "plan", Total: 1, Approved: 0, ApprovedForLocation: 0, Denied: 1, Unresolved: 0 },
        { Key: "unknown", Total: 1, Approved: 0, ApprovedForLocation: 0, Denied: 0, Unresolved: 1 },
    ];
    const calls = [];
    const { server, json, auth, dashboard, asked } = await setup(async (runner, argv) => {
        calls.push(argv);
        const selected = argv.includes("--agent-mode=plan") ? modes.slice(0, 1) : modes;
        return { data: { Sessions: 1, Requests: selected.length, ByAgentMode: selected }, warnings: [], durationMs: 1 };
    });
    t.after(() => server.close());
    await dashboard.refresh("session");
    const state = await call(`${server.origin}/api/state`, { headers: auth });
    assert.equal(state.status, 200);
    assert.deepEqual(state.json.results.session.data.ByAgentMode, modes);
    const response = await call(`${server.origin}/api/ask`, {
        method: "POST", headers: json, body: JSON.stringify({ text: "Compare Agent modes" }),
    });
    assert.equal(response.status, 200);
    assert.deepEqual(asked[0].summary.summary.byAgentMode, [
        { key: "plan", total: 1, approved: 0, approvedForLocation: 0, denied: 1, unresolved: 0 },
        { key: "unknown", total: 1, approved: 0, approvedForLocation: 0, denied: 0, unresolved: 1 },
    ]);
    const filtered = await call(`${server.origin}/api/query`, {
        method: "POST", headers: json,
        body: JSON.stringify({ query: { filters: { session: { agentMode: ["plan"] } } } }),
    });
    assert.equal(filtered.status, 200);
    await dashboard.refreshing("session");
    const next = await call(`${server.origin}/api/state`, { headers: auth });
    assert.deepEqual(next.json.query.filters.session.agentMode, ["plan"]);
    assert.deepEqual(next.json.results.session.data.ByAgentMode, modes.slice(0, 1));
    assert.equal(next.json.results.session.fresh, true);
    assert.ok(calls.at(-1).includes("--agent-mode=plan"));
    assert.equal((await call(`${server.origin}/api/ask`, {
        method: "POST", headers: json, body: JSON.stringify({ text: "Summarize the selected mode" }),
    })).status, 200);
    assert.deepEqual(asked[1].summary.summary.byAgentMode, [
        { key: "plan", total: 1, approved: 0, approvedForLocation: 0, denied: 1, unresolved: 0 },
    ]);
});

test("HTTP Ask agent rejects stale data when the selected CLI cannot filter Agent modes", async (t) => {
    const { server, json, dashboard, asked } = await setup(async (runner, argv) => {
        if (argv.includes("--agent-mode=plan")) throw new Error("update gh copilot-attendant or choose the source runner");
        return { data: { Requests: 3 }, warnings: [], durationMs: 1 };
    });
    t.after(() => server.close());
    await dashboard.ensureFresh();
    const response = await call(`${server.origin}/api/query`, {
        method: "POST", headers: json,
        body: JSON.stringify({ query: { filters: { session: { agentMode: ["plan"] } } } }),
    });
    assert.equal(response.status, 200);
    await dashboard.refreshing("session");
    assert.equal(dashboard.results.session.status, "error");
    assert.equal(dashboard.isFresh("session"), false);
    const ask = await call(`${server.origin}/api/ask`, {
        method: "POST", headers: json, body: JSON.stringify({ text: "Summarize permissions" }),
    });
    assert.equal(ask.status, 409);
    assert.equal(asked.length, 0);
});

test("HTTP Agent changes use cached data for state and Ask agent without launching gh", async (t) => {
    let calls = 0;
    const { server, json, dashboard, asked, auth } = await setup(async () => {
        calls++;
        return { data: vscodeDataset(), warnings: [], durationMs: 10 };
    });

    t.after(() => server.close());
    await dashboard.setQuery({ source: "vscode" });
    const ranAt = dashboard.results.vscode.ranAt;
    const response = await call(`${server.origin}/api/query`, {
        method: "POST", headers: json,
        body: JSON.stringify({ query: { top: 1, filters: { vscode: { agent: ["^Explore$"] } } } }),
    });
    assert.equal(response.status, 200);
    const projected = response.json.results.vscode;
    assert.equal(projected.data.LLMRequests, 4);
    assert.equal(projected.data.ByModel.length, 1);
    assert.equal(projected.data.LogDetails, undefined);
    assert.equal(projected.ranAt, ranAt);
    assert.equal(projected.fresh, true);
    assert.equal(projected.localFiltering, true);
    const state = await call(`${server.origin}/api/state`, { headers: auth });
    assert.deepEqual(state.json.results.vscode.data, projected.data);
    const ask = await call(`${server.origin}/api/ask`, {
        method: "POST", headers: json, body: JSON.stringify({ preset: "summarize" }),
    });
    assert.equal(ask.status, 200);
    assert.equal(asked[0].summary.summary.llmRequests, 4);
    assert.equal(asked[0].summary.ranAt, ranAt);
    assert.equal(calls, 1);
});

test("HTTP Fusion-only selection projects state and Ask agent without exposing permissions", async (t) => {
    let calls = 0;
    const { server, json, dashboard, asked, auth } = await setup(async () => {
        calls++;
        return { data: fusionDataset(), warnings: [], durationMs: 1 };
    });
    t.after(() => server.close());
    await dashboard.ensureFresh();
    const response = await call(`${server.origin}/api/query`, {
        method: "POST", headers: json, body: JSON.stringify({ query: { fusionOnly: true } }),
    });
    assert.equal(response.status, 200);
    assert.equal(response.json.results.session.data.FusionOnly, true);
    assert.equal("Requests" in response.json.results.session.data, false);
    assert.equal(response.json.results.session.fresh, true);
    const state = await call(`${server.origin}/api/state`, { headers: auth });
    assert.deepEqual(state.json.results.session.data, response.json.results.session.data);
    const disallowed = await call(`${server.origin}/api/ask`, {
        method: "POST", headers: json, body: JSON.stringify({ preset: "permissions" }),
    });
    assert.equal(disallowed.status, 400);
    assert.match(disallowed.json.error, /unavailable in Fusion-only mode/);
    const ask = await call(`${server.origin}/api/ask`, {
        method: "POST", headers: json, body: JSON.stringify({ preset: "summarize" }),
    });
    assert.equal(ask.status, 200);
    assert.equal(asked[0].summary.summary.usage.usageRecords, 3);
    assert.equal("requests" in asked[0].summary.summary, false);
    assert.equal("resolvedTurns" in asked[0].summary.summary.fusion, false);
    assert.equal(calls, 1);
});
