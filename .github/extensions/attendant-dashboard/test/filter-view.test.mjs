import { test } from "node:test";
import assert from "node:assert/strict";
import { activityFilterError, panelClearPatch } from "../ui/filter-view.mjs";
import { normalizeQuery, FILTERS } from "../lib/query.mjs";
import { Dashboard } from "../lib/dashboard.mjs";
import { vscodeDataset } from "./fixtures/vscode.mjs";
import { readFileSync } from "node:fs";
import vm from "node:vm";
import * as modelView from "../ui/model-view.mjs";
import * as workspaceView from "../ui/workspace-view.mjs";
import * as tableView from "../ui/table-view.mjs";
import { projectFusion } from "../lib/fusion-data.mjs";
import { emptyFusionDataset, fusionDataset } from "./fixtures/fusion.mjs";

class Element {
    constructor(tag) {
        this.tag = tag;
        this.children = [];
        this.attributes = {};
        this.events = {};
        this.style = { setProperty() {} };
        this.classList = { add() {} };
        this.scrollLeft = 0;
    }
    setAttribute(name, value) { this.attributes[name] = value; }
    getAttribute(name) { return this.attributes[name] ?? null; }
    get open() { return Object.hasOwn(this.attributes, "open"); }
    set open(value) {
        if (value) this.attributes.open = "";
        else delete this.attributes.open;
    }
    get disabled() { return Object.hasOwn(this.attributes, "disabled"); }
    hasAttribute(name) { return Object.hasOwn(this.attributes, name); }
    addEventListener(name, handler) { this.events[name] = handler; }
    append(child) {
        this.children.push(child);
        if (child instanceof Element) child.parentElement = this;
    }
    closest(selector) {
        for (let node = this; node; node = node.parentElement) {
            if (selector === ".panel" && node.className?.split(" ").includes("panel")) return node;
        }
        return null;
    }
}

function renderedPanels(query, data, { storage, filters = FILTERS } = {}) {
    const source = readFileSync(new URL("../ui/app.js", import.meta.url), "utf8")
        .replace(/^import .*;\n/gm, "").replace(/\nmain\(\);\s*$/, "\n");
    const submitted = [];
    const warnings = [];
    const context = vm.createContext({
        ...modelView, ...workspaceView, ...tableView, activityFilterError, panelClearPatch, Intl,
        Node: Element, document: {
            querySelector: (selector) => selector === 'meta[name="dashboard-token"]' ? { content: "test" } : null,
            createElement: (tag) => new Element(tag),
        },
        localStorage: storage ?? { getItem: () => null, setItem() {} },
        console: { warn: (...args) => warnings.push(args) },
        query, data, submitted, providerFilters: filters,
    });
    vm.runInContext(source, context);
    vm.runInContext("meta = { filters: providerFilters }; state = { query, results: { vscode: { argv: [] } } }; patchQuery = (patch) => submitted.push(patch);", context);
    const nodes = vm.runInContext(query.source === "vscode" ? "renderVscode(data)" : "renderSession(data)", context);
    const walk = (node) => node instanceof Element ? [node, ...node.children.flatMap(walk)] : [];
    const panels = nodes.flatMap(walk).filter((node) => node.tag === "section");
    return { nodes, panels, submitted, walk, context, warnings };
}

function interactiveFusion(data, { storage, fusionOnly = false, top = 10 } = {}) {
    const query = normalizeQuery({ source: "session", fusionOnly, top });
    const view = renderedPanels(query, data, { storage });
    const { context, walk } = view;
    const content = new Element("main");
    content.classList = { toggle() {} };
    content.replaceChildren = (...children) => {
        if (walk(content).includes(context.document.activeElement)) context.document.activeElement = null;
        content.children = children;
    };
    const elements = {
        "vscode-harness-warning": {}, content, error: { dataset: {} }, refresh: {}, command: {}, "command-label": {},
    };
    context.document.getElementById = (id) => elements[id];
    context.document.querySelectorAll = (selector) => selector === ".panel-disclosure, .category-disclosure"
        ? walk(content).filter((node) => node.tag === "details" &&
            ["panel-disclosure", "category-disclosure"].some((name) => node.className?.split(" ").includes(name)))
        : [];
    context.document.activeElement = null;
    context.document.querySelector = (selector) => {
        const match = /^\[([a-z-]+)(?:="([^"]+)")?\]$/.exec(selector);
        const found = match && walk(content).find((node) => Object.hasOwn(node.attributes, match[1]) &&
            (match[2] === undefined || node.attributes[match[1]] === match[2]));
        if (found) found.focus = () => { context.document.activeElement = found; };
        return found || null;
    };
    vm.runInContext(`
        meta = {};
        fillForm = renderFilters = renderStatus = renderWarnings = renderPresets = showError = () => {};
        state.results = { session: { status: "ok", data, command: [] } };
        render();
    `, context);
    const table = () => walk(content).find((node) => node.tag === "table" &&
        walk(node).some((child) => child.attributes["data-fusion-phase-sort"] === "pattern"));
    const button = (id) => walk(table()).find((node) => node.attributes["data-fusion-phase-sort"] === id);
    const header = (id) => walk(table()).find((node) => node.tag === "th" &&
        walk(node).some((child) => child.attributes["data-fusion-phase-sort"] === id));
    const rows = () => walk(table()).find((node) => node.tag === "tbody").children.map((row) => row.children[0].children[0]);
    const agentTable = () => walk(content).find((node) => node.tag === "table" &&
        walk(node).some((child) => child.attributes["data-agent-sort"] === "role"));
    const agentButton = (id) => walk(agentTable()).find((node) => node.attributes["data-agent-sort"] === id);
    const agentHeader = (id) => walk(agentTable()).find((node) => node.tag === "th" &&
        walk(node).some((child) => child.attributes["data-agent-sort"] === id));
    const text = (node) => typeof node === "string" ? node : node instanceof Element ? node.children.map(text).join("") : "";
    const agentRows = () => walk(agentTable()).find((node) => node.tag === "tbody").children.map((row) => text(row.children[0]));
    const modelTable = () => walk(content).find((node) => node.tag === "table" &&
        walk(node).some((child) => child.attributes["data-fusion-model-sort"] === "model"));
    const modelButton = (id) => walk(modelTable()).find((node) => node.attributes["data-fusion-model-sort"] === id);
    const modelHeader = (id) => walk(modelTable()).find((node) => node.tag === "th" &&
        walk(node).some((child) => child.attributes["data-fusion-model-sort"] === id));
    const modelRows = () => walk(modelTable()).find((node) => node.tag === "tbody").children;
    const modelNames = () => modelRows().map((row) => text(row.children[0]));
    const modelUnit = (label) => walk(modelTable().closest(".panel")).find((node) => node.tag === "button" && text(node) === label);
    const workspaceTable = () => walk(content).find((node) => node.tag === "table" &&
        walk(node).some((child) => child.attributes["data-fusion-workspace-sort"] === "workspace"));
    const workspaceButton = (id) => walk(workspaceTable()).find((node) => node.attributes["data-fusion-workspace-sort"] === id);
    const workspaceHeader = (id) => walk(workspaceTable()).find((node) => node.tag === "th" &&
        walk(node).some((child) => child.attributes["data-fusion-workspace-sort"] === id));
    const workspaceRows = () => walk(workspaceTable()).find((node) => node.tag === "tbody").children;
    const workspaceNames = () => workspaceRows().map((row) => text(row.children[0]));
    const workspaceUnit = (label) => walk(workspaceTable().closest(".panel")).find((node) => node.tag === "button" && text(node) === label);
    return { ...view, content, table, button, header, rows, agentTable, agentButton, agentHeader, agentRows,
        modelTable, modelButton, modelHeader, modelRows, modelNames, modelUnit,
        workspaceTable, workspaceButton, workspaceHeader, workspaceRows, workspaceNames, workspaceUnit, text };
}

function normalAgentDataset() {
    return {
        UsageSessions: 3,
        UsageIncompleteSessions: 1,
        ByRoleUsage: [
            { Key: "main", Sessions: 1, Requests: 4, PremiumRequests: 0.5, AIU: 2, InputTokens: 20,
                CacheReadTokens: 80, CacheWriteTokens: 2, OutputTokens: 5 },
            { Key: "subagent", Sessions: 2, Requests: 2, PremiumRequests: 1.5, AIU: 0.5, InputTokens: 10,
                CacheReadTokens: 20, CacheWriteTokens: 3, OutputTokens: 10 },
            { Key: "unknown", Sessions: 1, Requests: 0, PremiumRequests: 0, AIU: 0, InputTokens: 0,
                CacheReadTokens: 0, CacheWriteTokens: 0, OutputTokens: 0 },
        ],
    };
}

function fusionAgentEntry(Key, Sessions, UsageRecords, values) {
    return {
        Key, Sessions, UsageRecords,
        Metrics: Object.fromEntries(Object.entries(values).map(([field, total]) => [
            field, { Total: total, ObservedRecords: UsageRecords, CompleteRecords: UsageRecords, ExcludedRecords: 0 },
        ])),
    };
}

function fusionAgentEntries() {
    return [
        fusionAgentEntry("main", 1, 2, {
            AIU: 2, PremiumRequests: 0.5, PromptTokens: 100, UncachedInputTokens: 60,
            CacheReadTokens: 20, CacheWriteTokens: 10, OutputTokens: 50, APIDurationMs: 100,
        }),
        fusionAgentEntry("subagent", 2, 4, {
            AIU: 3, PremiumRequests: 1.5, PromptTokens: 200, UncachedInputTokens: 70,
            CacheReadTokens: 25, CacheWriteTokens: 15, OutputTokens: 100, APIDurationMs: 200,
        }),
    ];
}

function completeFusionUsage() {
    const data = projectFusion(fusionDataset());
    const values = {
        AIU: 2.000000001, PremiumRequests: 0.5, PromptTokens: 100, CacheReadTokens: 30,
        CacheWriteTokens: 10, UncachedInputTokens: 60, OutputTokens: 50, APIDurationMs: 0.00000001,
    };
    data.FusionUsage.Metrics = fusionAgentEntry("main", 2, 3, values).Metrics;
    Object.assign(data.FusionUsage.Metrics.PremiumRequests, { ObservedRecords: 2, CompleteRecords: 2, ExcludedRecords: 1 });
    data.Fusion.AIURecords = 3;
    return data;
}

function summaryCards(view) {
    const container = view.nodes.flatMap(view.walk).find((node) =>
        node.className === "cards usage-cards" || node.className === "cards fusion-cards");
    const cards = view.walk(container).filter((node) => node.className === "card");
    const text = (node) => typeof node === "string" ? node : node instanceof Element ? node.children.map(text).join("") : "";
    return cards.map((node) => {
        const find = (name) => view.walk(node).find((child) => child.className === name);
        return { label: text(find("label")), value: text(find("value")), sub: text(find("sub")), title: node.attributes.title };
    });
}

function panelTable(view, title) {
    const panel = view.panels.find((entry) =>
        view.walk(entry).some((node) => node.tag === "h2" && node.children[0] === title));
    return view.walk(panel).find((node) => node.tag === "table");
}

function tableHeadings(view, table) {
    const head = view.walk(table).find((node) => node.tag === "thead");
    const text = (node) => typeof node === "string" ? node : node instanceof Element ? node.children.map(text).join("") : "";
    return head.children[0].children.map((heading) => {
        const button = view.walk(heading).find((node) => node.tag === "button");
        return button ? text(button.children[0]) : undefined;
    });
}

function tableCell(view, table, column) {
    const index = tableHeadings(view, table).indexOf(column);
    assert.ok(index >= 0, `missing column ${column}`);
    return view.walk(table).find((node) => node.tag === "tbody").children[0].children[index];
}

test("the harness warning stays visible only on VS Code, including loading, empty, and failed results", () => {
    const { context } = renderedPanels(normalizeQuery({ source: "vscode" }), {});
    const elements = {
        "vscode-harness-warning": { hidden: true },
        content: { classList: { toggle() {} }, replaceChildren() {} },
        error: { dataset: {} },
        refresh: {},
        command: {},
        "command-label": {},
    };
    context.document.getElementById = (id) => elements[id];
    context.document.querySelectorAll = () => [];
    vm.runInContext(`
        meta = {};
        fillForm = renderFilters = renderStatus = renderWarnings = renderPresets = showError = () => {};
        renderSession = renderVscode = () => [];
    `, context);
    for (const status of ["idle", "loading", "ok", "error"]) {
        context.result = { status, command: [], data: status === "ok" ? {} : null };
        vm.runInContext("state.results = { session: result, vscode: result };", context);
        for (const source of ["vscode", "session", "vscode"]) {
            context.source = source;
            vm.runInContext("state.query.source = source; render();", context);
            assert.equal(elements["vscode-harness-warning"].hidden, source !== "vscode", `${source}: ${status}`);
        }
    }
});

function filteredQuery(source = "vscode") {
    return normalizeQuery({
        source, scope: "cwd", path: "/repo", period: "7d", until: "2026-10-11",
        session: "parent", top: 3,
        filters: {
            vscode: { agent: ["Explore", "executionSubagent"], model: ["a", "b"], tool: ["read", "write"], sandbox: ["applied"] },
            session: { operation: ["read", "write"], agentMode: ["interactive", "plan"], permissionMode: ["assisted"], result: ["approved"],
                commandAccess: ["read-only", "read-write"], sandbox: ["applied", "unknown"], kind: ["shell", "read"], command: ["git", "go"],
                path: ["/src", "/test"], url: ["example"], decisionSource: ["human_response"] },
        },
    });
}

test("each panel clears only its entire matching filter group without mutating the query", () => {
    for (const source of Object.keys(FILTERS)) {
        for (const filter of FILTERS[source]) {
            const query = filteredQuery(source);
            const before = structuredClone(query);
            const patch = panelClearPatch(query, source, filter);
            assert.deepEqual(patch, { filters: { [source]: { [filter]: [] } } });
            const next = normalizeQuery(patch, query);
            const expected = structuredClone(query);
            expected.filters[source][filter] = [];
            assert.deepEqual(next, expected);
            assert.deepEqual(query, before);
            assert.equal(panelClearPatch(next, source, filter), null);
        }
    }
});

test("workspace panels clear cwd and worktree scopes to all, retaining all other conditions", () => {
    for (const source of ["session", "vscode"]) {
        for (const scope of ["cwd", "worktree"]) {
            const query = normalizeQuery({ scope }, filteredQuery(source));
            const patch = panelClearPatch(query, source, "workspace");
            assert.deepEqual(patch, { scope: "all", path: "" });
            const next = normalizeQuery(patch, query);
            assert.deepEqual(next, { ...query, scope: "all", path: "" });
            assert.equal(panelClearPatch(next, source, "workspace"), null);
        }
    }
});

test("inactive panel conditions have no clear action and unsupported axes fail explicitly", () => {
    const query = normalizeQuery({ scope: "all" });
    for (const source of Object.keys(FILTERS)) {
        for (const filter of FILTERS[source]) assert.equal(panelClearPatch(query, source, filter), null);
    }
    assert.throws(() => panelClearPatch(query, "session", "model"), /Unknown session panel filter/);
    assert.throws(() => panelClearPatch(query, "vscode", "role"), /Unknown vscode panel filter/);
});

test("Subagents Clear projects locally while keeping the model/tool filters and fetch metadata", async () => {
    let calls = 0;
    const dashboard = new Dashboard({
        query: filteredQuery(),
        cwd: "/repo",
        deps: {
            resolveRunner: async () => ({ cmd: "fake", prefix: [], label: "fake" }),
            runStats: async () => {
                calls++;
                return { data: vscodeDataset(), warnings: [], durationMs: 10 };
            },
        },
    });

    await dashboard.ensureFresh();
    const before = dashboard.results.vscode;
    const oldQuery = structuredClone(dashboard.query);
    assert.equal(before.data.LLMRequests, 5);
    await dashboard.setQuery(panelClearPatch(dashboard.query, "vscode", "agent"));
    assert.equal(calls, 1);
    assert.equal(dashboard.results.vscode.data.LLMRequests, 7);
    assert.equal(dashboard.results.vscode.ranAt, before.ranAt);
    assert.deepEqual(dashboard.results.vscode.fetchArgv, before.fetchArgv);
    assert.deepEqual(dashboard.query, {
        ...oldQuery, filters: { ...oldQuery.filters, vscode: { ...oldQuery.filters.vscode, agent: [] } },
    });
});

test("VS Code renders enabled per-panel buttons, and clicks submit only the matching clear patch", () => {
    const query = filteredQuery();
    const data = { ByModel: [{ Key: "a", Requests: 1, UsageAIU: 1 }], ByWorkspace: [{ Key: "/repo", UsageAIU: 1 }], ByTool: [], ByAgent: [] };
    const { panels, submitted, walk } = renderedPanels(query, data);
    const expected = {
        "Usage share by model": "model", Models: "model",
        "Usage share by workspace": "workspace", Workspaces: "workspace",
        Tools: "tool", Subagents: "agent",
    };
    assert.equal(panels.length, Object.keys(expected).length);
    for (const panel of panels) {
        const title = walk(panel).find((node) => node.tag === "h2").children[0];
        const button = walk(panel).find((node) => node.attributes["data-clear-filter"]);
        assert.ok(button, title);
        assert.equal(button.attributes["data-clear-filter"], expected[title]);
        assert.equal(button.attributes.disabled, undefined);
        assert.equal(button.attributes.type, "button");
        assert.ok(button.attributes["aria-label"].startsWith(`Clear ${title} `));
        button.events.click();
        assert.deepEqual(submitted.at(-1), panelClearPatch(query, "vscode", expected[title]));
    }
});

test("CLI clear buttons cover permission filters and directories but not unsupported usage axes", () => {
    const query = filteredQuery("session");
    const data = { UsageSessions: 0, ByModelUsage: [], ByRoleUsage: [], ByAgentMode: [], ByPermissionMode: [], ByDecisionSource: [], ByCWDUsage: [] };
    const { panels, submitted, walk } = renderedPanels(query, data);
    const expected = {
        Workspaces: "workspace", "Working directories": "workspace",
        "Agent modes": "agentMode", "Permission modes": "permissionMode", Results: "result", "Decision sources": "decisionSource",
        "Read-only vs read-write": "commandAccess",
        "Tool kinds": "kind", Commands: "command", Paths: "path", URLs: "url",
    };
    for (const panel of panels) {
        const title = walk(panel).find((node) => node.tag === "h2").children[0];
        const button = walk(panel).find((node) => node.attributes["data-clear-filter"]);
        if (!expected[title]) {
            assert.equal(button, undefined, title);
            continue;
        }
        assert.equal(button.attributes["data-clear-filter"], expected[title]);
        button.events.click();
        assert.deepEqual(submitted.at(-1), panelClearPatch(query, "session", expected[title]));
    }
    assert.equal(submitted.length, Object.keys(expected).length);
});

test("Results and command-access rows add their matching exact filters", () => {
    const query = normalizeQuery({ source: "session" });
    const { panels, walk, submitted } = renderedPanels(query, {
        ByResult: [{ Key: "denied-no-approval-rule", Total: 2, Denied: 2 }],
        ByReadOnly: [{ Key: "read-only", Total: 3 }, { Key: "read-write", Total: 1 }],
    });
    const panelFor = (title) => panels.find((entry) =>
        walk(entry).some((node) => node.tag === "h2" && node.children[0] === title));
    const result = panelFor("Results");
    walk(result).find((node) => node.className === "key").events.click();
    assert.deepEqual(structuredClone(submitted.at(-1)), { filters: { session: { result: ["denied-no-approval-rule"] } } });

    const access = panelFor("Read-only vs read-write");
    assert.deepEqual(walk(access).find((node) => node.className === "hint").children, ["click to filter"]);
    walk(access).find((node) => node.className === "key" && node.children[0] === "read-write").events.click();
    assert.deepEqual(structuredClone(submitted.at(-1)), { filters: { session: { commandAccess: ["read-write"] } } });
});

test("Paths, URLs, and Working directories use the same wide panel layout", () => {
    const data = {
        UsageSessions: 1,
        ByPath: [{ Key: "/repo/a+b.txt", Total: 1, Approved: 1 }],
        ByURL: [{ Key: "https://example.test/a/long/path", Total: 1, Approved: 1 }],
        ByCWDUsage: [],
    };
    const { panels, walk, submitted } = renderedPanels(normalizeQuery({ source: "session" }), data);
    for (const title of ["Paths", "URLs", "Working directories"]) {
        const panel = panels.find((entry) => walk(entry).some((node) => node.tag === "h2" && node.children[0] === title));
        assert.ok(panel, title);
        assert.equal(panel.className, "panel wide", title);
    }
    const paths = panels.find((entry) => walk(entry).some((node) => node.tag === "h2" && node.children[0] === "Paths"));
    walk(paths).find((node) => node.tag === "button" && node.className === "key").events.click();
    assert.deepEqual(structuredClone(submitted.at(-1)), { filters: { session: { path: ["^/repo/a\\+b\\.txt$"] } } });
});

test("normal Agent usage sorts each column by its displayed raw value", () => {
    const { context } = renderedPanels(normalizeQuery({ source: "session" }), {});
    const columns = vm.runInContext("agentUsageColumns(false)", context);
    const normalize = vm.runInContext("normalizeAgentUsageView", context);
    const entries = normalAgentDataset().ByRoleUsage.slice(0, 2).map(context.sessionUsageEntry);
    const before = structuredClone(entries);
    const first = ["main", "subagent"];
    const second = ["subagent", "main"];
    const ascending = {
        role: first, sessions: first, requests: second, premium: first, prompt: second,
        cacheRead: second, cacheWrite: first, uncached: second, output: first, aiu: second,
        sessionCost: second, requestCost: second, tokenCost: second,
    };
    const initial = normalize(undefined, "normal");
    assert.deepEqual(initial, { sort: "role", direction: "asc" });
    assert.deepEqual(Array.from(columns, (column) => column.id), Object.keys(ascending));
    for (const column of columns) {
        const sorted = (direction) => tableView.sortTableEntries(entries, { sort: column.id, direction }, columns, () => 0).map((entry) => entry.Key);
        assert.deepEqual(sorted("asc"), ascending[column.id], column.id);
        assert.deepEqual(sorted("desc"), [...ascending[column.id]].reverse(), `${column.id} reversed`);
        assert.deepEqual(tableView.toggleTableSort(initial, column.id, columns), { sort: column.id, direction: "desc" });
    }
    assert.deepEqual(structuredClone(entries), before);
    assert.deepEqual(normalize(null, "normal"), initial);
    assert.deepEqual(normalize({ sort: "invalid", direction: "invalid" }, "normal"), initial);
    assert.equal(normalize({ sort: "records", direction: "asc" }, "normal").sort, "role");
    assert.equal(vm.runInContext('agentRoleLabel("toString")', context), "toString");
});

