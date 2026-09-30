import { test } from "node:test";
import assert from "node:assert/strict";
import { buildArgv, defaultQuery, exactPattern, normalizeQuery, QueryError } from "../lib/query.mjs";

test("default query builds the worktree-scoped session command", () => {
    assert.deepEqual(buildArgv(normalizeQuery()), ["session", "stats", "--period=30d", "--top=10", "--format", "json"]);
});

test("scopes map to the matching CLI flags", () => {
    assert.ok(buildArgv(normalizeQuery({ scope: "all", path: "/ignored" })).includes("--all"));
    assert.ok(buildArgv(normalizeQuery({ scope: "cwd", path: "/repo" })).includes("--cwd=/repo"));
    assert.ok(buildArgv(normalizeQuery({ scope: "worktree", path: "/wt" })).includes("--worktree=/wt"));
    assert.equal(normalizeQuery({ scope: "all", path: "/ignored" }).path, "");
    assert.throws(() => normalizeQuery({ scope: "cwd" }), QueryError);
});

test("values are passed as single --flag=value arguments", () => {
    const q = normalizeQuery({ filters: { session: { command: ["--all"], path: ["-x"] } } });
    const argv = buildArgv(q, "session");
    assert.ok(argv.includes("--command=--all"));
    assert.ok(argv.includes("--path=-x"));
    assert.ok(!argv.includes("--all"));
});

test("filters are emitted only for their own source", () => {
    const q = normalizeQuery({ filters: { session: { kind: ["shell"] }, vscode: { tool: ["read_file"] } } });
    assert.ok(buildArgv(q, "session").includes("--kind=shell"));
    assert.ok(!buildArgv(q, "session").some((a) => a.startsWith("--tool")));
    assert.ok(buildArgv(q, "vscode").includes("--tool=read_file"));
    assert.ok(!buildArgv(q, "vscode").some((a) => a.startsWith("--kind")));
});

test("decisionSource maps to --decision-source", () => {
    const q = normalizeQuery({ filters: { session: { decisionSource: ["unattended_fallback", "unknown"] } } });
    const argv = buildArgv(q, "session");
    assert.ok(argv.includes("--decision-source=unattended_fallback"));
    assert.ok(argv.includes("--decision-source=unknown"));
    assert.ok(!argv.some((a) => a.startsWith("--decisionSource")));
    assert.deepEqual(normalizeQuery({}, { filters: { session: { kind: ["shell"] } } }).filters.session.decisionSource, []);
});

test("patches keep unspecified filters and merge onto the base", () => {
    const base = normalizeQuery({ filters: { session: { kind: ["shell"], command: ["ls"] } } });
    const next = normalizeQuery({ filters: { session: { kind: [] } } }, base);
    assert.deepEqual(next.filters.session.kind, []);
    assert.deepEqual(next.filters.session.command, ["ls"]);
});

test("period and since clear each other when patched individually", () => {
    const withSince = normalizeQuery({ since: "2026-01-01" }, defaultQuery());
    assert.equal(withSince.period, "");
    const withPeriod = normalizeQuery({ period: "7d" }, withSince);
    assert.equal(withPeriod.since, "");
    assert.throws(() => normalizeQuery({ period: "7d", since: "2026-01-01" }), /mutually exclusive/);
});

test("rejects invalid values", () => {
    assert.throws(() => normalizeQuery({ source: "other" }), QueryError);
    assert.throws(() => normalizeQuery({ period: "week" }), QueryError);
    assert.throws(() => normalizeQuery({ since: "yesterday" }), QueryError);
    assert.throws(() => normalizeQuery({ until: "2026-13" }), QueryError);
    assert.throws(() => normalizeQuery({ top: -1 }), QueryError);
    assert.throws(() => normalizeQuery({ top: 1.5 }), QueryError);
    assert.throws(() => normalizeQuery({ session: "../x" }), QueryError);
    assert.throws(() => normalizeQuery({ path: "a\nb", scope: "cwd" }), QueryError);
    assert.throws(() => normalizeQuery({ bogus: 1 }), /unknown query field/);
    assert.throws(() => normalizeQuery({ filters: { session: { tool: ["x"] } } }), /unknown session filter/);
    assert.throws(() => normalizeQuery({ filters: { session: { operation: ["delete"] } } }), QueryError);
    assert.throws(() => normalizeQuery({ filters: { session: { path: ["("] } } }), /regular expression/);
    assert.throws(() => normalizeQuery({ filters: { vscode: { model: ["["] } } }), /regular expression/);
});

test("accepts the CLI's time formats", () => {
    for (const since of ["7d", "24h", "30m", "2w", "2026-09-01", "2026-09-01T10:00:00Z", "2026-09-01T10:00:00+09:00"]) {
        assert.equal(normalizeQuery({ since }).since, since);
    }
});

test("filter values are trimmed and de-duplicated", () => {
    const q = normalizeQuery({ filters: { session: { kind: [" shell ", "shell", ""] } } });
    assert.deepEqual(q.filters.session.kind, ["shell"]);
});

test("exactPattern anchors and escapes a literal", () => {
    const pattern = exactPattern("/tmp/a+b (1).txt");
    assert.equal(pattern, "^/tmp/a\\+b \\(1\\)\\.txt$");
    assert.ok(new RegExp(pattern).test("/tmp/a+b (1).txt"));
    assert.ok(!new RegExp(pattern).test("x/tmp/a+b (1).txt"));
});
