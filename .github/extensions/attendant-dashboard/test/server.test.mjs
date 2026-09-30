import { test } from "node:test";
import assert from "node:assert/strict";
import { request } from "node:http";
import { Dashboard } from "../lib/dashboard.mjs";
import { normalizeQuery } from "../lib/query.mjs";
import { startServer } from "../lib/server.mjs";

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

async function setup() {
    const asked = [];
    const dashboard = new Dashboard({
        query: normalizeQuery(),
        cwd: "/repo",
        deps: {
            resolveRunner: async () => ({ cmd: "fake", prefix: [], label: "fake" }),
            runStats: async () => ({ data: { Sessions: 1, Requests: 2 }, warnings: [], durationMs: 1 }),
        },
    });
    const server = await startServer({ dashboard, ask: async (a) => asked.push(a) });
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
});

test("API requests require the token", async (t) => {
    const { server, auth } = await setup();
    t.after(() => server.close());
    assert.equal((await call(`${server.url}api/state`)).status, 401);
    assert.equal((await call(`${server.url}api/state`, { headers: { "X-Dashboard-Token": "nope" } })).status, 401);
    assert.equal((await call(`${server.url}events`)).status, 401);
    const ok = await call(`${server.url}api/state`, { headers: auth });
    assert.equal(ok.status, 200);
    assert.equal(ok.json.query.source, "session");
    assert.ok(ok.json.meta.presets.length > 0);
});

test("foreign Host and Origin headers are rejected", async (t) => {
    const { server, auth } = await setup();
    t.after(() => server.close());
    assert.equal((await call(`${server.url}api/state`, { headers: { ...auth, Host: "evil.example" } })).status, 421);
    assert.equal((await call(`${server.url}api/state`, { headers: { ...auth, Origin: "http://evil.example" } })).status, 403);
    assert.equal((await call(server.url, { headers: { Host: "evil.example" } })).status, 421);
});

test("POST requires JSON and validates the query", async (t) => {
    const { server, auth, json, dashboard } = await setup();
    t.after(() => server.close());
    const url = `${server.url}api/query`;
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

test("ask requires loaded data and forwards presets", async (t) => {
    const { server, json, dashboard, asked } = await setup();
    t.after(() => server.close());
    const url = `${server.url}api/ask`;
    assert.equal((await call(url, { method: "POST", headers: json, body: JSON.stringify({ text: "hi" }) })).status, 409);
    await dashboard.refresh("session");
    assert.equal((await call(url, { method: "POST", headers: json, body: JSON.stringify({}) })).status, 400);
    assert.equal((await call(url, { method: "POST", headers: json, body: JSON.stringify({ preset: "nope" }) })).status, 400);
    const ok = await call(url, { method: "POST", headers: json, body: JSON.stringify({ preset: "summarize", text: "focus on shell" }) });
    assert.equal(ok.status, 200);
    assert.equal(asked.length, 1);
    assert.match(asked[0].text, /focus on shell/);
});