test("normal Agent usage keeps unavailable subtotals last without hiding recorded zeros", () => {
    const { context } = renderedPanels(normalizeQuery({ source: "session" }), {});
    const columns = vm.runInContext("agentUsageColumns(true)", context);
    const entries = normalAgentDataset().ByRoleUsage.map(context.sessionUsageEntry);
    const sorted = (column, direction) =>
        tableView.sortTableEntries(entries, { sort: column, direction }, columns, () => 0).map((entry) => entry.Key);
    assert.deepEqual(sorted("requests", "asc"), ["subagent", "main", "unknown"]);
    assert.deepEqual(sorted("requests", "desc"), ["main", "subagent", "unknown"]);
    for (const column of ["prompt", "cacheRead", "cacheWrite", "output"]) {
        assert.equal(sorted(column, "asc").at(-1), "unknown", column);
        assert.equal(sorted(column, "desc").at(-1), "unknown", column);
    }
    assert.equal(sorted("aiu", "asc")[0], "unknown");
    assert.equal(sorted("premium", "asc")[0], "unknown");
    entries[0].Requests = 0;
    assert.deepEqual(sorted("requests", "asc"), ["main", "subagent", "unknown"]);
    entries[0].Requests = 4;
    entries[2].Requests = 3;
    assert.deepEqual(sorted("requests", "asc"), ["subagent", "unknown", "main"]);
});

test("Fusion-only Agent usage sorts all columns including complete unit costs", () => {
    const { context } = renderedPanels(normalizeQuery({ source: "session" }), {});
    const columns = vm.runInContext("FUSION_AGENT_COLUMNS", context);
    const normalize = vm.runInContext("normalizeAgentUsageView", context);
    const entries = fusionAgentEntries();
    const before = structuredClone(entries);
    const first = ["main", "subagent"];
    const second = ["subagent", "main"];
    const ascending = {
        role: first, sessions: first, records: first, premium: first, prompt: first,
        cacheRead: first, cacheWrite: first, uncached: first, output: first, apiTime: first, aiu: first,
        sessionCost: second, recordCost: second, tokenCost: second,
    };
    const initial = normalize(undefined, "fusion");
    assert.deepEqual(initial, { sort: "aiu", direction: "desc" });
    assert.deepEqual(Array.from(columns, (column) => column.id), Object.keys(ascending));
    for (const column of columns) {
        const sorted = (direction) => tableView.sortTableEntries(entries, { sort: column.id, direction }, columns, () => 0).map((entry) => entry.Key);
        assert.deepEqual(sorted("asc"), ascending[column.id], column.id);
        assert.deepEqual(sorted("desc"), [...ascending[column.id]].reverse(), `${column.id} reversed`);
        const expected = column.id === "role" || column.id === "aiu" ? "asc" : "desc";
        assert.deepEqual(tableView.toggleTableSort(initial, column.id, columns), { sort: column.id, direction: expected });
    }
    assert.deepEqual(entries, before);
    assert.deepEqual(normalize(false, "fusion"), initial);
    assert.deepEqual(normalize({ sort: "invalid", direction: "invalid" }, "fusion"), initial);
    assert.equal(normalize({ sort: "requests", direction: "asc" }, "fusion").sort, "aiu");
});

test("Fusion-only Agent usage distinguishes missing, excluded, partial, and zero metrics", () => {
    const { context } = renderedPanels(normalizeQuery({ source: "session" }), {});
    const columns = vm.runInContext("FUSION_AGENT_COLUMNS", context);
    const [main, subagent] = fusionAgentEntries();
    const absent = structuredClone(main);
    absent.Key = "absent";
    absent.Metrics.AIU = { Total: 0, ObservedRecords: 0, CompleteRecords: 0, ExcludedRecords: 0 };
    absent.Metrics.PremiumRequests = { Total: 0, ObservedRecords: 0, CompleteRecords: 0, ExcludedRecords: 2 };
    const partial = structuredClone(main);
    partial.Key = "partial";
    partial.Metrics.AIU = { Total: 1, ObservedRecords: 1, CompleteRecords: 1, ExcludedRecords: 0 };
    const zero = structuredClone(main);
    zero.Key = "zero";
    zero.Metrics.AIU.Total = 0;
    zero.Metrics.PremiumRequests.Total = 0;
    const paid = structuredClone(subagent);
    paid.Key = "paid";
    const entries = [absent, partial, zero, paid];
    const sorted = (column, direction) =>
        tableView.sortTableEntries(entries, { sort: column, direction }, columns, () => 0).map((entry) => entry.Key);
    assert.deepEqual(sorted("aiu", "asc"), ["zero", "partial", "paid", "absent"]);
    assert.deepEqual(sorted("aiu", "desc"), ["paid", "partial", "zero", "absent"]);
    assert.deepEqual(sorted("premium", "asc"), ["zero", "partial", "paid", "absent"]);
    assert.deepEqual(sorted("premium", "desc"), ["paid", "partial", "zero", "absent"]);
    for (const column of ["sessionCost", "recordCost", "tokenCost"]) {
        assert.deepEqual(sorted(column, "asc"), ["zero", "paid", "absent", "partial"], column);
        assert.deepEqual(sorted(column, "desc"), ["paid", "zero", "absent", "partial"], column);
    }
    const unitCost = vm.runInContext("fusionUnitCost", context);
    const noTokens = structuredClone(zero);
    noTokens.Metrics.PromptTokens.Total = noTokens.Metrics.OutputTokens.Total = 0;
    assert.equal(unitCost(noTokens, "mtok"), null);
    const incompleteTokens = structuredClone(main);
    incompleteTokens.Metrics.PromptTokens.CompleteRecords = 1;
    assert.equal(unitCost(incompleteTokens, "mtok"), null);
    assert.equal(unitCost({ ...paid, Sessions: 0 }, "session"), null);

    const tieFirst = structuredClone(subagent);
    tieFirst.Metrics.AIU.Total = 2;
    const ties = [tieFirst, main, absent, { ...absent, Key: "another" }];
    assert.deepEqual(tableView.sortTableEntries(ties, { sort: "aiu", direction: "desc" }, columns, () => 0).map((entry) => entry.Key),
        ["subagent", "main", "absent", "another"]);
});

test("normal Agent usage headings sort locally and preserve focus and both table scroll positions", () => {
    const data = { ...normalAgentDataset(), Fusion: fusionDataset().Fusion };
    const before = structuredClone(data);
    const app = interactiveFusion(data, { top: 1 });
    const ids = ["role", "sessions", "requests", "premium", "prompt", "cacheRead", "cacheWrite", "uncached",
        "output", "aiu", "sessionCost", "requestCost", "tokenCost"];
    for (const id of ids) {
        assert.equal(app.agentButton(id).attributes.type, "button");
        assert.equal(app.agentHeader(id).attributes.scope, "col");
        assert.ok(app.agentButton(id).attributes["aria-label"].includes("sort "));
    }
    assert.equal(app.agentHeader("role").attributes["aria-sort"], "ascending");
    assert.deepEqual(app.agentRows(), ["Main agent", "Subagents", "Unknown"]);
    const unknownRow = app.walk(app.agentTable()).find((node) => node.tag === "tbody").children[2];
    assert.deepEqual(unknownRow.children.map((cell) => cell.children[0]),
        ["Unknown", "1", "–", "0", "–", "–", "–", "–", "–", "0", "–", "–", "–"]);
    app.table().closest(".panel").scrollLeft = 70;
    app.agentTable().closest(".panel").scrollLeft = 120;
    const original = app.agentButton("requests");
    app.context.document.activeElement = original;
    original.events.click();
    assert.equal(app.agentHeader("requests").attributes["aria-sort"], "descending");
    assert.equal(app.context.document.activeElement, app.agentButton("requests"));
    assert.equal(app.table().closest(".panel").scrollLeft, 70);
    assert.equal(app.agentTable().closest(".panel").scrollLeft, 120);
    app.agentButton("requests").events.click();
    assert.equal(app.agentHeader("requests").attributes["aria-sort"], "ascending");
    assert.deepEqual(app.agentRows(), ["Subagents", "Main agent", "Unknown"]);
    const focused = app.context.document.activeElement;
    vm.runInContext("setState({ ...state });", app.context);
    assert.notEqual(app.context.document.activeElement, focused);
    assert.equal(app.context.document.activeElement, app.agentButton("requests"));
    assert.equal(app.agentTable().closest(".panel").scrollLeft, 120);
    const other = new Element("input");
    app.context.document.activeElement = other;
    vm.runInContext("setState({ ...state });", app.context);
    assert.equal(app.context.document.activeElement, other);
    assert.deepEqual(app.submitted, []);
    assert.deepEqual(data, before);
});

test("Agent usage keeps separate sort preferences for normal and Fusion-only modes", () => {
    const saved = new Map();
    const storage = {
        getItem: (key) => saved.get(key) ?? null,
        setItem: (key, value) => saved.set(key, value),
    };
    const data = { ...normalAgentDataset(), Fusion: fusionDataset().Fusion };
    const app = interactiveFusion(data, { storage });
    app.agentButton("requests").events.click();
    app.agentButton("requests").events.click();
    assert.deepEqual(JSON.parse(saved.get("attendant-dashboard.agentView")), { sort: "requests", direction: "asc" });
    app.context.nextData = projectFusion(fusionDataset());
    vm.runInContext(`
        state.query = { ...state.query, fusionOnly: true };
        state.results.session.data = nextData;
        render();
    `, app.context);
    const ids = ["role", "sessions", "records", "premium", "prompt", "cacheRead", "cacheWrite", "uncached",
        "output", "apiTime", "aiu", "sessionCost", "recordCost", "tokenCost"];
    for (const id of ids) {
        assert.equal(app.agentButton(id).attributes.type, "button");
        assert.equal(app.agentHeader(id).attributes.scope, "col");
        assert.ok(app.agentButton(id).attributes["aria-label"].includes("sort "));
    }
    assert.equal(app.agentHeader("aiu").attributes["aria-sort"], "descending");
    assert.deepEqual(app.agentRows(), ["Main agent", "Subagents"]);
    app.agentTable().closest(".panel").scrollLeft = 120;
    app.context.document.activeElement = app.agentButton("records");
    app.agentButton("records").events.click();
    assert.equal(app.context.document.activeElement, app.agentButton("records"));
    assert.equal(app.agentTable().closest(".panel").scrollLeft, 120);
    app.agentButton("records").events.click();
    assert.deepEqual(app.agentRows(), ["Subagents", "Main agent"]);
    vm.runInContext("setState({ ...state });", app.context);
    assert.equal(app.context.document.activeElement, app.agentButton("records"));
    assert.equal(app.agentTable().closest(".panel").scrollLeft, 120);
    assert.deepEqual(JSON.parse(saved.get("attendant-dashboard.fusionAgentView")), { sort: "records", direction: "asc" });
    const filteredText = app.walk(app.content).flatMap((node) => node.children.filter((child) => typeof child === "string")).join(" ");
    assert.match(filteredText, /Top selects role groups by recorded AIU before sorting; set Top to 0/);
    for (const title of ["Models", "Workspaces"]) {
        const panel = app.walk(app.content).find((node) => node.tag === "section" &&
            app.walk(node).some((child) => child.tag === "h2" && child.children[0] === title));
        assert.equal(app.walk(panel).some((node) => Object.hasOwn(node.attributes, "data-agent-sort")), false);
    }
    app.context.normalData = data;
    vm.runInContext(`
        state.query = { ...state.query, fusionOnly: false };
        state.results.session.data = normalData;
        render();
    `, app.context);
    assert.equal(app.agentHeader("requests").attributes["aria-sort"], "ascending");
    assert.deepEqual(app.agentRows(), ["Subagents", "Main agent", "Unknown"]);
    const reloaded = interactiveFusion(projectFusion(fusionDataset()), { storage, fusionOnly: true });
    assert.equal(reloaded.agentHeader("records").attributes["aria-sort"], "ascending");
    assert.deepEqual(reloaded.agentRows(), ["Subagents", "Main agent"]);
    assert.deepEqual(app.submitted, []);
});

test("Fusion-only Agent usage sorts only the role groups selected by Top", () => {
    const data = projectFusion(fusionDataset(1));
    const before = structuredClone(data);
    const app = interactiveFusion(data, { fusionOnly: true, top: 1 });
    assert.deepEqual(app.agentRows(), ["Main agent"]);
    app.agentButton("role").events.click();
    assert.deepEqual(app.agentRows(), ["Main agent"]);
    assert.equal(data.FusionUsage.RoleGroups, 2);
    assert.deepEqual(app.submitted, []);
    assert.deepEqual(data, before);
    const all = interactiveFusion(projectFusion(fusionDataset()), { fusionOnly: true, top: 0 });
    assert.deepEqual(all.agentRows(), ["Main agent", "Subagents"]);
});

test("Agent usage sorting works when preferences cannot be loaded or saved", () => {
    for (const readFailure of ["invalid", "throw"]) {
        const storage = {
            getItem: (key) => {
                if (key !== "attendant-dashboard.agentView" && key !== "attendant-dashboard.fusionAgentView") return null;
                if (readFailure === "throw") throw new Error("storage unavailable");
                return "{invalid";
            },
            setItem: () => { throw new Error("storage unavailable"); },
        };
        for (const fusionOnly of [false, true]) {
            const data = fusionOnly ? projectFusion(fusionDataset()) : normalAgentDataset();
            const app = interactiveFusion(data, { storage, fusionOnly });
            const defaultColumn = fusionOnly ? "aiu" : "role";
            assert.equal(app.agentHeader(defaultColumn).attributes["aria-sort"], fusionOnly ? "descending" : "ascending");
            assert.ok(app.warnings.some(([message]) => message.includes("Could not load table view preferences")));
            app.agentButton(fusionOnly ? "records" : "requests").events.click();
            assert.ok(app.warnings.some(([message]) => message.includes("Could not save table view preferences")));
        }
    }
});

test("CLI Fusion panel shows recorded pattern and phase coverage without inventing costs", () => {
    const data = { Fusion: {
        Sessions: 1, ResolvedTurns: 2, UnidentifiedResolutions: 1, TurnsWithUsage: 1, PhasesWithUsage: 1,
        UsageRecords: 3, AIU: 0.000000001, AIURecords: 1, MissingFusionIDRecords: 1, MissingPhaseIdentityRecords: 1,
        ByPattern: [{ Key: "cascade", Sessions: 1, ResolvedTurns: 2, UnidentifiedResolutions: 1 }],
        ByPhase: [
            { Pattern: "cascade", Phase: "judge", Role: "judge", Model: "model-a", Sessions: 1,
                Turns: 1, Phases: 1, UsageRecords: 2, AIU: 0.000000001, AIURecords: 1, MissingPhaseIdentityRecords: 0 },
            { Pattern: "single", Phase: "primary", Role: "solver", Model: "model-b", Sessions: 1,
                Turns: 0, Phases: 0, UsageRecords: 1, AIU: 0, AIURecords: 0, MissingPhaseIdentityRecords: 1 },
        ],
    } };
    const { panels, walk } = renderedPanels(normalizeQuery({ source: "session" }), data);
    const fusion = panels.find((p) => walk(p).some((n) => n.tag === "h2" && n.children[0] === "Copilot Fusion"));
    assert.ok(fusion);
    const cards = walk(fusion).find((node) => node.className === "cards fusion-cards");
    assert.ok(cards);
    const text = (node) => typeof node === "string" ? node : node instanceof Element ? node.children.map(text).join(" ") : "";
    const content = text(fusion);
    for (const value of ["Resolved patterns", "cascade", "judge", "model-a", "Phase usage",
        "≥ 0", "1 / 3 records report AIU", "–", "checkpoint-only", "lack a complete phase identity",
        "not an atomic snapshot"]) {
        assert.ok(content.includes(value), `missing ${value}: ${content}`);
    }
    assert.equal(walk(fusion).some((node) => node.attributes["data-clear-filter"]), false);
});

test("Fusion disclosure preserves its state and focus in normal and Fusion-only views", () => {
    for (const fusionOnly of [false, true]) {
        const data = fusionOnly ? projectFusion(fusionDataset()) : { Fusion: fusionDataset().Fusion };
        const before = structuredClone(data);
        const app = interactiveFusion(data, { fusionOnly });
        const disclosure = () => app.walk(app.content).find((node) =>
            Object.hasOwn(node.attributes, "data-fusion-disclosure"));
        const summary = () => disclosure().children[0];
        assert.equal(disclosure().tag, "details");
        assert.equal(disclosure().open, true);
        assert.equal(summary().tag, "summary");
        assert.equal(summary().children[0].children[0], "Copilot Fusion");
        const panel = () => disclosure().closest(".panel");
        assert.equal(panel().children[1].children[0].children[0], fusionOnly ? "tagged usage only" : "recorded events only");
        assert.ok(app.walk(panel()).some((node) => node.className === "cards fusion-cards"));
        disclosure().open = false;
        app.context.document.activeElement = summary();
        vm.runInContext("render();", app.context);
        assert.equal(disclosure().open, false);
        assert.equal(app.context.document.activeElement, summary());
        vm.runInContext("setState({ ...state });", app.context);
        assert.equal(disclosure().open, false);
        assert.equal(app.context.document.activeElement, summary());
        disclosure().open = true;
        vm.runInContext("render();", app.context);
        assert.equal(disclosure().open, true);
        app.button("phase").events.click();
        assert.equal(disclosure().open, true);
        assert.deepEqual(app.submitted, []);
        assert.deepEqual(data, before);
    }
});

test("every panel has an independent expanded disclosure in each dashboard view", () => {
    const data = {
        UsageSessions: 1,
        ByModelUsage: [{ Key: "model-a", Sessions: 1, Requests: 1, AIU: 1 }],
        ByCWDUsage: [{ Key: "/fictional/project", Sessions: 1, Requests: 1, AIU: 1 }],
        ByRoleUsage: [], AskUser: {}, Fusion: fusionDataset().Fusion,
        ByAgentMode: [], ByDecisionSource: [], SessionActivity: activityDataset(),
    };
    const views = [
        {
            query: normalizeQuery({ source: "session" }), data,
            expected: ["Models", "Workspaces", "Copilot Fusion", "Summary", "Agent modes", "Paths", "Session activity"],
        },
        {
            query: normalizeQuery({ source: "session", fusionOnly: true }), data: projectFusion(fusionDataset()),
            expected: ["Models", "Workspaces", "Copilot Fusion", "Agent usage"],
        },
        {
            query: normalizeQuery({ source: "vscode" }),
            data: {
                ByModel: [{ Key: "model-a", Requests: 1, UsageAIU: 1 }],
                ByWorkspace: [{ Key: "/fictional/project", UsageAIU: 1 }],
                ByTool: [], ByAgent: [], UserQuestions: {},
            },
            expected: ["Models", "Workspaces", "Tools", "Subagents", "User questions"],
        },
    ];
    for (const { query, data: viewData, expected } of views) {
        const { panels, walk } = renderedPanels(query, viewData);
        const titles = panels.map((panel) => walk(panel).find((node) => node.tag === "h2").children[0]);
        for (const title of expected) assert.ok(titles.includes(title), `${query.source}: ${title}`);
        const keys = new Set();
        for (const panel of panels) {
            const [disclosure, actions, body] = panel.children;
            const title = walk(panel).find((node) => node.tag === "h2").children[0];
            assert.equal(disclosure.tag, "details", title);
            assert.equal(disclosure.open, true, title);
            const summary = disclosure.children[0];
            assert.equal(summary.tag, "summary", title);
            assert.equal(actions.className, "panel-actions", title);
            assert.equal(body.className, "panel-body", title);
            assert.equal(summary.attributes["aria-controls"], body.attributes.id, title);
            const key = Object.entries(summary.attributes).find(([name]) => name.startsWith("data-") && name.endsWith("-toggle"))?.[1];
            assert.ok(key, title);
            assert.equal(keys.has(key), false, `duplicate disclosure key: ${key}`);
            keys.add(key);
            const clear = walk(panel).find((node) => node.attributes["data-clear-filter"]);
            if (clear) {
                assert.equal(clear.parentElement, actions, title);
                assert.equal(clear.attributes["data-clear-panel"], key, title);
                assert.equal(walk(summary).includes(clear), false, title);
            }
        }
    }
});

test("panel disclosures survive rerenders, absent data, parent collapse, and view changes", () => {
    const data = {
        UsageSessions: 1,
        ByModelUsage: [{ Key: "model-a", Sessions: 1, Requests: 1, AIU: 1 }],
        ByCWDUsage: [{ Key: "/fictional/project", Sessions: 1, Requests: 1, AIU: 1 }],
        ByRoleUsage: [], Fusion: fusionDataset().Fusion,
    };
    const app = interactiveFusion(data);
    const disclosure = (name) => app.walk(app.content).find((node) =>
        Object.hasOwn(node.attributes, `data-${name}-disclosure`));
    const summary = (name) => disclosure(name).children[0];
    for (const name of ["panel-session-models", "panel-session-workspaces", "panel-session-paths"]) {
        assert.equal(disclosure(name).open, true, name);
        disclosure(name).open = false;
    }
    disclosure("permissions").open = false;
    app.context.document.activeElement = summary("panel-session-workspaces");
    vm.runInContext("render();", app.context);
    assert.equal(app.context.document.activeElement, summary("panel-session-workspaces"));
    assert.equal(disclosure("panel-session-models").open, false);
    assert.equal(disclosure("panel-session-workspaces").open, false);
    assert.equal(disclosure("panel-session-paths").open, false);
    assert.equal(disclosure("permissions").open, false);
    assert.equal(disclosure("fusion").open, true);

    vm.runInContext(`
        state.results.session.data = { ...data, ByModelUsage: undefined };
        render();
    `, app.context);
    assert.equal(disclosure("panel-session-models"), undefined);
    vm.runInContext("state.results.session.data = data; render();", app.context);
    assert.equal(disclosure("panel-session-models").open, false);

    app.context.fusionData = projectFusion(fusionDataset());
    vm.runInContext(`
        state.query = { ...state.query, fusionOnly: true };
        state.results.session.data = fusionData;
        render();
    `, app.context);
    assert.equal(disclosure("panel-session-models").open, false);
    assert.equal(disclosure("panel-session-workspaces").open, false);
    assert.equal(disclosure("fusion").open, true);

    vm.runInContext(`
        state.results.vscode = { status: "ok", data: { ByModel: [], ByWorkspace: [], ByTool: [] }, command: [] };
        state.query = { ...state.query, source: "vscode" };
        render();
    `, app.context);
    assert.equal(disclosure("panel-vscode-models").open, true);
    assert.equal(disclosure("panel-vscode-workspaces").open, true);
    disclosure("panel-vscode-models").open = false;
    vm.runInContext(`
        state.query = { ...state.query, source: "session", fusionOnly: false };
        state.results.session.data = data;
        render();
    `, app.context);
    assert.equal(disclosure("panel-session-models").open, false);
    assert.equal(disclosure("panel-session-workspaces").open, false);
    assert.equal(disclosure("panel-session-paths").open, false);
    disclosure("panel-session-models").open = true;
    vm.runInContext('state.query = { ...state.query, source: "vscode" }; render();', app.context);
    assert.equal(disclosure("panel-vscode-models").open, false);
    assert.equal(disclosure("panel-vscode-workspaces").open, true);
    assert.deepEqual(app.submitted, []);
});

