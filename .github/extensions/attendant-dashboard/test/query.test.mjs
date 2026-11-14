import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";
import { buildArgv, checkGoRegexp, COMMAND_ACCESS, SANDBOX_STATES, defaultQuery, exactPattern, MAX_TOP, normalizeQuery, OPERATIONS, QueryError, SCOPES, SOURCES } from "../lib/query.mjs";
import { RUNNERS } from "../lib/runner.mjs";

test("default query builds the worktree-scoped session command", () => {
    assert.deepEqual(buildArgv(normalizeQuery()), ["session", "stats", "--period=30d", "--top=20", "--format", "json"]);
    assert.equal(normalizeQuery().fusionOnly, false);
});

test("Top defaults to 20 without replacing explicit or saved values", () => {
    for (const top of ["", undefined, null]) assert.equal(normalizeQuery({ top }).top, 20);
    for (const top of [0, 10, 25]) {
        assert.equal(normalizeQuery({ top }).top, top);
        assert.equal(normalizeQuery({}, normalizeQuery({ top })).top, top);
    }
});

test("Fusion-only is a persistent session projection, not a CLI flag or VS Code filter", () => {
    const base = normalizeQuery({ fusionOnly: true, filters: { session: { kind: ["shell"] } } });
    const vscode = normalizeQuery({ source: "vscode" }, base);
    const back = normalizeQuery({ source: "session" }, vscode);
    assert.equal(vscode.fusionOnly, true);
    assert.equal(back.fusionOnly, true);
    assert.deepEqual(back.filters.session.kind, ["shell"]);
    assert.deepEqual(buildArgv(base, "session"), buildArgv(normalizeQuery({ fusionOnly: false }, base), "session"));
    assert.deepEqual(buildArgv(base, "vscode"), buildArgv(normalizeQuery({ fusionOnly: false }, base), "vscode"));
    assert.throws(() => normalizeQuery({ fusionOnly: "true" }), /fusionOnly must be a boolean/);
    assert.throws(() => normalizeQuery({ fusionOnly: null }), /fusionOnly must be a boolean/);
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
    const q = normalizeQuery({ filters: { session: { kind: ["shell"] }, vscode: { tool: ["read_file"], agent: ["^Explore$", "^executionSubagent$"] } } });
    assert.ok(buildArgv(q, "session").includes("--kind=shell"));
    assert.ok(!buildArgv(q, "session").some((a) => a.startsWith("--tool")));
    assert.ok(!buildArgv(q, "session").some((a) => a.startsWith("--agent")));
    assert.ok(buildArgv(q, "vscode").includes("--tool=read_file"));
    assert.ok(buildArgv(q, "vscode").includes("--agent=^Explore$"));
    assert.ok(buildArgv(q, "vscode").includes("--agent=^executionSubagent$"));
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

test("result and command access filters map to exact repeatable session flags", () => {
    const values = [" denied-no-rule ", "denied"];
    const q = normalizeQuery({ filters: { session: { result: values, commandAccess: ["read-only", "read-write"] } } });
    assert.deepEqual(q.filters.session.result, values);
    const argv = buildArgv(q, "session");
    for (const value of values) assert.ok(argv.includes(`--result=${value}`));
    assert.ok(argv.includes("--command-access=read-only"));
    assert.ok(argv.includes("--command-access=read-write"));
    assert.ok(!buildArgv(q, "vscode").some((arg) => arg.startsWith("--result=")));
    assert.throws(() => normalizeQuery({ filters: { session: { commandAccess: ["read"] } } }), /commandAccess must be one of/);
});

test("sandbox filters map to repeatable --sandbox flags for both sources", () => {
    const q = normalizeQuery({ filters: { session: { sandbox: ["applied", "unknown"] }, vscode: { sandbox: ["not-applied"] } } });
    const session = buildArgv(q, "session");
    assert.ok(session.includes("--sandbox=applied"));
    assert.ok(session.includes("--sandbox=unknown"));
    assert.ok(!session.includes("--sandbox=not-applied"));
    assert.deepEqual(buildArgv(q, "vscode").filter((arg) => arg.startsWith("--sandbox")), ["--sandbox=not-applied"]);
    for (const source of SOURCES) {
        assert.throws(() => normalizeQuery({ filters: { [source]: { sandbox: ["on"] } } }), /sandbox must be one of/);
    }
});

test("Agent modes preserve exact values and map to repeatable session-only flags", () => {
    const values = [" plan ", "plan", "future,mode", " ", " plan "];
    const q = normalizeQuery({ filters: { session: { agentMode: values, kind: ["shell"] } } });
    assert.deepEqual(q.filters.session.agentMode, values.slice(0, -1));
    const argv = buildArgv(q, "session");
    for (const value of q.filters.session.agentMode) assert.ok(argv.includes(`--agent-mode=${value}`));
    assert.ok(argv.includes("--kind=shell"));
    assert.ok(!buildArgv(q, "vscode").some((arg) => arg.startsWith("--agent-mode")));
    assert.deepEqual(normalizeQuery({}, { filters: { session: { kind: ["shell"] } } }).filters.session.agentMode, []);

    const fusion = normalizeQuery({ fusionOnly: true }, q);
    const switched = normalizeQuery({ source: "vscode" }, fusion);
    const back = normalizeQuery({ source: "session", fusionOnly: false }, switched);
    assert.deepEqual(back.filters.session.agentMode, q.filters.session.agentMode);
    assert.deepEqual(buildArgv(fusion, "session"), argv);
    assert.throws(() => normalizeQuery({ filters: { session: { agentMode: ["plan\nother"] } } }), /control characters/);
});

test("permission modes combine with human decisions and preserve exact values", () => {
    const values = ["assisted", "unknown", " future,mode ", "assisted"];
    const q = normalizeQuery({ filters: { session: {
        permissionMode: values, decisionSource: ["human_response"], result: ["approved"], kind: ["shell"],
    } } });
    assert.deepEqual(q.filters.session.permissionMode, values.slice(0, -1));
    const argv = buildArgv(q, "session");
    for (const value of q.filters.session.permissionMode) assert.ok(argv.includes(`--permission-mode=${value}`));
    for (const arg of ["--decision-source=human_response", "--result=approved", "--kind=shell"]) assert.ok(argv.includes(arg));
    assert.ok(!buildArgv(q, "vscode").some((arg) => arg.startsWith("--permission-mode")));
    assert.deepEqual(normalizeQuery({}, q).filters.session.permissionMode, q.filters.session.permissionMode);
    assert.deepEqual(normalizeQuery({}, { filters: { session: { kind: ["shell"] } } }).filters.session.permissionMode, []);
    assert.throws(() => normalizeQuery({ filters: { session: { permissionMode: ["assisted\nmanual"] } } }), /control characters/);
});

test("canvas open and refresh schemas accept the CLI permission query filters", () => {
    const source = readFileSync(new URL("../extension.mjs", import.meta.url), "utf8")
        .replace(/^import .*;\n/gm, "")
        .replace("session = await joinSession({ canvases: [canvas] });", "");
    const canvas = vm.runInNewContext(`${source}\ncanvas`, {
        SOURCES, SCOPES, OPERATIONS, COMMAND_ACCESS, SANDBOX_STATES, MAX_TOP, RUNNERS,
        QueryStore: class {}, createCanvas: (value) => value,
    });
    const openQuery = canvas.inputSchema.properties.query;
    const refreshQuery = canvas.actions.find((action) => action.name === "refresh").inputSchema.properties.query;
    const session = openQuery.properties.filters.properties.session;
    assert.equal(session.additionalProperties, false);
    assert.equal(session.properties.agentMode.type, "array");
    assert.equal(session.properties.agentMode.items.type, "string");
    assert.equal(session.properties.agentMode.maxItems, 20);
    assert.equal(session.properties.permissionMode.type, "array");
    assert.equal(session.properties.permissionMode.items.type, "string");
    assert.equal(session.properties.permissionMode.maxItems, 20);
    assert.equal(session.properties.result.items.type, "string");
    assert.deepEqual(session.properties.commandAccess.items.enum, COMMAND_ACCESS);
    assert.deepEqual(session.properties.sandbox.items.enum, SANDBOX_STATES);
    assert.deepEqual(openQuery.properties.filters.properties.vscode.properties.sandbox.items.enum, SANDBOX_STATES);
    assert.equal(refreshQuery, openQuery);
});

test("regex filters are checked against Go's RE2 syntax", () => {
    for (const bad of ["(?=a)", "(?!a)", "(?<=a)b", "(?<!a)b", "(?>a)", "(a)\\1", "(?P<n>a)(?P=n)", "\\k<n>", "a\\Z", "\\9"]) {
        assert.equal(typeof checkGoRegexp(bad), "string", bad);
        assert.throws(() => normalizeQuery({ filters: { session: { path: [bad] } } }), /regular expression/, bad);
    }
    for (const good of ["(?i)foo", "(?i:foo)bar", "(?P<name>a)", "(?<name>a)", "\\Q(a+)\\E", "\\Qa.b", "[[:alpha:]]+", "\\A/tmp\\z", "\\pL", "\\x{41}", "\\101", "[]a]", "(?s-i)x"]) {
        assert.equal(checkGoRegexp(good), null, good);
    }
    assert.deepEqual(normalizeQuery({ filters: { vscode: { tool: ["(?i)read"] } } }).filters.vscode.tool, ["(?i)read"]);
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

test("a rolling period clears inherited until but preserves explicit upper bounds", () => {
    const base = normalizeQuery({ period: "", since: "2026-01-01", until: "2026-02-01" });
    const next = normalizeQuery({ period: "7d" }, base);
    assert.equal(next.since, "");
    assert.equal(next.until, "");
    assert.equal(normalizeQuery({ period: "7d", until: "2026-03-01" }, base).until, "2026-03-01");
    assert.equal(normalizeQuery({ top: 5 }, base).until, base.until);
    assert.equal(normalizeQuery({ period: "" }, base).until, base.until);
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