test("Option-clicking any category or panel summary toggles every disclosure", () => {
    const app = interactiveFusion({
        UsageSessions: 1, ByModelUsage: [], ByRoleUsage: [], ByCWDUsage: [],
        Fusion: fusionDataset().Fusion, SessionActivity: activityDataset(),
    });
    const disclosures = () => app.walk(app.content).filter((node) => node.tag === "details" &&
        ["panel-disclosure", "category-disclosure"].some((name) => node.className?.split(" ").includes(name)));
    const summary = (name) => app.walk(app.content).find((node) =>
        Object.hasOwn(node.attributes, `data-${name}-toggle`));
    const optionClick = (name) => {
        let prevented = false;
        summary(name).events.click({ altKey: true, preventDefault() { prevented = true; } });
        assert.equal(prevented, true);
    };
    const excluded = () => disclosures().filter((disclosure) =>
        disclosure.hasAttribute("data-usage-disclosure") || disclosure.hasAttribute("data-permissions-disclosure"));
    const included = () => disclosures().filter((disclosure) => !excluded().includes(disclosure));
    assert.ok(included().length > 5);
    assert.ok(disclosures().every((disclosure) => disclosure.open));

    optionClick("usage");
    assert.ok(excluded().every((disclosure) => disclosure.open));
    assert.ok(included().every((disclosure) => !disclosure.open));
    vm.runInContext("render();", app.context);
    assert.ok(excluded().every((disclosure) => disclosure.open));
    assert.ok(included().every((disclosure) => !disclosure.open));

    optionClick("panel-session-models");
    assert.ok(excluded().every((disclosure) => disclosure.open));
    assert.ok(included().every((disclosure) => disclosure.open));
    const before = disclosures().map((disclosure) => disclosure.open);
    let prevented = false;
    summary("panel-session-models").events.click({ altKey: false, preventDefault() { prevented = true; } });
    assert.equal(prevented, false);
    assert.deepEqual(disclosures().map((disclosure) => disclosure.open), before);

    optionClick("fusion");
    assert.ok(excluded().every((disclosure) => disclosure.open));
    assert.ok(included().every((disclosure) => !disclosure.open));
    assert.deepEqual(app.submitted, []);
});

test("Clear stays separate from a collapsed panel and restores focus without stealing it", () => {
    const app = interactiveFusion({ UsageSessions: 1, ByModelUsage: [], ByCWDUsage: [] });
    const disclosure = (name) => app.walk(app.content).find((node) =>
        Object.hasOwn(node.attributes, `data-${name}-disclosure`));
    const summary = () => disclosure("panel-session-workspaces").children[0];
    const clear = () => app.walk(app.content).find((node) =>
        node.attributes["data-clear-panel"] === "panel-session-workspaces");
    vm.runInContext('state.query = { ...state.query, scope: "cwd", path: "/fictional/project" }; render();', app.context);
    assert.equal(clear().disabled, false);
    disclosure("panel-session-workspaces").open = false;
    app.context.document.activeElement = clear();
    clear().events.click();
    assert.deepEqual(app.submitted.at(-1), { scope: "all", path: "" });
    assert.equal(disclosure("panel-session-workspaces").open, false);
    vm.runInContext('state.query = { ...state.query, scope: "all", path: "" }; render();', app.context);
    assert.equal(clear().disabled, true);
    assert.equal(disclosure("panel-session-workspaces").open, false);
    assert.equal(app.context.document.activeElement, summary());

    vm.runInContext('state.query = { ...state.query, scope: "cwd", path: "/fictional/project" }; render();', app.context);
    clear().events.click();
    const otherSummary = () => disclosure("panel-session-models").children[0];
    app.context.document.activeElement = otherSummary();
    vm.runInContext('state.query = { ...state.query, scope: "all", path: "" }; render();', app.context);
    assert.equal(app.context.document.activeElement, otherSummary());
});

test("permission panels are grouped into one collapsible Permissions category", () => {
    const data = {
        UsageSessions: 1, ByModelUsage: [], ByRoleUsage: [], ByCWDUsage: [],
        Sessions: 1, Requests: 1, ByAgentMode: [], ByDecisionSource: [], SessionActivity: activityDataset(),
    };
    const { nodes, walk } = renderedPanels(normalizeQuery({ source: "session" }), data);
    const categories = nodes.filter((node) => node?.className === "category-disclosure");
    const categoryTitle = (node) => node.children[0].children[0].children[0];
    assert.deepEqual(Array.from(categories, categoryTitle), ["Usage", "Permissions"]);
    const panelTitles = (node) => walk(node).filter((child) => child.tag === "section")
        .map((panel) => walk(panel).find((child) => child.tag === "h2").children[0]);
    assert.deepEqual(panelTitles(categories[1]), [
        "Summary", "Agent modes", "Results", "Decision sources", "Read-only vs read-write",
        "Tool kinds", "Commands", "Paths", "URLs", "Working directories",
    ]);
    assert.ok(panelTitles(categories[0]).includes("Agent usage"));
    assert.equal(panelTitles(categories[0]).includes("Summary"), false);
    for (const category of categories) {
        assert.equal(category.tag, "details");
        assert.equal(category.open, true);
        assert.equal(category.children[0].tag, "summary");
    }
    assert.ok(nodes.indexOf(categories[1]) < nodes.findIndex((node) =>
        walk(node).some((child) => Object.hasOwn(child.attributes, "data-activity-disclosure"))));
});

test("permission panel contents align to the top of their grid cells", () => {
    const css = readFileSync(new URL("../ui/styles.css", import.meta.url), "utf8");
    assert.match(css, /\[data-permissions-disclosure\]\s*>\s*\.grid\s*>\s*\.panel\s*\{[^}]*align-content:\s*start\s*;/);
});

test("every dashboard category collapses independently and keeps state and focus across renders", () => {
    const app = interactiveFusion({
        UsageSessions: 1, ByModelUsage: [], ByRoleUsage: [], ByCWDUsage: [],
        Fusion: fusionDataset().Fusion, SessionActivity: activityDataset(),
    });
    const disclosure = (name) => app.walk(app.content).find((node) =>
        Object.hasOwn(node.attributes, `data-${name}-disclosure`));
    const summary = (name) => disclosure(name).children[0];
    for (const name of ["usage", "permissions", "fusion", "activity"]) assert.equal(disclosure(name).open, true, name);
    assert.ok(app.walk(disclosure("usage")).includes(disclosure("fusion")));
    disclosure("permissions").open = false;
    app.context.document.activeElement = summary("permissions");
    vm.runInContext("render();", app.context);
    assert.equal(disclosure("permissions").open, false);
    assert.equal(app.context.document.activeElement, summary("permissions"));
    for (const name of ["usage", "fusion", "activity"]) assert.equal(disclosure(name).open, true, name);
    disclosure("usage").open = false;
    vm.runInContext("render();", app.context);
    assert.equal(disclosure("usage").open, false);
    assert.equal(disclosure("fusion").open, true);

    app.context.fusionData = projectFusion(fusionDataset());
    vm.runInContext(`
        state.query = { ...state.query, fusionOnly: true };
        state.results.session.data = fusionData;
        render();
    `, app.context);
    assert.equal(disclosure("fusion-usage").open, true);
    disclosure("fusion-usage").open = false;
    vm.runInContext("render();", app.context);
    assert.equal(disclosure("fusion-usage").open, false);

    vm.runInContext(`
        state.results.vscode = { status: "ok", data: { ByModel: [], ByTool: [] }, command: [] };
        state.query = { ...state.query, source: "vscode" };
        render();
    `, app.context);
    const titles = (name) => app.walk(disclosure(name)).filter((node) => node.tag === "section")
        .map((panel) => app.walk(panel).find((node) => node.tag === "h2").children[0]);
    assert.equal(summary("vscode-usage").children[0].children[0], "Usage");
    assert.ok(app.walk(disclosure("vscode-usage")).some((node) => node.className === "cards usage-cards"));
    assert.deepEqual(titles("vscode-tools"), ["Tools", "Subagents"]);
    disclosure("vscode-tools").open = false;
    vm.runInContext("render();", app.context);
    assert.equal(disclosure("vscode-tools").open, false);
    assert.equal(disclosure("vscode-usage").open, true);

    vm.runInContext(`
        state.query = { ...state.query, source: "session", fusionOnly: false };
        state.results.session.data = data;
        render();
    `, app.context);
    assert.equal(disclosure("usage").open, false);
    assert.equal(disclosure("permissions").open, false);
    assert.equal(disclosure("activity").open, true);
    assert.deepEqual(app.submitted, []);
});

test("Fusion and session activity disclosures stay independent across view changes", () => {
    const app = interactiveFusion({ Fusion: fusionDataset().Fusion, SessionActivity: activityDataset() });
    const disclosure = (name) => app.walk(app.content).find((node) =>
        Object.hasOwn(node.attributes, `data-${name}-disclosure`));
    disclosure("fusion").open = false;
    disclosure("activity").open = false;
    vm.runInContext("render();", app.context);
    disclosure("fusion").open = true;
    vm.runInContext("render();", app.context);
    assert.equal(disclosure("fusion").open, true);
    assert.equal(disclosure("activity").open, false);

    disclosure("fusion").open = false;
    app.context.fusionData = projectFusion(fusionDataset());
    vm.runInContext(`
        state.query = { ...state.query, fusionOnly: true };
        state.results.session.data = fusionData;
        render();
    `, app.context);
    assert.equal(disclosure("fusion").open, false);
    assert.equal(disclosure("activity"), undefined);

    vm.runInContext(`
        state.results.vscode = { status: "ok", data: {}, command: [] };
        state.query = { ...state.query, source: "vscode" };
        render();
    `, app.context);
    assert.equal(disclosure("fusion"), undefined);
    vm.runInContext(`
        state.query = { ...state.query, source: "session", fusionOnly: false };
        state.results.session.data = data;
        render();
    `, app.context);
    assert.equal(disclosure("fusion").open, false);
    assert.equal(disclosure("activity").open, false);
    assert.deepEqual(app.submitted, []);
});

test("Copilot Fusion appears directly before Models in CLI views", () => {
    const data = {
        UsageSessions: 1,
        ByModelUsage: [{ Key: "model-a", Sessions: 1, Requests: 1, AIU: 1 }],
        ByCWDUsage: [{ Key: "/fictional/project", Sessions: 1, Requests: 1, AIU: 1 }],
        ByRoleUsage: [],
        AskUser: {},
        Fusion: fusionDataset().Fusion,
    };
    const before = structuredClone(data);
    const normal = renderedPanels(normalizeQuery({ source: "session" }), data);
    const titles = (view) => Array.from(view.panels, (panel) =>
        view.walk(panel).find((node) => node.tag === "h2")?.children[0]);
    assert.deepEqual(titles(normal).slice(0, 8), [
        "Usage share by model", "Copilot Fusion", "Models", "Usage share by workspace",
        "Workspaces", "User questions", "Agent usage", "Summary",
    ]);
    assert.equal(titles(normal).filter((title) => title === "Copilot Fusion").length, 1);
    assert.deepEqual(data, before);

    const withoutShare = renderedPanels(normalizeQuery({ source: "session" }), { ...data, ByModelUsage: [] });
    assert.deepEqual(titles(withoutShare).slice(0, 2), ["Copilot Fusion", "Models"]);
    const withoutFusion = renderedPanels(normalizeQuery({ source: "session" }), { ...data, Fusion: null });
    assert.deepEqual(titles(withoutFusion).slice(0, 2), ["Usage share by model", "Models"]);
    assert.equal(titles(withoutFusion).includes("Copilot Fusion"), false);
    for (const legacy of [
        { UsageSessions: 1, Fusion: data.Fusion },
        { Fusion: data.Fusion },
    ]) {
        const view = renderedPanels(normalizeQuery({ source: "session" }), legacy);
        assert.equal(titles(view).filter((title) => title === "Copilot Fusion").length, 1);
        assert.ok(titles(view).indexOf("Copilot Fusion") < titles(view).indexOf("Summary"));
        if (legacy.UsageSessions !== undefined) {
            assert.deepEqual(titles(view).slice(0, 2), ["Copilot Fusion", "Workspaces"]);
        }
    }

    const vscode = renderedPanels(normalizeQuery({ source: "vscode" }), {
        ByModel: [{ Key: "model-a", Requests: 1, UsageAIU: 1 }],
    });
    assert.deepEqual(titles(vscode).slice(0, 2), ["Usage share by model", "Models"]);
    assert.equal(titles(vscode).includes("Copilot Fusion"), false);
});

test("Fusion phase columns sort raw values in both directions without changing the source", () => {
    const { context } = renderedPanels(normalizeQuery({ source: "session" }), {});
    const { columns, normalize, sort, toggle } = vm.runInContext(`({
        columns: FUSION_PHASE_COLUMNS, normalize: normalizeFusionPhaseView,
        sort: sortFusionPhases, toggle: toggleFusionPhaseSort,
    })`, context);
    const entries = [
        { Key: "first", Pattern: "zeta", Phase: "alpha", Role: "zeta", Model: "model-a", Sessions: 2,
            Turns: 1, Phases: 7, UsageRecords: 10, AIU: 9, AIURecords: 2, MissingPhaseIdentityRecords: 3 },
        { Key: "second", Pattern: "alpha", Phase: "beta", Role: "alpha", Model: "model-z", Sessions: 1,
            Turns: 2, Phases: 5, UsageRecords: 20, AIU: 2, AIURecords: 10, MissingPhaseIdentityRecords: 4 },
    ];
    const before = structuredClone(entries);
    const first = ["first", "second"];
    const second = ["second", "first"];
    const ascending = {
        pattern: second, phase: first, role: second, model: first, sessions: second, turns: first,
        phases: second, records: first, aiu: second, coverage: first, missingIdentity: first,
    };
    const initial = normalize();
    assert.deepEqual(initial, { sort: "records", direction: "desc" });
    assert.deepEqual(Array.from(columns, (column) => column.id), Object.keys(ascending));
    for (const column of columns) {
        assert.deepEqual(Array.from(sort(entries, { sort: column.id, direction: "asc" }), (entry) => entry.Key),
            ascending[column.id], column.id);
        assert.deepEqual(Array.from(sort(entries, { sort: column.id, direction: "desc" }), (entry) => entry.Key),
            [...ascending[column.id]].reverse(), `${column.id} reversed`);
        const next = toggle(initial, column.id);
        const expected = column.direction === "asc" || column.id === "records" ? "asc" : "desc";
        assert.deepEqual(next, { sort: column.id, direction: expected });
        assert.notEqual(toggle(next, column.id).direction, next.direction);
    }
    assert.deepEqual(entries, before);
    for (const saved of [null, false, "invalid", { sort: "invalid", direction: "invalid" }]) {
        assert.deepEqual(normalize(saved), initial);
    }
    assert.deepEqual(normalize(JSON.parse(JSON.stringify({ sort: "coverage", direction: "asc" }))),
        { sort: "coverage", direction: "asc" });
});

test("Fusion phase sorting distinguishes missing AIU, zero values, coverage ratios, and stable ties", () => {
    const { context } = renderedPanels(normalizeQuery({ source: "session" }), {});
    const sort = vm.runInContext("sortFusionPhases", context);
    const names = (entries, column, direction) =>
        Array.from(sort(entries, { sort: column, direction }), (entry) => entry.Key);
    const coverage = [
        { Key: "unknown", UsageRecords: 0, AIURecords: 0 },
        { Key: "half-a", UsageRecords: 2, AIURecords: 1 },
        { Key: "tenth", UsageRecords: 10, AIURecords: 1 },
        { Key: "zero", UsageRecords: 10, AIURecords: 0 },
        { Key: "half-b", UsageRecords: 4, AIURecords: 2 },
        { Key: "fifth", UsageRecords: 10, AIURecords: 2 },
    ];
    assert.deepEqual(names(coverage, "coverage", "asc"), ["zero", "tenth", "fifth", "half-a", "half-b", "unknown"]);
    assert.deepEqual(names(coverage, "coverage", "desc"), ["half-a", "half-b", "fifth", "tenth", "zero", "unknown"]);
    const aiu = [
        { Key: "unreported", UsageRecords: 1, AIURecords: 0, AIU: 0 },
        { Key: "partial", UsageRecords: 2, AIURecords: 1, AIU: 0.5 },
        { Key: "zero", UsageRecords: 1, AIURecords: 1, AIU: 0 },
        { Key: "complete", UsageRecords: 1, AIURecords: 1, AIU: 1 },
    ];
    assert.deepEqual(names(aiu, "aiu", "asc"), ["zero", "partial", "complete", "unreported"]);
    assert.deepEqual(names(aiu, "aiu", "desc"), ["complete", "partial", "zero", "unreported"]);
    const models = [
        { Key: "unknown", Model: "" }, { Key: "zeta", Model: "model-z" }, { Key: "alpha", Model: "model-a" },
    ];
    assert.deepEqual(names(models, "model", "asc"), ["alpha", "zeta", "unknown"]);
    const original = [
        { Key: "first", Pattern: "alpha", UsageRecords: 4 },
        { Key: "second", Pattern: "beta", UsageRecords: 4 },
        { Key: "third", Pattern: "gamma", UsageRecords: 1 },
    ];
    assert.deepEqual(names(original, "pattern", "desc"), ["third", "second", "first"]);
    assert.deepEqual(names(original, "records", "desc"), ["first", "second", "third"]);
});

test("Fusion phase headings sort both views locally and preserve focus and scroll on updates", () => {
    const ids = ["pattern", "phase", "role", "model", "sessions", "turns", "phases", "records", "aiu", "coverage", "missingIdentity"];
    for (const fusionOnly of [false, true]) {
        const data = fusionOnly ? projectFusion(fusionDataset()) : { Fusion: fusionDataset().Fusion };
        const before = structuredClone(data);
        const app = interactiveFusion(data, { fusionOnly });
        for (const id of ids) {
            assert.equal(app.button(id).attributes.type, "button");
            assert.equal(app.header(id).attributes.scope, "col");
            assert.ok(app.button(id).attributes["aria-label"].includes("sort "));
        }
        assert.equal(app.header("records").attributes["aria-sort"], "descending");
        assert.equal(app.header("aiu").attributes["aria-sort"], "none");
        assert.deepEqual(app.rows(), ["cascade", "single"]);
        app.table().closest(".panel").scrollLeft = 120;
        const originalButton = app.button("aiu");
        app.context.document.activeElement = originalButton;
        originalButton.events.click();
        assert.equal(app.header("aiu").attributes["aria-sort"], "descending");
        assert.equal(app.button("aiu").attributes["aria-label"], "AIU: sort ascending");
        assert.equal(app.context.document.activeElement, app.button("aiu"));
        assert.notEqual(app.context.document.activeElement, originalButton);
        assert.equal(app.table().closest(".panel").scrollLeft, 120);
        app.button("aiu").events.click();
        assert.equal(app.header("aiu").attributes["aria-sort"], "ascending");
        assert.deepEqual(app.rows(), ["single", "cascade"]);
        const beforeUpdate = app.context.document.activeElement;
        vm.runInContext("setState({ ...state });", app.context);
        assert.notEqual(app.context.document.activeElement, beforeUpdate);
        assert.equal(app.context.document.activeElement, app.button("aiu"));
        assert.equal(app.table().closest(".panel").scrollLeft, 120);
        const movedFocus = new Element("input");
        app.context.document.activeElement = movedFocus;
        vm.runInContext("setState({ ...state });", app.context);
        assert.equal(app.context.document.activeElement, movedFocus);
        assert.deepEqual(app.submitted, []);
        assert.deepEqual(data, before);
        const text = app.walk(app.content).flatMap((node) => node.children.filter((child) => typeof child === "string")).join(" ");
        assert.match(text, /Top selects phase groups by record count before sorting; set Top to 0/);
    }
});

test("Fusion phase sort preferences survive mode changes and reloads", () => {
    const saved = new Map();
    const storage = {
        getItem: (key) => saved.get(key) ?? null,
        setItem: (key, value) => saved.set(key, value),
    };
    const app = interactiveFusion({ Fusion: fusionDataset().Fusion }, { storage });
    app.button("coverage").events.click();
    assert.deepEqual(app.rows(), ["single", "cascade"]);
    assert.deepEqual(JSON.parse(saved.get("attendant-dashboard.fusionPhaseView")), { sort: "coverage", direction: "desc" });
    app.context.nextData = projectFusion(fusionDataset());
    vm.runInContext(`
        state.query = { ...state.query, fusionOnly: true };
        state.results.session.data = nextData;
        render();
    `, app.context);
    assert.equal(app.header("coverage").attributes["aria-sort"], "descending");
    assert.deepEqual(app.rows(), ["single", "cascade"]);
    const reloaded = interactiveFusion(projectFusion(fusionDataset()), { storage, fusionOnly: true });
    assert.equal(reloaded.header("coverage").attributes["aria-sort"], "descending");
    assert.deepEqual(reloaded.rows(), ["single", "cascade"]);
});

test("Fusion phase sorting remains usable when stored preferences cannot be read or written", () => {
    const storage = {
        getItem: (key) => key === "attendant-dashboard.fusionPhaseView" ? "{invalid" : null,
        setItem: () => { throw new Error("storage unavailable"); },
    };
    const app = interactiveFusion({ Fusion: fusionDataset().Fusion }, { storage });
    assert.equal(app.header("records").attributes["aria-sort"], "descending");
    assert.ok(app.warnings.some(([message]) => message.includes("Could not load table view preferences")));
    app.button("coverage").events.click();
    assert.equal(app.header("coverage").attributes["aria-sort"], "descending");
    assert.deepEqual(app.rows(), ["single", "cascade"]);
    assert.ok(app.warnings.some(([message]) => message.includes("Could not save table view preferences")));
});

test("CLI Fusion cards format AIU to two decimal places and retain partial-coverage indicators", () => {
    const data = { Fusion: {
        Sessions: 1, UsageRecords: 2, AIU: 1234.123456789, AIURecords: 1,
        ByPattern: [], ByPhase: [],
    } };
    const { panels, walk } = renderedPanels(normalizeQuery({ source: "session" }), data);
    const fusion = panels.find((panel) => walk(panel).some((node) => node.tag === "h2" && node.children[0] === "Copilot Fusion"));
    const card = walk(fusion).find((node) => node.className === "card" &&
        walk(node).some((child) => child.className === "label" && child.children[0] === "Recorded AIU"));
    const value = walk(card).find((node) => node.className === "value");
    assert.equal(value.children[0], "≥ 1,234.12");
});

test("CLI Fusion panel distinguishes absent support from no recorded events", () => {
    const query = normalizeQuery({ source: "session" });
    const title = (panel, walk) => walk(panel).find((node) => node.tag === "h2")?.children[0];
    for (const data of [{}, { Fusion: null }]) {
        const { panels, walk } = renderedPanels(query, data);
        assert.equal(panels.some((panel) => title(panel, walk) === "Copilot Fusion"), false);
    }
    const { panels, walk } = renderedPanels(query, { Fusion: {
        Sessions: 0, ResolvedTurns: 0, UnidentifiedResolutions: 0, TurnsWithUsage: 0, PhasesWithUsage: 0,
        UsageRecords: 0, AIU: 0, AIURecords: 0, MissingFusionIDRecords: 0, MissingPhaseIdentityRecords: 0,
        ByPattern: [], ByPhase: [],
    } });
    const panel = panels.find((entry) => title(entry, walk) === "Copilot Fusion");
    assert.ok(panel);
    assert.ok(walk(panel).some((node) => node.children.some((child) =>
        typeof child === "string" && child.includes("Older or incomplete logs cannot establish whether Fusion was used"))));
});

test("Fusion-only rendering includes usage coverage without permission, resolution, or checkpoint data", () => {
    const query = normalizeQuery({ source: "session", fusionOnly: true });
    const data = projectFusion(fusionDataset());
    const { nodes, panels, walk } = renderedPanels(query, data);
    const text = (node) => typeof node === "string" ? node : node instanceof Element ? node.children.map(text).join(" ") : "";
    const content = nodes.map(text).join(" ");
    for (const value of ["Fusion usage", "Usage records", "Models", "Workspaces", "Agent usage", "Phase usage",
        "≥ 2", "2 / 3 records complete", "2 / 2 records complete · 1 excluded",
        "<0.01", "not necessarily distinct API calls"]) {
        assert.ok(content.includes(value), `missing ${value}: ${content}`);
    }
    for (const value of ["Resolved patterns", "Resolved turns", "Permission requests", "ordinary-command", "resolution-only"]) {
        assert.equal(content.includes(value), false, value);
    }
    const model = panels.find((entry) => walk(entry).some((node) => node.tag === "h2" && node.children[0] === "Models"));
    const rows = walk(model).filter((node) => node.tag === "tr");
    const incompleteCells = rows[1].children.map(text);
    assert.deepEqual(incompleteCells.slice(-2), ["–", ""]);
    assert.ok(walk(model).some((node) => String(node.attributes.title ?? "").includes("records complete")));
});

test("Fusion-only model shares compare recorded AIU for the listed models", () => {
    const data = projectFusion(fusionDataset());
    const before = structuredClone(data);
    const { panels, walk } = renderedPanels(normalizeQuery({ fusionOnly: true }), data);
    const title = (node) => walk(node).find((child) => child.tag === "h2")?.children[0];
    assert.deepEqual(Array.from(panels.slice(0, 5), title),
        ["Usage share by model", "Copilot Fusion", "Models", "Usage share by workspace", "Workspaces"]);
    assert.equal(Array.from(panels, title).filter((name) => name === "Copilot Fusion").length, 1);
    const share = panels[0];
    const text = (node) => typeof node === "string" ? node : node instanceof Element ? node.children.map(text).join(" ") : "";
    assert.match(text(share), /recorded AIU · listed models/);
    assert.match(text(share), /Missing AIU is excluded, so incomplete coverage can affect proportions/);
    const segments = walk(share).find((node) => node.className === "bar").children;
    assert.deepEqual(segments.map((node) => node.attributes.title), [
        "model-a: ≥ 2 AIU (100%)",
        "model-b: <0.01 AIU (<0.1%)",
    ]);
    assert.match(text(share), /model-a 100%/);
    assert.match(text(share), /model-b <0\.1%/);
    assert.deepEqual(data, before);

    const limited = renderedPanels(normalizeQuery({ fusionOnly: true, top: 1 }), projectFusion(fusionDataset(1)));
    const limitedShare = limited.panels.find((panel) =>
        limited.walk(panel).some((node) => node.tag === "h2" && node.children[0] === "Usage share by model"));
    const limitedSegments = limited.walk(limitedShare).find((node) => node.className === "bar").children;
    assert.deepEqual(limitedSegments.map((node) => node.attributes.title), ["model-a: ≥ 2 AIU (100%)"]);
});

test("Fusion-only model shares handle unknown models, missing values, zeros, and empty results", () => {
    const data = projectFusion(fusionDataset());
    const [first] = data.FusionUsage.ByModel;
    const unknown = structuredClone(first);
    unknown.Key = "";
    unknown.Metrics.AIU.Total = 1;
    const missing = structuredClone(first);
    missing.Key = "model-missing";
    missing.Metrics.AIU = { Total: 0, ObservedRecords: 0, CompleteRecords: 0, ExcludedRecords: 0 };
    const partialZero = structuredClone(first);
    partialZero.Key = "model-zero";
    partialZero.Metrics.AIU = { Total: 0, ObservedRecords: 1, CompleteRecords: 0, ExcludedRecords: 0 };
    data.FusionUsage.ByModel = [first, unknown, missing, partialZero];
    data.FusionUsage.ModelGroups = 4;
    data.FusionUsage.UsageRecords = data.Fusion.UsageRecords = 8;
    data.FusionUsage.Metrics.AIU = { Total: 3, ObservedRecords: 3, CompleteRecords: 2, ExcludedRecords: 0 };
    data.Fusion.AIU = 3;
    data.Fusion.AIURecords = 3;
    const view = renderedPanels(normalizeQuery({ fusionOnly: true }), data);
    const share = view.panels.find((panel) =>
        view.walk(panel).some((node) => node.tag === "h2" && node.children[0] === "Usage share by model"));
    const segments = view.walk(share).find((node) => node.className === "bar").children;
    assert.deepEqual(segments.map((node) => node.attributes.title), [
        "model-a: ≥ 2 AIU (67%)",
        "Unknown model: ≥ 1 AIU (33%)",
    ]);
    for (const emptyData of [
        { ...data, FusionUsage: { ...data.FusionUsage, ByModel: [missing, partialZero] } },
        projectFusion(emptyFusionDataset()),
    ]) {
        const empty = renderedPanels(normalizeQuery({ fusionOnly: true }), emptyData);
        const panel = empty.panels.find((entry) =>
            empty.walk(entry).some((node) => node.tag === "h2" && node.children[0] === "Usage share by model"));
        assert.ok(panel);
        assert.equal(empty.walk(panel).some((node) => node.className === "bar"), false);
        assert.ok(empty.walk(panel).some((node) => node.children.includes("No positive recorded model AIU to compare.")));
    }
});

test("Fusion-only workspace shares match model share semantics and retain workspace scope controls", () => {
    const data = projectFusion(fusionDataset());
    const before = structuredClone(data);
    const query = normalizeQuery({ source: "session", fusionOnly: true, scope: "cwd", path: "/fictional/a" });
    const view = renderedPanels(query, data);
    const panel = view.panels.find((entry) =>
        view.walk(entry).some((node) => node.tag === "h2" && node.children[0] === "Usage share by workspace"));
    const bar = view.walk(panel).find((node) => node.className === "bar");
    assert.deepEqual(bar.children.map((node) => node.attributes.title), [
        "/fictional/a: ≥ 2 AIU (100%)",
        "/fictional/b: <0.01 AIU (<0.1%)",
    ]);
    const clear = view.walk(panel).find((node) => node.attributes["data-clear-filter"] === "workspace");
    clear.events.click();
    assert.deepEqual(JSON.parse(JSON.stringify(view.submitted)), [{ scope: "all", path: "" }]);
    assert.deepEqual(data, before);

    for (const emptyData of [
        projectFusion(emptyFusionDataset()),
        { ...data, FusionUsage: { ...data.FusionUsage, ByCWD: [] } },
    ]) {
        const empty = renderedPanels(normalizeQuery({ fusionOnly: true }), emptyData);
        const share = empty.panels.find((entry) =>
            empty.walk(entry).some((node) => node.tag === "h2" && node.children[0] === "Usage share by workspace"));
        assert.ok(share);
        assert.equal(empty.walk(share).some((node) => node.className === "bar"), false);
        assert.ok(empty.walk(share).some((node) => node.children.includes("No positive recorded workspace AIU to compare.")));
    }
});

test("Fusion-only cache metrics remain visible with coverage in cards and model rows", () => {
    const data = projectFusion(fusionDataset());
    data.FusionUsage.ByModel[1].Metrics.CacheWriteTokens = {
        Total: 0, ObservedRecords: 0, CompleteRecords: 0, ExcludedRecords: 0,
    };
    const app = interactiveFusion(data, { fusionOnly: true });
    const card = (label) => app.walk(app.content).find((node) => node.className === "card" &&
        app.walk(node).some((child) => child.className === "label" && child.children[0] === label));
    assert.match(app.text(card("Cache read")), /≥ 30/);
    assert.match(app.text(card("Cache write")), /≥ 10/);
    const headings = app.walk(app.modelTable()).find((node) => node.tag === "thead").children[0].children;
    const index = (name) => headings.findIndex((heading) =>
        app.walk(heading).some((node) => node.attributes["data-fusion-model-sort"] === name));
    const read = index("cacheRead");
    const write = index("cacheWrite");
    assert.ok(read >= 0 && write >= 0);
    const rows = app.modelRows();
    assert.equal(app.text(rows[0].children[read]), "≥ 30");
    assert.equal(app.text(rows[0].children[write]), "≥ 10");
    assert.match(rows[0].children[read].attributes.title, /1 \/ 2 records complete/);
    assert.equal(app.text(rows[1].children[read]), "0");
    assert.equal(app.text(rows[1].children[write]), "–");
});

test("Fusion token notation matches normal formatting without overstating partial values", () => {
    const { context } = renderedPanels(normalizeQuery({ fusionOnly: true }), projectFusion(fusionDataset()));
    const metric = (total, observed, complete, excluded = 0) => ({
        Total: total, ObservedRecords: observed, CompleteRecords: complete, ExcludedRecords: excluded,
    });
    const format = (value, partial) => context.fusionMetricText(
        metric(value, partial ? 1 : 2, partial ? 0 : 2), 2, context.tokenCount,
    );
    const cases = [
        [0, "0", "≥ 0"],
        [9_999, new Intl.NumberFormat().format(9_999), `≥ ${new Intl.NumberFormat().format(9_999)}`],
        [10_000, "10K", "≥ 10K"],
        [12_360, "12.4K", "≥ 12.3K"],
        [999_949, "999.9K", "≥ 999.9K"],
        [999_950, "1M", "≥ 999.9K"],
        [999_999, "1M", "≥ 999.9K"],
        [1_000_000, "1M", "≥ 1M"],
        [999_949_999, "999.9M", "≥ 999.9M"],
        [999_950_000, "1B", "≥ 999.9M"],
        [999_999_999, "1B", "≥ 999.9M"],
        [1_000_000_000, "1B", "≥ 1B"],
        [999_949_999_999, "999.9B", "≥ 999.9B"],
        [999_950_000_000, "1T", "≥ 999.9B"],
        [999_999_999_999, "1T", "≥ 999.9B"],
        [1_000_000_000_000, "1T", "≥ 1T"],
        [999_999_999_999_999, "1000T", "≥ 999.9T"],
        [Number.MAX_SAFE_INTEGER, "9007.2T", "≥ 9007.1T"],
    ];
    for (const [value, complete, partial] of cases) {
        assert.equal(format(value, false), complete, `complete ${value}`);
        assert.equal(format(value, false), context.big(value), `normal ${value}`);
        assert.equal(format(value, true), partial, `partial ${value}`);
    }
    assert.equal(context.fusionMetricText(metric(0, 0, 0), 2, context.tokenCount), "–");
    assert.equal(context.fusionMetricText(metric(0, 1, 1, 1), 2, context.fusionNumber), "0");

    const displayedValue = (text) => {
        const match = /^≥ ([\d,]+(?:\.\d+)?)([KMBT]?)$/.exec(text);
        assert.ok(match, text);
        const [whole, fraction = ""] = match[1].replaceAll(",", "").split(".");
        const unit = { "": 1, K: 1e3, M: 1e6, B: 1e9, T: 1e12 }[match[2]];
        return Number(whole) * unit + Number(fraction || 0) * unit / 10 ** fraction.length;
    };
    let seed = 0x417ecc;
    for (const scale of [10_000, 1e6, 1e9, 1e12, 1e15]) {
        const limit = Math.min(scale * 1_000, Number.MAX_SAFE_INTEGER);
        for (let i = 0; i < 300; i++) {
            seed = (Math.imul(seed, 1_664_525) + 1_013_904_223) >>> 0;
            const value = Math.floor(seed / 2 ** 32 * limit);
            if (value < 10_000) continue;
            const text = format(value, true);
            assert.ok(displayedValue(text) <= value, `${text} exceeds ${value}`);
        }
    }
});

test("partial Fusion AIU and API time labels never exceed their recorded subtotals", () => {
    const { context } = renderedPanels(normalizeQuery({ fusionOnly: true }), projectFusion(fusionDataset()));
    const partial = (total) => ({ Total: total, ObservedRecords: 1, CompleteRecords: 1, ExcludedRecords: 0 });
    const number = (text) => Number(text.replaceAll(",", ""));
    const samples = [
        0, 0.000000000123456, 0.0000000015, 1.2345678906, 9.9999999999,
        999.99999, 1199.999, 59_999.9, 60_001, 3_599_999, 3_600_001, 12_345_678.99,
    ];
    let seed = 0x4127ab;
    for (const scale of [1e-13, 1e-9, 1, 1000, 60_000, 3_600_000, 1e12]) {
        for (let i = 0; i < 50; i++) {
            seed = (Math.imul(seed, 1_664_525) + 1_013_904_223) >>> 0;
            samples.push(seed / 2 ** 32 * scale);
        }
    }
    for (const value of samples) {
        const aiu = context.fusionMetricText(partial(value), 2, context.fusionNumber);
        assert.ok(aiu.startsWith("≥ "));
        const formattedAiu = aiu.slice(2);
        assert.ok(!/[eE]/.test(formattedAiu) && (formattedAiu.split(".")[1]?.length ?? 0) <= 2, aiu);
        assert.ok(number(formattedAiu) <= value, `${aiu} exceeds ${value}`);
        const legacyAiu = context.fusionAIU({ AIU: value, AIURecords: 1, UsageRecords: 2 });
        const formattedLegacy = legacyAiu.slice(2);
        assert.ok(!/[eE]/.test(formattedLegacy) && (formattedLegacy.split(".")[1]?.length ?? 0) <= 2, legacyAiu);
        assert.ok(number(formattedLegacy) <= value, `${legacyAiu} exceeds ${value}`);
        const duration = context.fusionMetricText(partial(value), 2, context.fusionDuration);
        const match = /^≥ (.+) (ms|s|min|h)$/.exec(duration);
        assert.ok(match, duration);
        const scale = { ms: 1, s: 1000, min: 60_000, h: 3_600_000 }[match[2]];
        assert.ok(number(match[1]) * scale <= value, `${duration} exceeds ${value} ms`);
    }
});

test("Fusion token cards and all usage tables show compact values with exact counts and coverage", () => {
    const metric = (total, observed, complete) => ({
        Total: total, ObservedRecords: observed, CompleteRecords: complete, ExcludedRecords: 0,
    });
    const tokenMetrics = (records) => ({
        PromptTokens: metric(12_360, records, 0),
        UncachedInputTokens: metric(12_360, records, records),
        CacheReadTokens: metric(0, 1, 0),
        CacheWriteTokens: metric(0, 0, 0),
        OutputTokens: metric(999_950, records, records),
    });
    const otherTokens = {
        PromptTokens: metric(0, 1, 0),
        UncachedInputTokens: metric(0, 1, 1),
        CacheReadTokens: metric(0, 0, 0),
        CacheWriteTokens: metric(0, 0, 0),
        OutputTokens: metric(0, 1, 1),
    };
    const source = fusionDataset();
    const usage = source.Fusion.Usage;
    Object.assign(usage.Metrics, tokenMetrics(3));
    for (const entries of [usage.ByModel, usage.ByCWD, usage.ByRole]) {
        Object.assign(entries[0].Metrics, tokenMetrics(2));
        Object.assign(entries[1].Metrics, otherTokens);
    }
    const data = projectFusion(source);
    const before = structuredClone(data);
    const app = interactiveFusion(data, { fusionOnly: true });
    const cases = [
        ["Prompt tokens", "prompt", "PromptTokens", "≥ 12.3K"],
        ["Uncached input", "uncached", "UncachedInputTokens", "12.4K"],
        ["Cache read", "cacheRead", "CacheReadTokens", "≥ 0"],
        ["Cache write", "cacheWrite", "CacheWriteTokens", "–"],
        ["Output tokens", "output", "OutputTokens", "1M"],
    ];
    const tooltip = (value, records) => {
        const coverage = `${value.CompleteRecords} / ${records} records complete` +
            (value.ObservedRecords !== value.CompleteRecords
                ? ` · ${value.ObservedRecords} have partial or complete values` : "");
        return value.ObservedRecords
            ? `Recorded: ${new Intl.NumberFormat().format(value.Total)} tokens · ${coverage}` : coverage;
    };
    for (const [label, , field, expected] of cases) {
        const card = app.walk(app.content).find((node) => node.className === "card" &&
            app.walk(node).some((child) => child.className === "label" && child.children[0] === label));
        const value = app.walk(card).find((node) => node.className === "value");
        assert.equal(app.text(value), expected, label);
        assert.equal(card.attributes.title, tooltip(data.FusionUsage.Metrics[field], 3), label);
        assert.equal(value.attributes["aria-label"], `${expected}; ${card.attributes.title}`, label);
    }
    for (const [table, attribute, entries] of [
        [app.modelTable(), "data-fusion-model-sort", data.FusionUsage.ByModel],
        [app.workspaceTable(), "data-fusion-workspace-sort", data.FusionUsage.ByCWD],
        [app.agentTable(), "data-agent-sort", data.FusionUsage.ByRole],
    ]) {
        const headings = app.walk(table).find((node) => node.tag === "thead").children[0].children;
        const row = app.walk(table).find((node) => node.tag === "tbody").children[0];
        for (const [, id, field, expected] of cases) {
            const index = headings.findIndex((heading) =>
                app.walk(heading).some((node) => node.attributes[attribute] === id));
            assert.ok(index >= 0, id);
            assert.equal(app.text(row.children[index]), expected, id);
            assert.equal(row.children[index].attributes.title, tooltip(entries[0].Metrics[field], 2), id);
            assert.equal(row.children[index].attributes["aria-label"], `${expected}; ${row.children[index].attributes.title}`, id);
        }
    }
    assert.deepEqual(app.submitted, []);
    assert.deepEqual(data, before);
});

test("normal and complete Fusion summary cards use the same token notation", () => {
    const normal = {
        UsageSessions: 1, UsageInputTokens: 12_360, UsageCacheReadTokens: 0,
        UsageCacheWriteTokens: 0, UsageOutputTokens: 999_950,
    };
    const fusion = projectFusion(fusionDataset());
    for (const [field, total] of [["PromptTokens", 12_360], ["OutputTokens", 999_950]]) {
        fusion.FusionUsage.Metrics[field] = {
            Total: total, ObservedRecords: 3, CompleteRecords: 3, ExcludedRecords: 0,
        };
    }
    const value = (view, label) => {
        const nodes = view.nodes.flatMap(view.walk);
        const card = nodes.find((node) => node.className === "card" &&
            view.walk(node).some((child) => child.className === "label" && child.children[0] === label));
        return view.walk(card).find((node) => node.className === "value").children[0];
    };
    const ordinary = renderedPanels(normalizeQuery({ source: "session" }), normal);
    const filtered = renderedPanels(normalizeQuery({ source: "session", fusionOnly: true }), fusion);
    for (const label of ["Prompt tokens", "Output tokens"]) {
        assert.equal(value(filtered, label), value(ordinary, label), label);
    }
});

test("VS Code summary aligns shared cards and retains activity metrics", () => {
    const data = {
        Sessions: 2, Turns: 4, LLMRequests: 6, ToolCalls: 4,
        ByModel: [{ Key: "model-a", UsageAIU: 1.234, InputTokens: 300, CachedTokens: 100, OutputTokens: 50 }],
        ByTool: [{ Key: "tool-a", Total: 4, OK: 3, Error: 1 }],
    };
    const before = structuredClone(data);
    const view = renderedPanels(normalizeQuery({ source: "vscode" }), data);
    assert.deepEqual(summaryCards(view).map(({ label, value, sub }) => [label, value, sub]), [
        ["Sessions", "2", ""],
        ["Usage (AIU)", "1.23", ""],
        ["LLM requests", "6", "1.5 / turn"],
        ["Input tokens", "300", "33% cached"],
        ["Output tokens", "50", ""],
        ["Turns", "4", "2.0 / session"],
        ["Tool calls", "4", "25% errors (listed tools)"],
    ]);
    assert.deepEqual(data, before);
});

test("usage cards use equal widths and keep titles on one line", () => {
    const css = readFileSync(new URL("../ui/styles.css", import.meta.url), "utf8");
    assert.match(css, /\.usage-cards,\s*\.fusion-cards\s*\{[^}]*grid-template-columns:\s*repeat\(auto-fill,\s*minmax\(160px,\s*1fr\)\)\s*;[^}]*overflow-x:\s*auto\s*;/);
    assert.match(css, /\.usage-cards \.card,\s*\.fusion-cards \.card\s*\{[^}]*min-width:\s*0\s*;[^}]*overflow-wrap:\s*anywhere\s*;/);
    assert.match(css, /\.usage-cards \.label,\s*\.fusion-cards \.label\s*\{[^}]*white-space:\s*nowrap\s*;/);
});

test("card titles bottom-align while values stay aligned", () => {
    const css = readFileSync(new URL("../ui/styles.css", import.meta.url), "utf8");
    assert.match(css, /@supports \(grid-template-rows:\s*subgrid\)\s*\{[^}]*\.card\s*\{[^}]*grid-row:\s*span 3\s*;[^}]*grid-template-rows:\s*subgrid\s*;/);
    assert.match(css, /\.card \.label\s*\{[^}]*display:\s*flex\s*;[^}]*align-items:\s*flex-end\s*;/);
    assert.match(css, /\.card \.label\s*\{[^}]*grid-row:\s*1\s*;/);
    assert.match(css, /\.card \.value\s*\{[^}]*grid-row:\s*2\s*;/);
    assert.match(css, /\.card \.sub\s*\{[^}]*grid-row:\s*3\s*;/);
    assert.match(css, /@supports not \(grid-template-rows:\s*subgrid\)\s*\{[^}]*\.card \.label\s*\{[^}]*min-height:\s*2lh\s*;/);
});

test("normal and Fusion usage cards share order, notation, and supported derived values", () => {
    const normal = {
        UsageSessions: 2, UsageIncompleteSessions: 0, UsageAIU: 2.000000001, UsageRequests: 3,
        UsagePremiumRequests: 0.5, UsageInputTokens: 60, UsageCacheReadTokens: 30,
        UsageCacheWriteTokens: 10, UsageOutputTokens: 50, UsageAPIDurationMs: 0.00000001,
    };
    const fusion = completeFusionUsage();
    const beforeNormal = structuredClone(normal);
    const beforeFusion = structuredClone(fusion);
    const ordinary = summaryCards(renderedPanels(normalizeQuery({ source: "session" }), normal));
    const filtered = summaryCards(renderedPanels(normalizeQuery({ source: "session", fusionOnly: true }), fusion))
        .slice(0, ordinary.length);
    assert.deepEqual(ordinary.map((card) => card.label), [
        "Sessions with usage", "Usage (AIU)", "Recorded API requests", "Premium requests", "Prompt tokens",
        "Cache read", "Cache write", "Uncached input", "Output tokens", "API time",
    ]);
    assert.deepEqual(filtered.map((card) => card.label), [
        "Sessions with usage", "Usage (AIU)", "Usage records", "Premium requests", "Prompt tokens",
        "Cache read", "Cache write", "Uncached input", "Output tokens", "API time",
    ]);
    assert.deepEqual(filtered.map((card) => card.value), ordinary.map((card) => card.value));
    for (const label of ["Usage (AIU)", "Premium requests", "Prompt tokens", "Output tokens", "API time"]) {
        const original = ordinary.find((card) => card.label === label);
        const projected = filtered.find((card) => card.label === label);
        assert.ok(projected.sub.startsWith(original.sub), `${label}: ${projected.sub} vs ${original.sub}`);
    }
    assert.match(filtered.find((card) => card.label === "Premium requests").sub,
        /4 AIU \/ premium request · 2 \/ 2 records complete · 1 excluded/);
    assert.match(filtered.find((card) => card.label === "Prompt tokens").sub, /^30% cache hit/);
    assert.equal(ordinary.find((card) => card.label === "API time").value, "0.00000001 ms");
    assert.deepEqual(normal, beforeNormal);
    assert.deepEqual(fusion, beforeFusion);
});

test("Fusion summary ratios reject incomplete, excluded-only, zero-denominator and inconsistent inputs", () => {
    const cases = [
        ["AIU", (metrics) => Object.assign(metrics.AIU, { ObservedRecords: 2, CompleteRecords: 2 }),
            [["Usage (AIU)", "– / session"], ["Premium requests", "– AIU / premium request"]]],
        ["partial premium", (metrics) => { metrics.PremiumRequests.CompleteRecords = 1; },
            [["Premium requests", "– AIU / premium request"]]],
        ["excluded premium", (metrics) => Object.assign(metrics.PremiumRequests,
            { Total: 0, ObservedRecords: 0, CompleteRecords: 0, ExcludedRecords: 3 }),
            [["Premium requests", "– AIU / premium request"]]],
        ["zero premium", (metrics) => { metrics.PremiumRequests.Total = 0; },
            [["Premium requests", "– AIU / premium request"]]],
        ["inconsistent cache", (metrics) => { metrics.CacheReadTokens.Total = 101; },
            [["Prompt tokens", "– cache hit"]]],
        ["partial cache", (metrics) => { metrics.CacheReadTokens.CompleteRecords = 2; },
            [["Prompt tokens", "– cache hit"]]],
        ["partial output", (metrics) => { metrics.OutputTokens.CompleteRecords = 2; },
            [["Output tokens", "– / session"]]],
        ["partial duration", (metrics) => { metrics.APIDurationMs.CompleteRecords = 2; },
            [["API time", "– / session"]]],
    ];
    for (const [name, change, expected] of cases) {
        const data = completeFusionUsage();
        change(data.FusionUsage.Metrics);
        const cards = summaryCards(renderedPanels(normalizeQuery({ fusionOnly: true }), data));
        for (const [label, sub] of expected) {
            assert.ok(cards.find((card) => card.label === label).sub.startsWith(sub), `${name}: ${label}`);
        }
        if (name === "excluded premium") assert.equal(cards.find((card) => card.label === "Premium requests").value, "–");
        if (name === "zero premium") assert.equal(cards.find((card) => card.label === "Premium requests").value, "0");
    }
    const zero = completeFusionUsage();
    zero.FusionUsage.Metrics.AIU.Total = 0;
    const cards = summaryCards(renderedPanels(normalizeQuery({ fusionOnly: true }), zero));
    assert.match(cards.find((card) => card.label === "Premium requests").sub, /^0 AIU \/ premium request/);
});

test("normal usage cards distinguish unavailable values, recorded zeros and partial prompt totals", () => {
    const data = {
        UsageSessions: 1, UsageAIU: 0, UsagePremiumRequests: 0,
        UsageCacheReadTokens: 12_360, UsageOutputTokens: 0,
    };
    const before = structuredClone(data);
    const view = renderedPanels(normalizeQuery({ source: "session" }), data);
    const cards = summaryCards(view);
    const find = (label) => cards.find((card) => card.label === label);
    assert.equal(find("Usage (AIU)").value, "0");
    assert.equal(find("Recorded API requests").value, "–");
    assert.equal(find("Premium requests").sub, "– AIU / premium request");
    assert.equal(find("Prompt tokens").value, "≥ 12.3K");
    assert.equal(find("Prompt tokens").sub, "– cache hit");
    assert.match(find("Prompt tokens").title, /Recorded subtotal: .*12,360 tokens/);
    assert.equal(find("Cache read").value, "12.4K");
    assert.equal(find("Cache write").value, "–");
    assert.equal(find("Uncached input").value, "–");
    assert.equal(find("Output tokens").value, "0");
    assert.equal(find("Output tokens").sub, "0 / session");
    assert.equal(find("API time").value, "–");
    const missing = summaryCards(renderedPanels(normalizeQuery({ source: "session" }), { UsageSessions: 0 }));
    assert.equal(missing.find((card) => card.label === "Prompt tokens").value, "–");
    assert.equal(missing.find((card) => card.label === "Prompt tokens").title, "Token data is unavailable");
    assert.deepEqual(data, before);
});

test("CLI, Fusion, and VS Code breakdowns share two-decimal AIU values", () => {
    const raw = {
        Key: "model-a", Sessions: 2, Requests: 2, PremiumRequests: 0.00000001, AIU: 0.000000001,
        InputTokens: 60, CacheReadTokens: 30, CacheWriteTokens: 10, OutputTokens: 50, APIDurationMs: 0.00000001,
    };
    const normal = {
        UsageSessions: 2, UsageIncompleteSessions: 0, ByModelUsage: [raw],
        ByCWDUsage: [{ ...raw, Key: "/fictional/project" }], ByRoleUsage: [{ ...raw, Key: "main" }],
    };
    const fusion = completeFusionUsage();
    const values = {
        AIU: raw.AIU, PremiumRequests: raw.PremiumRequests, PromptTokens: 100, CacheReadTokens: raw.CacheReadTokens,
        CacheWriteTokens: raw.CacheWriteTokens, UncachedInputTokens: raw.InputTokens, OutputTokens: raw.OutputTokens,
        APIDurationMs: raw.APIDurationMs,
    };
    fusion.FusionUsage.ByModel = [fusionAgentEntry(raw.Key, 2, 2, values)];
    fusion.FusionUsage.ByCWD = [fusionAgentEntry("/fictional/project", 2, 2, values)];
    fusion.FusionUsage.ByRole = [fusionAgentEntry("main", 2, 2, values)];
    const beforeNormal = structuredClone(normal);
    const beforeFusion = structuredClone(fusion);
    const storage = {
        getItem: (key) => key === "attendant-dashboard.fusionWorkspaceView"
            ? JSON.stringify({ unit: "session", sort: "aiu", direction: "desc" }) : null,
        setItem() {},
    };
    const ordinary = renderedPanels(normalizeQuery({ source: "session" }), normal, { storage });
    const filtered = renderedPanels(normalizeQuery({ source: "session", fusionOnly: true }), fusion, { storage });
    const text = (node) => typeof node === "string" ? node : node instanceof Element ? node.children.map(text).join("") : "";
    const normalizeLabels = (labels, extraApiTime) => labels
        .filter((label) => label !== undefined && !(extraApiTime && label === "API time"))
        .map((label) => ({ "API requests": "Usage count", "Usage records": "Usage count",
            "AIU / request": "AIU / count", "AIU / record": "AIU / count" })[label] ?? label);
    for (const title of ["Models", "Workspaces", "Agent usage"]) {
        const normalTable = panelTable(ordinary, title);
        const fusionTable = panelTable(filtered, title);
        assert.deepEqual(normalizeLabels(tableHeadings(ordinary, normalTable), false),
            normalizeLabels(tableHeadings(filtered, fusionTable), title !== "Workspaces"), title);
        for (const label of ["Premium req.", "Prompt", "Cache read", "Cache write", "Uncached", "Output", "AIU"]) {
            assert.equal(text(tableCell(ordinary, normalTable, label)), text(tableCell(filtered, fusionTable, label)), `${title} ${label}`);
        }
        for (const label of ["Prompt", "Cache read", "Cache write", "Uncached", "Output"]) {
            const ordinaryCell = tableCell(ordinary, normalTable, label);
            const filteredCell = tableCell(filtered, fusionTable, label);
            assert.ok(filteredCell.attributes.title.startsWith(ordinaryCell.attributes.title), `${title} ${label} tooltip`);
            assert.match(ordinaryCell.attributes["aria-label"], /Recorded: .* tokens$/);
        }
        if (title !== "Agent usage") {
            assert.equal(text(tableCell(ordinary, normalTable, "Cache hit")), text(tableCell(filtered, fusionTable, "Cache hit")), title);
            assert.equal(tableCell(ordinary, normalTable, "Tokens").attributes["aria-label"],
                tableCell(filtered, fusionTable, "Tokens").attributes["aria-label"], `${title} token bar`);
        }
        if (title === "Workspaces") {
            assert.equal(text(tableCell(ordinary, normalTable, "API time")), text(tableCell(filtered, fusionTable, "API time")));
            assert.equal(text(tableCell(ordinary, normalTable, "AIU / session")), text(tableCell(filtered, fusionTable, "AIU / session")));
        }
        if (title === "Agent usage") {
            assert.equal(text(tableCell(ordinary, normalTable, "AIU / session")), text(tableCell(filtered, fusionTable, "AIU / session")));
            assert.equal(text(tableCell(ordinary, normalTable, "AIU / 1M tokens")), text(tableCell(filtered, fusionTable, "AIU / 1M tokens")));
        }
        assert.equal(text(tableCell(ordinary, normalTable, "AIU")), "<0.01", title);
        assert.equal(text(tableCell(ordinary, normalTable, "Premium req.")), "<0.01", title);
    }

    const vscode = renderedPanels(normalizeQuery({ source: "vscode" }), {
        ByModel: [{ Key: "model-a", Requests: 2, InputTokens: 100, OutputTokens: 50, UsageAIU: 0.04 }],
    });
    const vscodeTable = panelTable(vscode, "Models");
    assert.equal(text(tableCell(vscode, vscodeTable, "AIU")), "0.04");
    assert.equal(text(tableCell(vscode, vscodeTable, "AIU / request")), "0.02");
    const vscodeCard = vscode.nodes.flatMap(vscode.walk).find((node) => node.className === "card" &&
        vscode.walk(node).some((child) => child.className === "label" && child.children[0] === "Usage (AIU)"));
    assert.equal(vscode.walk(vscodeCard).find((node) => node.className === "value").children[0], "0.04");
    assert.deepEqual(normal, beforeNormal);
    assert.deepEqual(fusion, beforeFusion);
});

test("normal Agent costs validate raw inputs, recorded zeros, and incomplete or legacy breakdowns", () => {
    const complete = {
        Key: "complete", Sessions: 2, Requests: 2, PremiumRequests: 0.5, AIU: 2,
        InputTokens: 60, CacheReadTokens: 30, CacheWriteTokens: 10, OutputTokens: 50,
    };
    const missingAIU = { ...complete, Key: "missing-aiu" };
    const missingRequest = { ...complete, Key: "missing-request" };
    const missingToken = { ...complete, Key: "missing-token" };
    delete missingAIU.AIU;
    delete missingRequest.Requests;
    delete missingToken.CacheWriteTokens;
    const data = {
        UsageSessions: 2, UsageIncompleteSessions: 0,
        ByRoleUsage: [
            complete, missingAIU, missingRequest, missingToken,
            { ...complete, Key: "zero", AIU: 0 },
            { ...complete, Key: "zero-denominators", Sessions: 0, Requests: 0 },
            { ...complete, Key: "unsafe-token", InputTokens: Number.MAX_SAFE_INTEGER + 1 },
        ],
    };
    const before = structuredClone(data);
    const app = interactiveFusion(data);
    const table = app.agentTable();
    const headings = app.walk(table).find((node) => node.tag === "thead").children[0].children;
    const index = (id) => headings.findIndex((heading) =>
        app.walk(heading).some((node) => node.attributes["data-agent-sort"] === id));
    const row = (key) => app.walk(table).find((node) => node.tag === "tbody").children
        .find((entry) => app.text(entry.children[0]) === key);
    const value = (key, id) => app.text(row(key).children[index(id)]);
    assert.equal(value("complete", "sessions"), "2");
    assert.equal(value("complete", "uncached"), "60");
    assert.equal(value("complete", "sessionCost"), "1");
    assert.equal(value("complete", "requestCost"), "1");
    assert.equal(value("complete", "tokenCost"), "13,333.33");
    for (const id of ["aiu", "sessionCost", "requestCost", "tokenCost"]) assert.equal(value("missing-aiu", id), "–", id);
    assert.equal(value("missing-request", "requests"), "–");
    assert.equal(value("missing-request", "requestCost"), "–");
    assert.equal(value("missing-request", "sessionCost"), "1");
    assert.equal(value("missing-token", "prompt"), "–");
    assert.equal(value("missing-token", "cacheWrite"), "–");
    assert.equal(value("missing-token", "tokenCost"), "–");
    assert.equal(value("missing-token", "requestCost"), "1");
    for (const id of ["sessionCost", "requestCost", "tokenCost"]) assert.equal(value("zero", id), "0", id);
    assert.equal(value("zero-denominators", "sessionCost"), "–");
    assert.equal(value("zero-denominators", "requestCost"), "–");
    assert.equal(value("unsafe-token", "tokenCost"), "–");
    assert.match(row("complete").children[index("prompt")].attributes.title, /Recorded: 100 tokens/);
    assert.equal(row("complete").children[index("prompt")].attributes["aria-label"], "100; Recorded: 100 tokens");
    for (const incomplete of [1, undefined]) {
        const view = interactiveFusion({ ...data, UsageIncompleteSessions: incomplete });
        const heading = view.agentHeader("sessionCost");
        const cells = view.walk(view.agentTable()).find((node) => node.tag === "tbody").children;
        const cellIndex = view.walk(view.agentTable()).find((node) => node.tag === "thead").children[0].children.indexOf(heading);
        assert.ok(cells.every((entry) => view.text(entry.children[cellIndex]) === "–"));
    }
    assert.deepEqual(data, before);
});

test("normal token tooltips do not claim exact counts for missing or unsafe inputs", () => {
    const raw = {
        Key: "model-a", Sessions: 1, Requests: 1, AIU: 1,
        InputTokens: 10, CacheReadTokens: 0, OutputTokens: 5,
    };
    const data = {
        UsageSessions: 1, UsageCacheReadTokens: Number.MAX_SAFE_INTEGER + 1,
        ByModelUsage: [raw], ByCWDUsage: [{ ...raw, Key: "/fictional/project" }],
    };
    const before = structuredClone(data);
    const view = renderedPanels(normalizeQuery({ source: "session" }), data);
    for (const title of ["Models", "Workspaces"]) {
        const table = panelTable(view, title);
        for (const column of ["Tokens", "Prompt", "Cache write"]) {
            const cell = tableCell(view, table, column);
            assert.equal(cell.attributes.title, undefined, `${title} ${column}`);
            assert.equal(cell.attributes["aria-label"], undefined, `${title} ${column}`);
        }
        assert.equal(tableCell(view, table, "Cache read").attributes.title, "Recorded: 0 tokens");
        assert.equal(tableCell(view, table, "Uncached").attributes.title, "Recorded: 10 tokens");
        assert.equal(tableCell(view, table, "Output").attributes.title, "Recorded: 5 tokens");
    }
    assert.equal(summaryCards(view).find((card) => card.label === "Cache read").title,
        "Exact token count exceeds numeric precision");
    assert.deepEqual(data, before);
});

test("token tables sort raw Fusion values even when compact labels match", () => {
    const data = projectFusion(fusionDataset());
    for (const entries of [data.FusionUsage.ByModel, data.FusionUsage.ByCWD, data.FusionUsage.ByRole]) {
        for (const [index, total] of [12_360, 12_399].entries()) {
            entries[index].Metrics.PromptTokens = {
                Total: total, ObservedRecords: entries[index].UsageRecords,
                CompleteRecords: entries[index].UsageRecords, ExcludedRecords: 0,
            };
        }
    }
    const before = structuredClone(data);
    const app = interactiveFusion(data, { fusionOnly: true });
    for (const [table, attribute] of [
        [app.modelTable(), "data-fusion-model-sort"],
        [app.workspaceTable(), "data-fusion-workspace-sort"],
        [app.agentTable(), "data-agent-sort"],
    ]) {
        const headings = app.walk(table).find((node) => node.tag === "thead").children[0].children;
        const index = headings.findIndex((heading) =>
            app.walk(heading).some((node) => node.attributes[attribute] === "prompt"));
        const rows = app.walk(table).find((node) => node.tag === "tbody").children;
        assert.deepEqual(rows.map((row) => app.text(row.children[index])), ["12.4K", "12.4K"]);
    }
    app.modelButton("prompt").events.click();
    app.workspaceButton("prompt").events.click();
    app.agentButton("prompt").events.click();
    assert.deepEqual(app.modelNames(), ["model-b", "model-a"]);
    assert.deepEqual(app.workspaceNames(), ["/fictional/b", "/fictional/a"]);
    assert.deepEqual(app.agentRows(), ["Subagents", "Main agent"]);
    app.modelButton("prompt").events.click();
    app.workspaceButton("prompt").events.click();
    app.agentButton("prompt").events.click();
    assert.deepEqual(app.modelNames(), ["model-a", "model-b"]);
    assert.deepEqual(app.workspaceNames(), ["/fictional/a", "/fictional/b"]);
    assert.deepEqual(app.agentRows(), ["Main agent", "Subagents"]);
    assert.deepEqual(app.submitted, []);
    assert.deepEqual(data, before);
});

test("normal Agent usage also rounds partial token subtotals down", () => {
    const data = normalAgentDataset();
    Object.assign(data.ByRoleUsage[2], {
        InputTokens: 12_360, CacheReadTokens: 0, CacheWriteTokens: 0, OutputTokens: 999_950,
    });
    const app = interactiveFusion(data);
    const table = app.agentTable();
    const headings = app.walk(table).find((node) => node.tag === "thead").children[0].children;
    const row = app.walk(table).find((node) => node.tag === "tbody").children
        .find((entry) => app.text(entry.children[0]) === "Unknown");
    for (const [id, expected] of [["prompt", "≥ 12.3K"], ["output", "≥ 999.9K"]]) {
        const index = headings.indexOf(app.agentHeader(id));
        assert.ok(index >= 0, id);
        assert.equal(app.text(row.children[index]), expected, id);
    }
    assert.deepEqual(app.submitted, []);
});

test("normal and Fusion-only unit controls use the right-aligned panel layout", () => {
    const normal = {
        UsageSessions: 1,
        ByModelUsage: [{ Key: "model-a", Sessions: 1, Requests: 1, AIU: 1 }],
        ByCWDUsage: [{ Key: "/fictional/workspace", Sessions: 1, Requests: 1, AIU: 1 }],
    };
    for (const [fusionOnly, data] of [[false, normal], [true, projectFusion(fusionDataset())]]) {
        const { panels, walk } = renderedPanels(normalizeQuery({ fusionOnly }), data);
        for (const title of ["Models", "Workspaces"]) {
            const panel = panels.find((entry) => walk(entry).some((node) => node.tag === "h2" && node.children[0] === title));
            const controls = walk(panel).find((node) => node.className === "panel-controls");
            assert.ok(controls, `${title}: ${fusionOnly}`);
            assert.ok(walk(controls).some((node) => node.className === "controls"));
            assert.ok(walk(controls).some((node) => node.className === "segmented"));
        }
    }
    const css = readFileSync(new URL("../ui/styles.css", import.meta.url), "utf8");
    assert.match(css, /\.panel-controls \.controls\s*\{[^}]*margin-left:\s*auto\s*;/);
});

test("table headings retain full labels and accessible sort controls with two wrap groups", () => {
    const app = interactiveFusion(projectFusion(fusionDataset()), { fusionOnly: true });
    for (const [label, parts] of [
        ["AIU", []],
        ["Cache hit", ["Cache", "hit"]],
        ["AIU / turn", ["AIU /", "turn"]],
        ["AIU / 1M tokens", ["AIU /", "1M tokens"]],
        ["Missing phase identity", ["Missing phase", "identity"]],
        ["Other decision timing", ["Other decision", "timing"]],
    ]) {
        const heading = app.context.tableHeading(label);
        assert.equal(app.text(heading), label);
        assert.deepEqual(app.walk(heading).filter((node) => node.className === "table-heading-part").map(app.text), parts);
    }

    const button = app.modelButton("unit");
    const label = app.text(button.children[0]);
    assert.equal(label, "AIU / record");
    assert.match(button.attributes["aria-label"], /^AIU \/ record: sort (ascending|descending)$/);
    assert.equal(button.children[1].attributes["aria-hidden"], "true");
    assert.equal(app.modelHeader("unit").attributes.scope, "col");

    const toolTable = app.context.toolTable([{ Key: "tool-a", Total: 1, OK: 1, Error: 0 }]);
    const plainHeading = app.walk(toolTable).find((node) => node.tag === "th" && app.text(node) === "Total time");
    assert.deepEqual(app.walk(plainHeading).filter((node) => node.className === "table-heading-part").map(app.text), ["Total", "time"]);

    const css = readFileSync(new URL("../ui/styles.css", import.meta.url), "utf8");
    assert.match(css, /thead th\s*\{[^}]*white-space:\s*normal\s*;/);
    assert.match(css, /\.table-heading-part\s*\{[^}]*white-space:\s*nowrap\s*;/);
    assert.match(css, /\.sort-header\s*\{[^}]*white-space:\s*normal\s*;/);
    assert.match(css, /\.sort-indicator\s*\{[^}]*flex-shrink:\s*0\s*;/);
    assert.match(css, /th,\s*td\s*\{[^}]*white-space:\s*nowrap\s*;/);
    assert.match(css, /\.panel\s*\{[^}]*overflow-x:\s*auto\s*;/);
});

test("Fusion Models and Workspaces render only complete, consistent token bars and valid cache hits", () => {
    const values = {
        AIU: 2, PremiumRequests: 0.5, PromptTokens: 100, CacheReadTokens: 30,
        CacheWriteTokens: 10, UncachedInputTokens: 60, OutputTokens: 50, APIDurationMs: 100,
    };
    const good = fusionAgentEntry("good", 1, 2, values);
    const partial = structuredClone(good);
    partial.Key = "partial";
    partial.Metrics.CacheWriteTokens.CompleteRecords = 1;
    const inconsistent = structuredClone(good);
    inconsistent.Key = "inconsistent";
    inconsistent.Metrics.PromptTokens.Total = 101;
    const overCache = structuredClone(good);
    overCache.Key = "over-cache";
    overCache.Metrics.CacheReadTokens.Total = 101;
    const zero = structuredClone(good);
    zero.Key = "zero";
    for (const field of ["PromptTokens", "CacheReadTokens", "CacheWriteTokens", "UncachedInputTokens", "OutputTokens"]) {
        zero.Metrics[field].Total = 0;
    }
    const missing = structuredClone(good);
    missing.Key = "missing";
    missing.Metrics.CacheReadTokens = { Total: 0, ObservedRecords: 0, CompleteRecords: 0, ExcludedRecords: 0 };
    const entries = [good, partial, inconsistent, overCache, zero, missing];
    const data = projectFusion(fusionDataset());
    data.FusionUsage.ByModel = structuredClone(entries);
    data.FusionUsage.ByCWD = entries.map((entry) => ({ ...structuredClone(entry), Key: `/fictional/${entry.Key}` }));
    data.FusionUsage.ModelGroups = data.FusionUsage.CWDGroups = entries.length;
    const before = structuredClone(data);
    const saved = new Map();
    const storage = { getItem: (key) => saved.get(key) ?? null, setItem: (key, value) => saved.set(key, value) };
    const app = interactiveFusion(data, { fusionOnly: true, storage });
    for (const [kind, table, button, header, rows, prefix] of [
        ["model", app.modelTable, app.modelButton, app.modelHeader, app.modelRows, ""],
        ["workspace", app.workspaceTable, app.workspaceButton, app.workspaceHeader, app.workspaceRows, "/fictional/"],
    ]) {
        const current = table();
        const headings = app.walk(current).find((node) => node.tag === "thead").children[0].children;
        const tokenIndex = headings.indexOf(header("tokens"));
        const hitIndex = headings.indexOf(header("cacheHit"));
        const row = (key) => rows().find((entry) => app.text(entry.children[0]) === `${prefix}${key}`);
        const barCell = row("good").children[tokenIndex];
        const bar = app.walk(barCell).find((node) => node.className === "bar");
        assert.deepEqual(bar.children.map((segment) => segment.attributes.title), [
            "Cache read: 30", "Cache write: 10", "Uncached input: 60", "Output: 50",
        ], kind);
        assert.equal(barCell.attributes["aria-label"], "Cache read: 30; Cache write: 10; Uncached input: 60; Output: 50");
        assert.equal(app.text(row("good").children[hitIndex]), "30%");
        assert.equal(app.text(row("partial").children[hitIndex]), "30%");
        assert.equal(app.text(row("zero").children[hitIndex]), "–");
        assert.equal(app.text(row("over-cache").children[hitIndex]), "–");
        assert.equal(app.text(row("missing").children[hitIndex]), "–");
        assert.equal(app.walk(row("zero").children[tokenIndex]).find((node) => node.className === "bar").children.length, 0);
        for (const key of ["partial", "inconsistent", "over-cache", "missing"]) {
            assert.equal(app.text(row(key).children[tokenIndex]), "–", `${kind} ${key}`);
            assert.match(row(key).children[tokenIndex].attributes.title, /complete and consistent/);
        }
        const panel = current.closest(".panel");
        for (const label of ["Cache read", "Cache write", "Uncached input", "Output"]) {
            assert.ok(app.walk(panel).some((node) => node.children.includes(label)), `${kind} legend ${label}`);
        }
        panel.scrollLeft = 85;
        app.context.document.activeElement = button("tokens");
        button("tokens").events.click();
        assert.equal(header("tokens").attributes["aria-sort"], "descending");
        assert.equal(app.context.document.activeElement, button("tokens"));
        assert.equal(table().closest(".panel").scrollLeft, 85);
        const keys = () => rows().map((entry) => app.text(entry.children[0]).replace(prefix, ""));
        assert.deepEqual(keys().slice(0, 2), ["good", "zero"]);
        button("tokens").events.click();
        assert.deepEqual(keys().slice(0, 2), ["zero", "good"]);
        button("cacheHit").events.click();
        assert.equal(header("cacheHit").attributes["aria-sort"], "descending");
        assert.deepEqual(keys().slice(-3), ["missing", "over-cache", "zero"]);
    }
    assert.deepEqual(JSON.parse(saved.get("attendant-dashboard.fusionModelView")),
        { unit: "record", sort: "cacheHit", direction: "desc" });
    assert.deepEqual(JSON.parse(saved.get("attendant-dashboard.fusionWorkspaceView")),
        { unit: "record", sort: "cacheHit", direction: "desc" });
    const reloaded = interactiveFusion(data, { fusionOnly: true, storage });
    assert.equal(reloaded.modelHeader("cacheHit").attributes["aria-sort"], "descending");
    assert.equal(reloaded.workspaceHeader("cacheHit").attributes["aria-sort"], "descending");
    assert.deepEqual(app.submitted, []);
    assert.deepEqual(data, before);
});

function fusionModelDataset() {
    const data = projectFusion(fusionDataset());
    data.FusionUsage.ByModel = fusionAgentEntries().map((entry, index) => ({
        ...entry, Key: `model-${index ? "b" : "a"}`, Sessions: index ? 1 : 2,
    }));
    return data;
}

function fusionWorkspaceDataset() {
    const data = fusionModelDataset();
    const [first, second] = fusionAgentEntries();
    first.Key = "/fictional/workspace-a";
    first.Sessions = 2;
    first.Metrics.AIU.CompleteRecords = first.UsageRecords;
    second.Key = "/fictional/workspace-b";
    second.Sessions = 1;
    second.Metrics.AIU.Total = 3;
    second.Metrics.AIU.ObservedRecords = second.Metrics.AIU.CompleteRecords = second.UsageRecords;
    second.Metrics.PromptTokens.Total = 300;
    second.Metrics.PromptTokens.ObservedRecords = second.Metrics.PromptTokens.CompleteRecords = second.UsageRecords;
    second.Metrics.OutputTokens.Total = 0;
    second.Metrics.OutputTokens.ObservedRecords = second.Metrics.OutputTokens.CompleteRecords = second.UsageRecords;
    data.FusionUsage.ByCWD = [first, second];
    return data;
}

test("Fusion-only Models selects one unit cost with comparison bars and sorts locally by the selected unit", () => {
    const data = fusionModelDataset();
    const before = structuredClone(data);
    const app = interactiveFusion(data, { fusionOnly: true });
    const costs = () => app.modelRows().map((row) => app.text(row.children.at(-2)));
    const widths = () => app.modelRows().map((row) => row.children.at(-1).children[0].children.map((bar) => bar.style.width));
    assert.equal(app.modelUnit("Usage record").attributes["aria-pressed"], "true");
    assert.equal(app.modelUnit("Session").attributes["aria-pressed"], "false");
    assert.match(app.text(app.modelButton("unit").children[0]), /^AIU \/ record$/);
    assert.deepEqual(app.modelNames(), ["model-b", "model-a"]);
    assert.deepEqual(costs(), ["0.75", "1"]);
    assert.deepEqual(widths(), [["75%"], ["100%"]]);
    assert.equal(app.walk(app.modelTable()).filter((node) => node.tag === "th" &&
        app.text(node).startsWith("AIU / ")).length, 1);
    assert.ok(app.walk(app.modelTable()).some((node) => node.attributes["aria-label"] === "Unit cost comparison"));
    assert.equal(app.walk(app.modelTable()).some((node) => /API requests/.test(app.text(node))), false);
    const workspacePanel = app.workspaceTable().closest(".panel");
    assert.ok(app.walk(workspacePanel).some((node) => node.className === "segmented"));
    assert.deepEqual(app.walk(app.workspaceTable()).filter((node) => node.tag === "th")
        .map(app.text).filter((label) => label.startsWith("AIU / ")), ["AIU / record↕"]);
    assert.deepEqual(app.walk(app.agentTable()).filter((node) => node.tag === "th")
        .map(app.text).filter((label) => label.startsWith("AIU / ")),
    ["AIU / session↕", "AIU / record↕", "AIU / 1M tokens↕"]);
    for (const id of ["model", "sessions", "records", "aiu", "premium", "prompt", "uncached",
        "cacheRead", "cacheWrite", "output", "apiTime", "unit"]) {
        assert.equal(app.modelButton(id).attributes.type, "button");
        assert.equal(app.modelHeader(id).attributes.scope, "col");
    }

    app.modelTable().closest(".panel").scrollLeft = 90;
    app.context.document.activeElement = app.modelButton("unit");
    app.modelButton("unit").events.click();
    assert.equal(app.modelHeader("unit").attributes["aria-sort"], "descending");
    assert.deepEqual(app.modelNames(), ["model-a", "model-b"]);
    assert.equal(app.context.document.activeElement, app.modelButton("unit"));
    assert.equal(app.modelTable().closest(".panel").scrollLeft, 90);
    app.modelUnit("Session").events.click();
    assert.equal(app.text(app.modelButton("unit").children[0]), "AIU / session");
    assert.deepEqual(app.modelNames(), ["model-b", "model-a"]);
    assert.deepEqual(costs(), ["3", "1"]);
    assert.deepEqual(widths()[0], ["100%"]);
    assert.ok(Math.abs(parseFloat(widths()[1][0]) - 100 / 3) < 1e-10);
    app.modelUnit("1M tokens").events.click();
    assert.equal(app.text(app.modelButton("unit").children[0]), "AIU / 1M tokens");
    assert.deepEqual(app.modelNames(), ["model-a", "model-b"]);
    assert.deepEqual(costs(), ["13,333.33", "10,000"]);
    assert.deepEqual(widths(), [["100%"], ["75%"]]);
    app.modelButton("unit").events.click();
    assert.deepEqual(app.modelNames(), ["model-b", "model-a"]);
    assert.equal(app.modelHeader("unit").attributes["aria-sort"], "ascending");
    assert.deepEqual(app.submitted, []);
    assert.deepEqual(data, before);
});

test("Fusion-only Workspaces selects one unit cost with comparison bars, sorting, and scope filtering", () => {
    const data = fusionWorkspaceDataset();
    const before = structuredClone(data);
    const app = interactiveFusion(data, { fusionOnly: true });
    const costs = () => app.workspaceRows().map((row) => app.text(row.children.at(-2)));
    const widths = () => app.workspaceRows().map((row) => row.children.at(-1).children[0].children.map((bar) => bar.style.width));
    assert.equal(app.workspaceUnit("Usage record").attributes["aria-pressed"], "true");
    assert.deepEqual(app.workspaceNames(), ["/fictional/workspace-b", "/fictional/workspace-a"]);
    assert.deepEqual(costs(), ["0.75", "1"]);
    assert.deepEqual(widths(), [["75%"], ["100%"]]);
    assert.equal(app.walk(app.workspaceTable()).filter((node) => node.tag === "th" &&
        app.text(node).startsWith("AIU / ")).length, 1);
    for (const id of ["workspace", "sessions", "records", "aiu", "premium", "prompt", "uncached",
        "cacheRead", "cacheWrite", "output", "apiTime", "unit"]) {
        assert.equal(app.workspaceButton(id).attributes.type, "button");
        assert.equal(app.workspaceHeader(id).attributes.scope, "col");
    }

    app.workspaceButton("unit").events.click();
    assert.deepEqual(app.workspaceNames(), ["/fictional/workspace-a", "/fictional/workspace-b"]);
    assert.equal(app.workspaceHeader("unit").attributes["aria-sort"], "descending");
    app.workspaceUnit("Session").events.click();
    assert.equal(app.text(app.workspaceButton("unit").children[0]), "AIU / session");
    assert.deepEqual(app.workspaceNames(), ["/fictional/workspace-b", "/fictional/workspace-a"]);
    assert.deepEqual(costs(), ["3", "1"]);
    app.workspaceUnit("1M tokens").events.click();
    assert.equal(app.text(app.workspaceButton("unit").children[0]), "AIU / 1M tokens");
    assert.deepEqual(app.workspaceNames(), ["/fictional/workspace-a", "/fictional/workspace-b"]);
    assert.deepEqual(costs(), ["13,333.33", "10,000"]);
    assert.deepEqual(widths(), [["100%"], ["75%"]]);

    app.workspaceRows()[0].children[0].children[0].events.click();
    assert.deepEqual(JSON.parse(JSON.stringify(app.submitted)), [{ scope: "cwd", path: "/fictional/workspace-a" }]);
    assert.deepEqual(data, before);
});

test("Fusion-only Workspaces preferences persist independently and incomplete costs remain unavailable", () => {
    const saved = new Map([
        ["attendant-dashboard.fusionModelView", JSON.stringify({ unit: "session", sort: "requests", direction: "asc" })],
        ["attendant-dashboard.workspaceView", JSON.stringify({ unit: "mtok", sort: "workspace", direction: "desc" })],
    ]);
    const storage = { getItem: (key) => saved.get(key) ?? null, setItem: (key, value) => saved.set(key, value) };
    const app = interactiveFusion(fusionWorkspaceDataset(), { fusionOnly: true, storage });
    assert.equal(app.workspaceUnit("Usage record").attributes["aria-pressed"], "true");
    const modelPreference = saved.get("attendant-dashboard.fusionModelView");
    const workspacePreference = saved.get("attendant-dashboard.workspaceView");
    app.workspaceUnit("Session").events.click();
    app.workspaceButton("unit").events.click();
    assert.deepEqual(JSON.parse(saved.get("attendant-dashboard.fusionWorkspaceView")),
        { unit: "session", sort: "unit", direction: "desc" });
    assert.equal(saved.get("attendant-dashboard.fusionModelView"), modelPreference);
    assert.equal(saved.get("attendant-dashboard.workspaceView"), workspacePreference);
    const reloaded = interactiveFusion(fusionWorkspaceDataset(), { fusionOnly: true, storage });
    assert.equal(reloaded.workspaceUnit("Session").attributes["aria-pressed"], "true");
    assert.equal(reloaded.workspaceHeader("unit").attributes["aria-sort"], "descending");

    const incomplete = fusionWorkspaceDataset();
    incomplete.FusionUsage.ByCWD[0].Metrics.AIU.CompleteRecords = 1;
    const partial = interactiveFusion(incomplete, { fusionOnly: true });
    const row = partial.workspaceRows().find((entry) => partial.text(entry.children[0]) === "/fictional/workspace-a");
    assert.equal(partial.text(row.children.at(-2)), "–");
    assert.equal(row.children.at(-1).children[0].children.length, 0);
    assert.deepEqual(app.submitted, []);
    assert.deepEqual(reloaded.submitted, []);
});

test("Fusion-only Models keeps incomplete costs unavailable and small or recorded-zero costs accurate", () => {
    const data = projectFusion(fusionDataset());
    const zero = structuredClone(data.FusionUsage.ByModel[1]);
    zero.Key = "";
    zero.Metrics.AIU.Total = 0;
    data.FusionUsage.ByModel.push(zero);
    data.FusionUsage.ModelGroups = 3;
    const app = interactiveFusion(data, { fusionOnly: true });
    const row = (key) => app.modelRows().find((entry) => app.text(entry.children[0]) === key);
    const cost = (key) => app.text(row(key).children.at(-2));
    const bars = (key) => row(key).children.at(-1).children[0].children;
    assert.equal(cost("model-a"), "–");
    assert.equal(bars("model-a").length, 0);
    assert.equal(cost("model-b"), "<0.01");
    assert.equal(bars("model-b")[0].style.width, "100%");
    assert.match(bars("model-b")[0].attributes.title, /AIU \/ record: <0\.01$/);
    assert.equal(cost("Unknown model"), "0");
    assert.equal(bars("Unknown model").length, 0);
    for (const direction of ["desc", "asc"]) {
        app.modelButton("unit").events.click();
        assert.equal(app.modelHeader("unit").attributes["aria-sort"], direction === "asc" ? "ascending" : "descending");
        assert.equal(app.modelNames().at(-1), "model-a");
    }
    app.modelUnit("1M tokens").events.click();
    assert.equal(cost("model-b"), "<0.01");
    assert.equal(cost("model-a"), "–");

    for (const unavailable of [false, true]) {
        const only = structuredClone(zero);
        only.Key = "only-model";
        if (unavailable) only.Metrics.AIU.ObservedRecords = only.Metrics.AIU.CompleteRecords = 0;
        const isolated = fusionModelDataset();
        isolated.FusionUsage.ByModel = [only];
        const single = interactiveFusion(isolated, { fusionOnly: true });
        const cells = single.modelRows()[0].children;
        assert.equal(single.text(cells.at(-2)), unavailable ? "–" : "0");
        assert.equal(cells.at(-1).children[0].children.length, 0);
    }
});

test("Fusion-only Models preferences persist independently of normal Models and respect Top", () => {
    const saved = new Map([["attendant-dashboard.sessionModelView", JSON.stringify({ unit: "session", sort: "requests", direction: "asc" })]]);
    const storage = { getItem: (key) => saved.get(key) ?? null, setItem: (key, value) => saved.set(key, value) };
    const original = saved.get("attendant-dashboard.sessionModelView");
    const app = interactiveFusion(fusionModelDataset(), { fusionOnly: true, storage });
    app.modelUnit("1M tokens").events.click();
    app.modelButton("unit").events.click();
    assert.deepEqual(JSON.parse(saved.get("attendant-dashboard.fusionModelView")), { unit: "mtok", sort: "unit", direction: "desc" });
    assert.equal(saved.get("attendant-dashboard.sessionModelView"), original);
    vm.runInContext(`
        state.query = { ...state.query, fusionOnly: false };
        state.results.session.data = { UsageSessions: 1, ByModelUsage: [{ Key: "ordinary", Sessions: 1, Requests: 1, AIU: 1 }] };
        render();
    `, app.context);
    assert.deepEqual(vm.runInContext("modelViews.session", app.context), { unit: "session", sort: "requests", direction: "asc" });
    assert.equal(saved.get("attendant-dashboard.fusionModelView"), JSON.stringify({ unit: "mtok", sort: "unit", direction: "desc" }));
    const reloaded = interactiveFusion(fusionModelDataset(), { fusionOnly: true, storage });
    assert.equal(reloaded.modelUnit("1M tokens").attributes["aria-pressed"], "true");
    assert.equal(reloaded.modelHeader("unit").attributes["aria-sort"], "descending");
    assert.deepEqual(reloaded.modelNames(), ["model-a", "model-b"]);
    const limited = interactiveFusion(projectFusion(fusionDataset(1)), { fusionOnly: true, top: 1 });
    limited.modelUnit("Session").events.click();
    limited.modelButton("model").events.click();
    assert.deepEqual(limited.modelNames(), ["model-a"]);
    assert.deepEqual(app.submitted, []);
    assert.deepEqual(reloaded.submitted, []);
    assert.deepEqual(limited.submitted, []);
});

test("Fusion-only Models handles empty results and invalid or unavailable preferences", () => {
    const empty = renderedPanels(normalizeQuery({ fusionOnly: true }), projectFusion(emptyFusionDataset()));
    const model = empty.panels.find((panel) => empty.walk(panel).some((node) => node.tag === "h2" && node.children[0] === "Models"));
    assert.equal(empty.walk(model).some((node) => node.tag === "table"), false);
    assert.ok(empty.walk(model).some((node) => node.children.includes("No Fusion-tagged usage records.")));
    for (const value of ["{invalid", '{"unit":"request","sort":"invalid","direction":"invalid"}', null]) {
        const storage = {
            getItem: (key) => {
                if (key !== "attendant-dashboard.fusionModelView") return null;
                if (value === null) throw new Error("storage unavailable");
                return value;
            },
            setItem: () => { throw new Error("storage unavailable"); },
        };
        const app = interactiveFusion(fusionModelDataset(), { fusionOnly: true, storage });
        assert.equal(app.modelUnit("Usage record").attributes["aria-pressed"], "true");
        assert.equal(app.modelHeader("aiu").attributes["aria-sort"], "descending");
        app.modelUnit("Session").events.click();
        assert.equal(app.modelUnit("Session").attributes["aria-pressed"], "true");
        assert.ok(app.warnings.some(([message]) => message.includes("Could not save table view preferences")));
        assert.deepEqual(app.submitted, []);
    }
});

test("AIU formatting is capped at two decimals while API time retains small values", () => {
    const { context } = renderedPanels(normalizeQuery({ source: "session", fusionOnly: true }), projectFusion(fusionDataset()));
    const format = (value) => vm.runInContext(`fusionNumber(${value})`, context);
    for (const [value, expected] of [
        [0, "0"], [0.0000000000001, "<0.01"], [0.0099, "<0.01"],
        [0.01, "0.01"], [1, "1"], [12.34567, "12.35"], [1234.567, "1,234.57"],
    ]) {
        assert.equal(format(value), expected);
        assert.ok(!/[eE]/.test(format(value)));
        const fraction = format(value).split(".")[1];
        assert.ok(!fraction || fraction.replace(/\D/g, "").length <= 2);
    }
    assert.equal(vm.runInContext("fusionLowerBoundNumber(12.349)", context), "12.34");
    assert.equal(vm.runInContext("fusionLowerBoundNumber(0.009)", context), "0");
    assert.equal(vm.runInContext("fusionDuration(0.0000000000001)", context), "1.000e-13 ms");
    assert.equal(vm.runInContext("fusionNumber(0)", context), "0");
    assert.equal(vm.runInContext("fusionDuration(0)", context), "0 ms");
});

test("Fusion-only switch disables permission filters and persists across source tabs", () => {
    const query = normalizeQuery({ source: "session", filters: { session: { kind: ["shell"], agentMode: ["plan"] } } });
    const { context, submitted, walk } = renderedPanels(query, {});
    const root = new Element("section");
    root.replaceChildren = (...children) => { root.children = children; };
    context.FILTERS = FILTERS;
    context.document.getElementById = () => root;
    vm.runInContext('meta = { filters: FILTERS }; renderFilters("session");', context);
    let toggle = walk(root).find((node) => node.className === "toggle fusion-filter");
    let checkbox = walk(toggle).find((node) => node.tag === "input");
    assert.equal(checkbox.attributes.checked, undefined);
    checkbox.events.change({ target: { checked: true } });
    assert.equal(submitted.at(-1).fusionOnly, true);
    assert.deepEqual(Object.keys(submitted.at(-1)), ["fusionOnly"]);
    vm.runInContext('state.query = { ...state.query, fusionOnly: true }; renderFilters("session");', context);
    toggle = walk(root).find((node) => node.className === "toggle fusion-filter");
    checkbox = walk(toggle).find((node) => node.tag === "input");
    assert.equal(checkbox.attributes.checked, "");
    const disabled = walk(root).filter((node) => node.tag === "input" && node !== checkbox);
    assert.ok(disabled.length > 0 && disabled.every((node) => node.attributes.disabled === ""));
    const remove = walk(root).find((node) => node.attributes["aria-label"] === "Remove shell");
    assert.equal(remove.attributes.disabled, "");
    const removeMode = walk(root).find((node) => node.attributes["aria-label"] === "Remove plan");
    assert.equal(removeMode.attributes.disabled, "");
    vm.runInContext('state.query.source = "vscode"; renderFilters("vscode");', context);
    assert.equal(walk(root).some((node) => node.className === "toggle fusion-filter"), false);
    assert.equal(context.query.fusionOnly, false, "the original query object is not mutated");
    assert.equal(vm.runInContext("state.query.fusionOnly", context), true);
    assert.equal(vm.runInContext("state.query.filters.session.agentMode[0]", context), "plan");
});

test("mode filter inputs keep literal whitespace and remove only their own chips", () => {
    const query = filteredQuery("session");
    const { context, submitted, walk } = renderedPanels(query, {});
    const root = new Element("section");
    root.replaceChildren = (...children) => { root.children = children; };
    context.FILTERS = FILTERS;
    context.document.getElementById = () => root;
    vm.runInContext('meta = { filters: FILTERS }; renderFilters("session");', context);
    const input = walk(root).find((node) => node.attributes["aria-label"] === "Add Agent mode filter");
    assert.equal(input.attributes.placeholder, "exact value");
    input.events.keydown({ key: "Enter", preventDefault() {}, target: { value: " future,mode " } });
    assert.deepEqual(structuredClone(submitted.at(-1)), { filters: { session: { agentMode: ["interactive", "plan", " future,mode "] } } });
    walk(root).find((node) => node.attributes["aria-label"] === "Remove plan").events.click();
    assert.deepEqual(structuredClone(submitted.at(-1)), { filters: { session: { agentMode: ["interactive"] } } });
    assert.deepEqual(query.filters.session.agentMode, ["interactive", "plan"]);
    const permissionInput = walk(root).find((node) => node.attributes["aria-label"] === "Add Permission mode filter");
    assert.equal(permissionInput.attributes.placeholder, "exact value");
    permissionInput.events.keydown({ key: "Enter", preventDefault() {}, target: { value: " future,mode " } });
    assert.deepEqual(structuredClone(submitted.at(-1)), { filters: { session: { permissionMode: ["assisted", " future,mode "] } } });
    walk(root).find((node) => node.attributes["aria-label"] === "Remove assisted").events.click();
    assert.deepEqual(structuredClone(submitted.at(-1)), { filters: { session: { permissionMode: [] } } });
    assert.deepEqual(query.filters.session.permissionMode, ["assisted"]);
});

test("a stale dashboard provider disables the Fusion switch and explains that a reload is needed", () => {
    const query = normalizeQuery({ source: "session" });
    const { context, walk } = renderedPanels(query, {});
    const root = new Element("section");
    root.replaceChildren = (...children) => { root.children = children; };
    context.FILTERS = FILTERS;
    context.document.getElementById = () => root;
    vm.runInContext('delete state.query.fusionOnly; meta = { filters: FILTERS }; renderFilters("session");', context);
    const toggle = walk(root).find((node) => node.className === "toggle fusion-filter");
    const checkbox = walk(toggle).find((node) => node.tag === "input");
    assert.equal(checkbox.attributes.disabled, "");
    assert.ok(walk(root).some((node) => node.children.some((child) =>
        typeof child === "string" && child.includes("Reload the dashboard extension"))));
});

function periodForm() {
    const view = renderedPanels(normalizeQuery(), {});
    const elements = Object.fromEntries(
        ["refresh", "reset", "query", "ask-send", "ask-text", "copy"].map((id) => [id, new Element("div")]),
    );
    const form = elements.query;
    for (const name of ["scope", "path", "period", "since", "until", "session", "top"]) {
        form[name] = new Element("input");
        form[name].value = "";
    }
    form.scope.value = "all";
    form.top.value = "10";
    form.period.options = ["", "this-month", "last-month", "7d", "30d"].map((value) => ({ value }));
    view.context.document.getElementById = (id) => elements[id];
    view.context.document.querySelectorAll = () => [];
    view.context.FormData = class {
        get(name) { return form[name].value; }
    };
    vm.runInContext("wire();", view.context);
    return { ...view, form };
}

test("calendar presets use local calendar boundaries, including leap years and year rollover", () => {
    const { context } = periodForm();
    for (const [year, month] of [[2026, 0], [2024, 2], [2026, 10], [2026, 11]]) {
        const now = new Date(year, month, 15, 12);
        const start = new Date(year, month, 1).toISOString();
        const previous = new Date(year, month - 1, 1).toISOString();
        const current = context.calendarPeriodBounds("this-month", now);
        const last = context.calendarPeriodBounds("last-month", now);
        assert.equal(current.period, "");
        assert.equal(current.since, start);
        assert.equal(current.until, "");
        assert.equal(last.since, previous);
        assert.equal(last.until, start, "Until is the exclusive start of this month");
        assert.equal(context.periodSelection(current, now), "this-month");
        assert.equal(context.periodSelection(last, now), "last-month");
        assert.equal(context.periodSelection(last, new Date(year, month + 1, 15)), "");
        const args = normalizeQuery({ ...last });
        assert.equal(args.since, previous);
        assert.equal(args.until, start);
    }
    assert.equal(context.calendarPeriodBounds("7d"), null);
});

test("Period changes clear stale bounds and manual edits leave calendar presets", () => {
    const { form, context } = periodForm();
    for (const period of ["7d", "30d"]) {
        form.since.value = "2026-01-01";
        form.until.value = "2026-02-01";
        form.period.value = period;
        form.period.events.change();
        assert.equal(form.since.value, "");
        assert.equal(form.until.value, "");
        assert.equal(context.readForm().period, period);
    }
    form.until.value = "2026-03-01";
    form.until.events.input();
    assert.equal(form.period.value, "30d", "explicit Until remains compatible with rolling periods");
    form.period.value = "";
    form.period.events.change();
    assert.equal(form.until.value, "2026-03-01", "None preserves manual bounds");
    for (const period of ["this-month", "last-month"]) {
        form.period.value = period;
        form.period.events.change();
        assert.ok(form.since.value.endsWith("Z"));
        assert.equal(Boolean(form.until.value), period === "last-month");
        const submitted = context.readForm();
        assert.equal(submitted.period, "", "UI presets never reach CLI --period");
        assert.equal(submitted.since, form.since.value);
        assert.equal(submitted.until, form.until.value);
        for (const name of ["since", "until"]) {
            form.period.value = period;
            form.period.events.change();
            form[name].value = "";
            form[name].events.input();
            assert.equal(form.period.value, "", `deleting ${name} exits the preset`);
        }
    }
});

test("fillForm restores calendar selections and invalidates their labels across month rollover", () => {
    const { form, context } = periodForm();
    const setNow = (year, month) => {
        context.Date = class extends Date {
            constructor(...args) { super(...(args.length ? args : [new Date(year, month, 15).getTime()])); }
        };
    };
    setNow(2026, 10);
    const bounds = context.calendarPeriodBounds("this-month");
    const query = normalizeQuery({ ...bounds });
    context.fillForm(query);
    assert.equal(form.period.value, "this-month");
    setNow(2026, 11);
    context.fillForm(query);
    assert.equal(form.period.value, "", "expired This month label is not cached");
    assert.equal(form.since.value, bounds.since, "saved dates do not silently move forward");
    assert.equal(context.readForm().since, bounds.since);
    context.fillForm(normalizeQuery({ period: "7d" }));
    assert.equal(form.period.value, "7d");
    assert.equal(form.until.value, "");
});

test("Reset omits the Fusion field for an old provider and clears it for a current provider", () => {
    for (const source of ["session", "vscode"]) {
        for (const supported of [false, true]) {
            const query = normalizeQuery({ source, fusionOnly: true });
            const { context, submitted } = renderedPanels(query, {});
            const elements = Object.fromEntries(
                ["refresh", "reset", "query", "ask-send", "ask-text", "copy"].map((id) => [id, new Element("div")]),
            );
            for (const name of ["scope", "path", "period", "since", "until"]) elements.query[name] = new Element("input");
            context.document.getElementById = (id) => elements[id];
            context.document.querySelectorAll = () => [];
            context.FILTERS = FILTERS;
            vm.runInContext(
                `meta = { sources: ["session", "vscode"], filters: FILTERS }; ${supported ? "" : "delete state.query.fusionOnly;"} wire();`,
                context,
            );
            elements.reset.events.click();
            const patch = submitted.at(-1);
            assert.equal(patch.source, source);
            assert.equal(patch.scope, "worktree");
            assert.equal(patch.period, "30d");
            assert.equal(patch.top, 20);
            assert.equal(Object.hasOwn(patch, "fusionOnly"), supported);
            if (supported) assert.equal(patch.fusionOnly, false);
            for (const [tab, filters] of Object.entries(FILTERS)) {
                for (const filter of filters) assert.equal(patch.filters[tab][filter].length, 0);
            }
        }
    }
});

test("CLI usage time guidance matches the supported payload", () => {
    const query = normalizeQuery({ source: "session" });
    const text = (node) => typeof node === "string" ? node : node instanceof Element ? node.children.map(text).join(" ") : "";
    const legacy = renderedPanels(query, { UsageSessions: 1, ByRoleUsage: [] });
    assert.match(legacy.nodes.map(text).join(" "), /sessions' lifetime usage/);
    const current = renderedPanels(query, { UsageSessions: 1, ByRoleUsage: [], Fusion: {} });
    const content = current.nodes.map(text).join(" ");
    assert.match(content, /select recorded usage updates/);
    assert.match(content, /may include usage from before the selected period/);
    assert.doesNotMatch(content, /sessions' lifetime usage/);
});

test("unfiltered panels disable Clear, while filters with no matching rows still allow clearing", () => {
    const query = normalizeQuery({ source: "vscode", scope: "all" });
    const { panels, walk } = renderedPanels(query, {});
    for (const panel of panels) {
        const button = walk(panel).find((node) => node.attributes["data-clear-filter"]);
        assert.ok(button);
        assert.equal(button.attributes.disabled, "");
    }
    const filtered = renderedPanels(normalizeQuery({ filters: { vscode: { agent: ["Nobody"] } } }, query), {});
    const button = filtered.panels.flatMap(filtered.walk).find((node) => node.attributes["data-clear-filter"] === "agent");
    assert.equal(button.attributes.disabled, undefined);
    button.events.click();
    assert.deepEqual(filtered.submitted, [{ filters: { vscode: { agent: [] } } }]);
});

test("User questions panel displays outcomes, missing timing notes, and unavailable averages without Clear", () => {
    const query = filteredQuery("session");
    const data = { AskUser: {
        Sessions: 1, Questions: 6, Answered: 1, Cancelled: 1, Failed: 1, Unresolved: 2, Unknown: 1,
        TimedAnswers: 0, UntimedAnswers: 1, TotalWaitMs: 0, AvgWaitMs: null, MaxWaitMs: null,
    } };
    const { panels, walk } = renderedPanels(query, data);
    const panel = panels.find((node) => walk(node).some((child) => child.tag === "h2" && child.children[0] === "User questions"));
    assert.ok(panel);
    const nodes = walk(panel);
    assert.equal(nodes.some((node) => node.attributes["data-clear-filter"]), false);
    const text = nodes.flatMap((node) => node.children.filter((child) => typeof child === "string")).join("\n");
    for (const label of ["Questions", "Answered", "Cancelled", "Failed", "Unresolved", "Unknown outcome",
        "Average answer wait", "Longest answer wait", "missing or invalid timestamps", "not unique wall-clock"]) {
        assert.ok(text.includes(label), label);
    }
    assert.equal(nodes.filter((node) => node.className === "value" && node.children[0] === "–").length, 2);
    const legacy = renderedPanels(query, {});
    assert.equal(legacy.panels.some((node) => legacy.walk(node).some((child) => child.tag === "h2" && child.children[0] === "User questions")), false);
});

test("VS Code User questions panel uses completed-call units, supports skips, and does not invent unresolved counts", () => {
    const data = { UserQuestions: {
        Sessions: 1, Calls: 3, Answered: 1, Skipped: 1, Cancelled: 1, Failed: 0, Unknown: 0,
        TimedAnswers: 1, UntimedAnswers: 0, TotalWaitMs: 0, AvgWaitMs: 0, MaxWaitMs: 0,
    } };
    const { panels, walk } = renderedPanels(filteredQuery("vscode"), data);
    const panel = panels.find((node) => walk(node).some((child) => child.tag === "h2" && child.children[0] === "User questions"));
    assert.ok(panel);
    const nodes = walk(panel);
    const text = nodes.flatMap((node) => node.children.filter((child) => typeof child === "string")).join("\n");
    assert.match(text, /Question tool calls/);
    assert.match(text, /Skipped/);
    assert.match(text, /multi-question batch/);
    assert.match(text, /in-flight questions/);
    assert.doesNotMatch(text, /Unresolved/);
    assert.equal(nodes.filter((node) => node.className === "value" && node.children[0] === "0 ms").length, 3);
    assert.equal(nodes.some((node) => node.attributes["data-clear-filter"]), false);
    const legacy = renderedPanels(filteredQuery("vscode"), {});
    assert.equal(legacy.panels.some((node) => legacy.walk(node).some((child) => child.tag === "h2" && child.children[0] === "User questions")), false);
});

test("CLI question and permission panels use shared framing and follow the VS Code workspace/question order", () => {
    const query = filteredQuery("session");
    const before = structuredClone(query);
    const askUser = { Sessions: 1, Questions: 1, Answered: 1, TimedAnswers: 1, TotalWaitMs: 0, AvgWaitMs: 0, MaxWaitMs: 0 };
    const data = {
        UsageSessions: 2, ByModelUsage: [], ByRoleUsage: [], ByCWDUsage: [],
        Sessions: 2, Requests: 4, AskUser: askUser,
        ByKind: [{ Key: "shell", Total: 4, Approved: 2, ApprovedForLocation: 0, Denied: 1, Unresolved: 1 }],
    };
    const { nodes, panels, walk } = renderedPanels(query, data);
    const title = (panel) => walk(panel).find((node) => node.tag === "h2").children[0];
    const titles = Array.from(panels, title);
    const workspace = titles.indexOf("Workspaces");
    assert.deepEqual(titles.slice(workspace, workspace + 4), ["Workspaces", "User questions", "Agent usage", "Summary"]);
    assert.equal(titles.filter((text) => text === "User questions").length, 1);
    const usageGrid = nodes.flatMap(walk).find((node) => node?.className === "grid" &&
        node.children.some((child) => child?.tag === "section" && title(child) === "Workspaces"));
    assert.deepEqual(usageGrid.children.filter((node) => node?.tag === "section").map(title).slice(-3),
        ["Workspaces", "User questions", "Agent usage"]);
    const permissions = panels.find((panel) => title(panel) === "Summary");
    assert.equal(permissions.className, "panel wide");
    assert.equal(permissions.children[0].tag, "details");
    assert.ok(walk(permissions).some((node) => node.className === "legend"));
    const cards = walk(permissions).filter((node) => node.className === "card");
    assert.deepEqual(cards.map((node) => walk(node).find((child) => child.className === "value").children[0]), ["2", "4", "2", "1", "1"]);
    assert.equal(walk(permissions).some((node) => node.attributes["data-clear-filter"]), false);
    assert.deepEqual(query, before);
    const vscode = renderedPanels(filteredQuery("vscode"), { UserQuestions: { ...askUser, Calls: 1 } });
    const vscodeTitles = Array.from(vscode.panels, (panel) => vscode.walk(panel).find((node) => node.tag === "h2").children[0]);
    assert.deepEqual(vscodeTitles.slice(vscodeTitles.indexOf("Workspaces"), vscodeTitles.indexOf("Workspaces") + 2),
        ["Workspaces", "User questions"]);
});

test("permission-only and older CLI payloads retain framed summaries and optional question panels", () => {
    for (const data of [{ Sessions: 0, Requests: 0 }, { Sessions: 1, Requests: 0, AskUser: {} }, { UsageSessions: 0, ByRoleUsage: [] }]) {
        const { panels, walk } = renderedPanels(filteredQuery("session"), data);
        const titles = panels.map((panel) => walk(panel).find((node) => node.tag === "h2").children[0]);
        assert.equal(titles.filter((title) => title === "Summary").length, 1);
        assert.equal(titles.filter((title) => title === "User questions").length, data.AskUser ? 1 : 0);
        assert.equal(panels[titles.indexOf("Summary")].className, "panel wide");
    }
});

test("Permissions contains human response wait and separate non-human/unknown timings without changing panel order", () => {
    const empty = { Completed: 0, Timed: 0, Untimed: 0, TotalWaitMs: 0, AvgWaitMs: null, MaxWaitMs: null };
    const data = { Requests: 5, UsageSessions: 0, ByRoleUsage: [], AskUser: {}, PermissionWait: {
        HumanResponses: { ...empty, Completed: 2, Timed: 1, Untimed: 1, TotalWaitMs: 1000, AvgWaitMs: 1000, MaxWaitMs: 1000 },
        NonHumanDecisions: { ...empty, Completed: 1, Timed: 1, TotalWaitMs: 0, AvgWaitMs: 0, MaxWaitMs: 0 },
        UnknownSourceDecisions: { ...empty, Completed: 1, Timed: 1, TotalWaitMs: 60000, AvgWaitMs: 60000, MaxWaitMs: 60000 },
        Unresolved: 1,
    } };
    const query = filteredQuery("session");
    const { panels, walk } = renderedPanels(query, data);
    const title = (panel) => walk(panel).find((node) => node.tag === "h2").children[0];
    const titles = Array.from(panels, title);
    assert.deepEqual(titles.slice(titles.indexOf("User questions"), titles.indexOf("Summary") + 1),
        ["User questions", "Agent usage", "Summary"]);
    const permissions = panels[titles.indexOf("Summary")];
    const nodes = walk(permissions);
    const waitHeading = nodes.find((node) => node.children[0] === "Permission response wait");
    assert.equal(waitHeading.tag, "h3");
    assert.equal(walk(permissions).find((node) => node.children[0] === "Summary").tag, "h2");
    const text = nodes.flatMap((node) => node.children.filter((child) => typeof child === "string")).join("\n");
    for (const label of ["Permission response wait", "Human responses", "Total permission wait", "Average permission wait",
        "Longest permission wait", "No recorded decision", "Other recorded sources", "Unknown source", "missing or invalid timestamps",
        "Top does not", "Unresolved requests have no inferred duration"]) {
        assert.ok(text.includes(label), label);
    }
    assert.equal(nodes.filter((node) => node.className === "value" && node.children[0] === "1.0 s").length, 3);
    assert.equal(permissions.className, "panel wide");
    const legacy = renderedPanels(query, { Requests: 1 });
    assert.equal(legacy.panels.flatMap(legacy.walk).some((node) => node.tag === "h3" && node.children[0] === "Permission response wait"), false);
});

test("unknown-source permission timing never fabricates human wait samples", () => {
    const empty = { Completed: 0, Timed: 0, Untimed: 0, TotalWaitMs: 0, AvgWaitMs: null, MaxWaitMs: null };
    const data = { PermissionWait: {
        HumanResponses: empty, NonHumanDecisions: empty,
        UnknownSourceDecisions: { ...empty, Completed: 1, Timed: 1, TotalWaitMs: 60000, AvgWaitMs: 60000, MaxWaitMs: 60000 },
        Unresolved: 0,
    } };
    const { panels, walk } = renderedPanels(filteredQuery("session"), data);
    const permissions = panels.find((panel) => walk(panel).some((node) => node.tag === "h2" && node.children[0] === "Summary"));
    const nodes = walk(permissions);
    assert.equal(nodes.filter((node) => node.className === "value" && node.children[0] === "–").length, 2);
    assert.equal(nodes.filter((node) => node.className === "value" && node.children[0] === "0 ms").length, 1);
});

test("Agent modes filters exact permission modes and clears only its own filter", () => {
    const query = filteredQuery("session");
    const before = structuredClone(query);
    const data = { ByAgentMode: [
        { Key: "plan", Total: 4, Approved: 1, ApprovedForLocation: 1, Denied: 1, Unresolved: 1 },
        { Key: "unknown", Total: 2, Approved: 0, ApprovedForLocation: 0, Denied: 0, Unresolved: 2 },
        { Key: " <future,mode> ", Total: 1, Approved: 1, ApprovedForLocation: 0, Denied: 0, Unresolved: 0 },
    ] };
    const { panels, walk, submitted } = renderedPanels(query, data);
    const panel = panels.find((entry) => walk(entry).some((node) => node.tag === "h2" && node.children[0] === "Agent modes"));
    assert.ok(panel);
    const nodes = walk(panel);
    const rows = nodes.filter((node) => node.className === "row");
    assert.deepEqual(rows.map((row) => row.attributes.title), [
        "Approved: 1\nApproved for location: 1\nDenied: 1\nUnresolved: 1",
        "Approved: 0\nApproved for location: 0\nDenied: 0\nUnresolved: 2",
        "Approved: 1\nApproved for location: 0\nDenied: 0\nUnresolved: 0",
    ]);
    const text = nodes.flatMap((node) => node.children.filter((child) => typeof child === "string")).join("\n");
    for (const expected of ["plan", "unknown", " <future,mode> ", "Missing modes are unknown", "permission filters and Top apply"]) {
        assert.ok(text.includes(expected), expected);
    }
    rows[1].children[0].events.click();
    assert.deepEqual(structuredClone(submitted.at(-1)), { filters: { session: { agentMode: ["interactive", "plan", "unknown"] } } });
    rows[2].children[0].events.click();
    assert.deepEqual(structuredClone(submitted.at(-1)), { filters: { session: { agentMode: ["interactive", "plan", " <future,mode> "] } } });
    const clear = nodes.find((node) => node.attributes["data-clear-filter"] === "agentMode");
    assert.equal(clear.attributes.disabled, undefined);
    clear.events.click();
    assert.deepEqual(submitted.at(-1), { filters: { session: { agentMode: [] } } });
    assert.deepEqual(query, before);
});

test("a stale dashboard provider disables Agent mode picking and explains that a reload is needed", () => {
    const query = filteredQuery("session");
    delete query.filters.session.agentMode;
    const filters = { ...FILTERS, session: FILTERS.session.filter((name) => name !== "agentMode") };
    const { panels, walk, submitted } = renderedPanels(query, {
        ByAgentMode: [{ Key: "plan", Total: 1, Approved: 1 }],
    }, { filters });
    const panel = panels.find((entry) => walk(entry).some((node) => node.tag === "h2" && node.children[0] === "Agent modes"));
    const nodes = walk(panel);
    assert.ok(nodes.some((node) => node.children.some((child) =>
        typeof child === "string" && child.includes("Reload the dashboard extension"))));
    assert.equal(nodes.some((node) => node.tag === "button"), false);
    assert.deepEqual(submitted, []);
});

test("Agent modes distinguishes empty results from older builds and stays outside Fusion-only", () => {
    for (const data of [{}, { ByAgentMode: null }, { ByAgentMode: [] }]) {
        const { panels, walk } = renderedPanels(filteredQuery("session"), data);
        const panel = panels.find((entry) => walk(entry).some((node) => node.tag === "h2" && node.children[0] === "Agent modes"));
        assert.equal(Boolean(panel), Array.isArray(data.ByAgentMode));
        if (panel) assert.ok(walk(panel).some((node) => node.children.includes("No entries.")));
    }
    const data = projectFusion({ ...fusionDataset(), ByAgentMode: [{ Key: "plan", Total: 1 }] });
    const { panels, walk } = renderedPanels(normalizeQuery({ source: "session", fusionOnly: true }), data);
    assert.equal(panels.some((panel) => walk(panel).some((node) => node.tag === "h2" && node.children[0] === "Agent modes")), false);
});

test("Permission modes combine with other filters and clear only their own values", () => {
    const query = filteredQuery("session");
    const before = structuredClone(query);
    const data = { ByPermissionMode: [
        { Key: "assisted", Total: 2, Approved: 1, Denied: 1 },
        { Key: "unknown", Total: 1, Unresolved: 1 },
        { Key: " future,mode ", Total: 1, Approved: 1 },
    ] };
    const { panels, walk, submitted } = renderedPanels(query, data);
    const panel = panels.find((entry) => walk(entry).some((node) => node.tag === "h2" && node.children[0] === "Permission modes"));
    assert.ok(panel);
    const nodes = walk(panel);
    const rows = nodes.filter((node) => node.className === "row");
    const text = nodes.flatMap((node) => node.children.filter((child) => typeof child === "string")).join("\n");
    assert.match(text, /Missing modes are unknown/);
    assert.match(text, /does not imply a safety review ran/);
    rows[2].children[0].events.click();
    assert.deepEqual(structuredClone(submitted.at(-1)), { filters: { session: { permissionMode: ["assisted", " future,mode "] } } });
    nodes.find((node) => node.attributes["data-clear-filter"] === "permissionMode").events.click();
    assert.deepEqual(submitted.at(-1), { filters: { session: { permissionMode: [] } } });
    assert.deepEqual(query, before);
    assert.deepEqual(query.filters.session.decisionSource, ["human_response"]);

    for (const oldData of [{}, { ByPermissionMode: null }]) {
        const old = renderedPanels(query, oldData);
        assert.equal(old.panels.some((entry) => old.walk(entry).some((node) => node.tag === "h2" && node.children[0] === "Permission modes")), false);
    }
    const filters = { ...FILTERS, session: FILTERS.session.filter((name) => name !== "permissionMode") };
    const stale = renderedPanels(query, data, { filters });
    const stalePanel = stale.panels.find((entry) => stale.walk(entry).some((node) => node.tag === "h2" && node.children[0] === "Permission modes"));
    const staleNodes = stale.walk(stalePanel);
    assert.ok(staleNodes.some((node) => node.children.some((child) =>
        typeof child === "string" && child.includes("Reload the dashboard extension"))));
    assert.equal(staleNodes.some((node) => node.tag === "button"), false);
});

function activityDataset() {
    const hours = Array(24).fill(0);
    hours[1] = 2;
    hours[4] = 1;
    const coverage = Array(24).fill("full");
    coverage[0] = "outside";
    coverage[1] = "partial";
    coverage[2] = "nonexistent";
    return {
        Version: 1, TimeZone: "Asia/Kathmandu", AsOf: "2026-03-03T11:00:00+05:45", AgentModes: [],
        StartDate: "2026-01-01", EndDate: "2026-03-03", EmptyReason: "",
        ScannedSessions: 3, UntimedSessions: 1, UntimedEvents: 2, MalformedEvents: 1,
        Days: [{ Date: "2026-03-02", Hours: hours }],
        Coverage: [{ Date: "2026-03-02", Hours: coverage }],
    };
}

function activityFooter(root, walk) {
    const table = walk(root).find((node) => node.className === "activity-table");
    return walk(table).find((node) => node.tag === "tfoot").children;
}

test("session activity renders accessible local-hour counts and distinguishes coverage", () => {
    const query = normalizeQuery({ source: "session", filters: { session: { kind: ["read"] } } });
    const data = { SessionActivity: activityDataset() };
    const before = structuredClone(data);
    const { panels, walk } = renderedPanels(query, data);
    const panel = panels.find((entry) => walk(entry).some((node) => node.tag === "h2" && node.children[0] === "Session activity"));
    assert.equal(panel.className, "panel wide");
    assert.equal(panels.at(-1), panel);
    const disclosure = walk(panel).find((node) => node.tag === "details");
    assert.equal(disclosure.open, true);
    assert.equal(disclosure.children[0].tag, "summary");
    assert.equal(disclosure.children[0].children[0].tag, "h2");
    const text = (node) => typeof node === "string" ? node : node instanceof Element ? node.children.map(text).join(" ") : "";
    const content = text(panel);
    for (const phrase of ["Asia/Kathmandu", "not continuous active time", "Other permission filters and Top do not apply",
        "2 included events in 1 scoped session have no usable timestamp", "1 event envelope across the scoped logs could not be parsed"]) {
        assert.ok(content.includes(phrase), phrase);
    }
    const table = walk(panel).find((node) => node.tag === "table");
    assert.match(table.attributes["aria-label"], /2026-02-02 to 2026-03-03/);
    const head = walk(table).find((node) => node.tag === "thead");
    assert.equal(walk(head).filter((node) => node.tag === "th").length, 25);
    const rows = walk(table).find((node) => node.tag === "tbody").children;
    assert.equal(rows.length, 30);
    const date = rows.find((row) => row.children[0].children[0] === "2026-03-02");
    assert.deepEqual(date.children.slice(1, 6).map(text), ["–", "2 *", "×", "0", "1"]);
    assert.match(date.children[1].attributes["aria-label"], /outside the selected time window/);
    assert.match(date.children[2].attributes["aria-label"], /2 sessions with recorded events; selected window covers only part/);
    assert.match(date.children[3].attributes["aria-label"], /local clock hour did not exist/);
    assert.equal(date.children[2].className.includes("activity-partial"), true);
    const zeroDay = rows.find((row) => row.children[0].children[0] === "2026-03-01");
    assert.equal(zeroDay.children.length, 25);
    assert.equal(zeroDay.children[1].children[0], "0");
    assert.deepEqual(data, before);
});

test("session activity footer aggregates covered daily cells exactly", () => {
    const hours = (values) => Array.from({ length: 24 }, (_, hour) => values[hour] ?? 0);
    const coverage = (values) => Array.from({ length: 24 }, (_, hour) => values[hour] ?? "full");
    const activity = {
        ...activityDataset(),
        StartDate: "2026-03-01", EndDate: "2026-03-03",
        Days: [
            { Date: "2026-03-01", Hours: hours({ 0: 6, 1: 2, 3: Number.MAX_SAFE_INTEGER, 4: 1 }) },
            { Date: "2026-03-02", Hours: hours({ 3: Number.MAX_SAFE_INTEGER }) },
        ],
        Coverage: [
            { Date: "2026-03-01", Hours: coverage({ 2: "outside" }) },
            { Date: "2026-03-02", Hours: coverage({ 0: "partial", 1: "outside", 2: "nonexistent" }) },
            { Date: "2026-03-03", Hours: coverage({ 1: "nonexistent", 2: "outside" }) },
        ],
    };
    const before = structuredClone(activity);
    const { panels, walk } = renderedPanels(normalizeQuery({ source: "session" }), { SessionActivity: activity });
    const panel = panels.at(-1);
    const table = walk(panel).find((node) => node.className === "activity-table");
    const note = walk(panel).find((node) => node.attributes.id === "activity-summary-note");
    const [total, average] = activityFooter(panel, walk);
    const text = (node) => typeof node === "string" ? node : node instanceof Element ? node.children.map(text).join("") : "";
    assert.equal(table.attributes["aria-describedby"], note.attributes.id);
    assert.match(text(note), /session can be counted on multiple days/);
    assert.match(text(note), /outside and nonexistent hours are excluded/);
    assert.equal(total.children.length, 25);
    assert.equal(average.children.length, 25);
    assert.equal(total.children[0].children[0], "Total");
    assert.equal(average.children[0].children[0], "Average");
    assert.equal(total.children[0].attributes.scope, "row");
    assert.equal(average.children[0].attributes.scope, "row");
    assert.equal(text(total.children[1]), "6*");
    assert.equal(text(average.children[1]), "2*");
    assert.match(average.children[1].attributes["aria-label"], /Average for 00:00: 2 sessions per covered day; 3 covered days; includes partially covered hours/);
    assert.equal(text(total.children[2]), "2");
    assert.equal(text(average.children[2]), "2");
    assert.match(average.children[2].attributes["aria-label"], /1 covered day/);
    assert.equal(text(total.children[3]), "–");
    assert.equal(text(average.children[3]), "–");
    assert.match(total.children[3].attributes["aria-label"], /Total for 02:00: no covered days on this page/);
    assert.equal(text(total.children[4]), new Intl.NumberFormat().format(18014398509481982n));
    assert.ok(total.children[1].className.includes("activity-summary-cell"));
    assert.ok(total.children[1].className.includes("activity-level-1"));
    assert.ok(total.children[1].className.includes("activity-partial"));
    assert.ok(total.children[4].className.includes("activity-level-4"));
    assert.ok(total.children[6].className.includes("activity-level-0"));
    assert.ok(average.children[4].className.includes("activity-level-4"));
    const decimalSuffix = new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 }).formatToParts(0.67)
        .filter((part) => part.type === "decimal" || part.type === "fraction")
        .map((part) => part.value).join("");
    assert.equal(text(average.children[4]), `${new Intl.NumberFormat().format(6004799503160660n)}${decimalSuffix}`);
    assert.equal(text(average.children[5]), new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 }).format(1 / 3));
    assert.deepEqual(activity, before);
});

test("session activity footer follows the visible page and its coverage", () => {
    const hours = (value) => [value, ...Array(23).fill(0)];
    const partial = ["partial", ...Array(23).fill("full")];
    const activity = {
        ...activityDataset(),
        Days: [
            { Date: "2026-01-01", Hours: hours(4) },
            { Date: "2026-01-31", Hours: hours(5) },
            { Date: "2026-03-02", Hours: hours(6) },
        ],
        Coverage: [{ Date: "2026-03-02", Hours: partial }],
    };
    const { content, walk } = interactiveFusion({ SessionActivity: activity });
    const text = (node) => typeof node === "string" ? node : node instanceof Element ? node.children.map(text).join("") : "";
    const values = () => activityFooter(content, walk).map((row) => text(row.children[1]));
    const earlier = () => walk(content).find((node) => node.attributes["data-activity-page"] === "earlier");
    const later = () => walk(content).find((node) => node.attributes["data-activity-page"] === "later");
    const decimal = new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 });
    assert.deepEqual(values(), ["6*", `${decimal.format(6 / 30)}*`]);
    const total = () => activityFooter(content, walk)[0];
    assert.ok(total().children[1].className.includes("activity-level-4"));
    earlier().events.click();
    assert.deepEqual(values(), ["5", decimal.format(5 / 30)]);
    assert.ok(total().children[1].className.includes("activity-level-4"));
    earlier().events.click();
    assert.deepEqual(values(), ["4", "2"]);
    later().events.click();
    later().events.click();
    assert.deepEqual(values(), ["6*", `${decimal.format(6 / 30)}*`]);
});

test("session activity counts selected Agent-mode permission requests at request time", () => {
    const query = normalizeQuery({ source: "session", filters: { session: { agentMode: ["plan"] } } });
    const activity = {
        ...activityDataset(),
        AgentModes: ["plan"],
        Days: [{ Date: "2026-03-02", Hours: Array.from({ length: 24 }, (_, hour) => hour === 4 ? 1 : 0) }],
    };
    const { panels, walk } = renderedPanels(query, { SessionActivity: activity });
    const panel = panels.find((entry) => walk(entry).some((node) =>
        node.tag === "h2" && node.children[0] === "Session activity"));
    const content = walk(panel);
    const text = (node) => typeof node === "string" ? node : node instanceof Element ? node.children.map(text).join(" ") : "";
    assert.ok(content.some((node) => text(node).includes("matching permission.requested event")));
    const table = content.find((node) => node.tag === "table");
    const row = walk(table).find((node) => node.tag === "tr" &&
        node.children[0]?.children?.[0] === "2026-03-02");
    assert.match(row.children[5].attributes["aria-label"], /1 session with a matching permission request/);
    const [total] = activityFooter(panel, walk);
    assert.equal(total.children[5].children[0], "1");
});

test("session activity describes sandbox-filtered completions and linked Agent modes", () => {
    const query = normalizeQuery({ source: "session", filters: { session: { sandbox: ["applied"], agentMode: ["plan"] } } });
    const activity = {
        ...activityDataset(), AgentModes: ["plan"], SandboxStates: ["applied"],
        Days: [{ Date: "2026-03-02", Hours: Array.from({ length: 24 }, (_, hour) => hour === 4 ? 1 : 0) }],
    };
    const { panels, walk } = renderedPanels(query, { SessionActivity: activity });
    const panel = panels.at(-1);
    const text = (node) => typeof node === "string" ? node : node instanceof Element ? node.children.map(text).join(" ") : "";
    const content = text(panel);
    for (const phrase of ["completed tool execution", "completion timestamp", "Unrecorded outcomes are unknown",
        "unfinished calls are excluded", "linked permission request with a selected Agent mode",
        "2 matching completed tool executions", "no matching completed executions recorded"]) {
        assert.ok(content.includes(phrase), phrase);
    }
    const table = walk(panel).find((node) => node.tag === "table");
    const row = walk(table).find((node) => node.tag === "tr" &&
        node.children[0]?.children?.[0] === "2026-03-02");
    assert.match(row.children[5].attributes["aria-label"], /1 session with a matching completed tool execution/);
});

test("session activity hides legacy or mismatched Sandbox-filtered results", () => {
    const query = normalizeQuery({ source: "session", filters: { session: { sandbox: ["applied"] } } });
    for (const [states, message] of [
        [undefined, "does not confirm"],
        [[], "no Sandbox filter"],
        [["not-applied"], '"not-applied"'],
    ]) {
        const view = renderedPanels(query, { SessionActivity: { ...activityDataset(), SandboxStates: states } });
        const panel = view.panels.at(-1);
        const text = view.walk(panel).map((node) => typeof node.children?.[0] === "string" ? node.children[0] : "").join(" ");
        assert.ok(text.includes(message), message);
        assert.equal(view.walk(panel).some((node) => node.tag === "table"), false);
    }
    const noFilter = renderedPanels(normalizeQuery({ source: "session" }), {
        SessionActivity: { ...activityDataset(), SandboxStates: ["applied"] },
    });
    assert.equal(noFilter.walk(noFilter.panels.at(-1)).some((node) => node.tag === "table"), false);
    const legacy = renderedPanels(normalizeQuery({ source: "session" }), { SessionActivity: activityDataset() });
    assert.ok(legacy.walk(legacy.panels.at(-1)).some((node) => node.tag === "table"));
    const duplicate = renderedPanels(query, {
        SessionActivity: { ...activityDataset(), SandboxStates: ["applied", "applied"] },
    });
    assert.ok(duplicate.walk(duplicate.panels.at(-1)).some((node) => node.tag === "table"));
});

test("session activity hides data collected for a different or unverified Agent-mode filter", () => {
    const query = normalizeQuery({ source: "session", filters: { session: { agentMode: ["plan"] } } });
    for (const [activity, message] of [
        [{ ...activityDataset(), AgentModes: [] }, "no Agent mode filter"],
        [{ ...activityDataset(), AgentModes: ["interactive"] }, '"interactive"'],
        [{ ...activityDataset(), AgentModes: undefined }, "does not confirm"],
    ]) {
        const { panels, walk } = renderedPanels(query, { SessionActivity: activity });
        const panel = panels.find((entry) => walk(entry).some((node) =>
            node.tag === "h2" && node.children[0] === "Session activity"));
        const text = walk(panel).map((node) => typeof node.children?.[0] === "string" ? node.children[0] : "").join(" ");
        assert.ok(text.includes(message), message);
        assert.equal(walk(panel).some((node) => node.tag === "table"), false);
    }

    const noFilter = renderedPanels(normalizeQuery({ source: "session" }), {
        SessionActivity: { ...activityDataset(), AgentModes: ["plan"] },
    });
    assert.equal(noFilter.walk(noFilter.panels.at(-1)).some((node) => node.tag === "table"), false);

    const duplicateMetadata = renderedPanels(query, {
        SessionActivity: { ...activityDataset(), AgentModes: ["plan", "plan"] },
    });
    assert.ok(duplicateMetadata.walk(duplicateMetadata.panels.at(-1)).some((node) => node.tag === "table"));
});

test("session activity preserves disclosure state and keyboard focus across updates", () => {
    const { content, context, walk, submitted } = interactiveFusion({ SessionActivity: activityDataset() });
    const disclosure = () => walk(content).find((node) => Object.hasOwn(node.attributes ?? {}, "data-activity-disclosure"));
    const summary = () => disclosure().children[0];
    assert.equal(disclosure().open, true);
    disclosure().open = false;
    context.document.activeElement = summary();
    vm.runInContext("render();", context);
    assert.equal(disclosure().open, false);
    assert.equal(context.document.activeElement, summary());
    vm.runInContext("render();", context);
    assert.equal(disclosure().open, false);
    disclosure().open = true;
    vm.runInContext("render();", context);
    assert.equal(disclosure().open, true);
    walk(disclosure().closest(".panel")).find((node) => node.attributes["data-activity-page"] === "earlier").events.click();
    assert.equal(disclosure().open, true);
    assert.deepEqual(submitted, []);
});

test("session activity pages across the entire range without changing the query", () => {
    const data = { SessionActivity: activityDataset() };
    const view = interactiveFusion(data);
    const { content, context, walk, submitted } = view;
    const activity = () => walk(content).find((entry) => entry.tag === "section" &&
        walk(entry).some((node) => node.tag === "h2" && node.children[0] === "Session activity"));
    const button = (direction) => walk(activity()).find((node) => node.attributes["data-activity-page"] === direction);
    const dates = () => walk(activity()).find((node) => node.tag === "tbody").children.map((row) => row.children[0].children[0]);
    assert.equal(dates()[0], "2026-03-03");
    assert.equal(dates().at(-1), "2026-02-02");
    assert.equal(button("later").attributes.disabled, "");
    const first = button("earlier");
    first.closest(".panel").scrollLeft = 47;
    context.document.activeElement = first;
    first.events.click();
    assert.equal(dates()[0], "2026-02-01");
    assert.equal(dates().at(-1), "2026-01-03");
    assert.equal(button("earlier").closest(".panel").scrollLeft, 47);
    assert.equal(context.document.activeElement, button("earlier"));
    button("earlier").events.click();
    assert.deepEqual(dates(), ["2026-01-02", "2026-01-01"]);
    assert.equal(button("earlier").attributes.disabled, "");
    button("later").events.click();
    assert.equal(dates()[0], "2026-02-01");
    assert.equal(dates().at(-1), "2026-01-03");
    assert.deepEqual(submitted, []);
});

test("session activity distinguishes empty, legacy, invalid, and unrelated views", () => {
    const query = normalizeQuery({ source: "session" });
    const text = (node) => typeof node === "string" ? node : node instanceof Element ? node.children.map(text).join(" ") : "";
    for (const [activity, phrase] of [
        [undefined, "Update gh copilot-attendant or choose the source runner"],
        [{ Version: 2 }, "format is unsupported"],
        [{ ...activityDataset(), Days: [{ Date: "2026-03-02", Hours: [2] }] }, "data is invalid"],
        [{ ...activityDataset(), StartDate: "", EndDate: "", EmptyReason: "empty-window", Days: [], Coverage: [] }, "Since must be earlier than Until"],
        [{ ...activityDataset(), StartDate: "", EndDate: "", EmptyReason: "no-dated-events", Days: [], Coverage: [] }, "No dated events"],
    ]) {
        const view = renderedPanels(query, { SessionActivity: activity });
        const content = view.nodes.map(text).join(" ");
        assert.ok(content.includes(phrase), phrase);
        const panel = view.panels.find((entry) => view.walk(entry).some((node) =>
            node.tag === "h2" && node.children[0] === "Session activity"));
        assert.equal(view.panels.at(-1), panel);
        assert.ok(view.walk(panel).some((node) => node.tag === "details"));
        assert.equal(view.walk(panel).some((node) => node.tag === "table"), false);
    }
    const fusion = projectFusion({ ...fusionDataset(), SessionActivity: activityDataset() });
    assert.equal("SessionActivity" in fusion, false);
    for (const [source, data] of [["session", fusion], ["vscode", { SessionActivity: activityDataset() }]]) {
        const view = renderedPanels(normalizeQuery({ source, fusionOnly: source === "session" }), data);
        assert.equal(view.panels.some((entry) => view.walk(entry).some((node) =>
            node.tag === "h2" && node.children[0] === "Session activity")), false);
    }
});

test("session usage notes reflect Agent mode usage filtering", () => {
    const note = (filters, data) => {
        const query = normalizeQuery({ source: "session", filters: { session: filters } });
        const { context } = renderedPanels(query, { UsageSessions: 0 });
        context.probe = data;
        return vm.runInContext("sessionUsageFilterNote(probe)", context);
    };
    assert.equal(note({}, {}), null);
    assert.equal(note({ kind: ["shell"] }, { UsageAgentModes: [] }), "permission filters do not apply");
    assert.equal(note({ agentMode: ["plan"] }, {}),
        "this CLI result does not restrict usage by Agent mode; permission filters do not apply");
    assert.equal(note({ agentMode: ["plan"] }, { UsageAgentModes: ["autopilot"] }),
        "collected with a different Agent mode selection; refresh to restrict usage");
    assert.equal(note({ agentMode: ["plan", "autopilot"] }, { UsageAgentModes: ["autopilot", "plan"] }),
        "limited to sessions with a matching Agent mode request");
    assert.equal(note({ agentMode: ["plan"], kind: ["shell"] }, { UsageAgentModes: ["plan"] }),
        "limited to sessions with a matching Agent mode request; other permission filters do not apply");
});
