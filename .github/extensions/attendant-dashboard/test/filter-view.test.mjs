import { test } from "node:test";
import assert from "node:assert/strict";
import { panelClearPatch } from "../ui/filter-view.mjs";
import { normalizeQuery, FILTERS } from "../lib/query.mjs";
import { Dashboard } from "../lib/dashboard.mjs";
import { vscodeDataset } from "./fixtures/vscode.mjs";
import { readFileSync } from "node:fs";
import vm from "node:vm";
import * as modelView from "../ui/model-view.mjs";
import * as workspaceView from "../ui/workspace-view.mjs";

class Element {
    constructor(tag) {
        this.tag = tag;
        this.children = [];
        this.attributes = {};
        this.events = {};
        this.style = { setProperty() {} };
        this.classList = { add() {} };
    }
    setAttribute(name, value) { this.attributes[name] = value; }
    addEventListener(name, handler) { this.events[name] = handler; }
    append(child) { this.children.push(child); }
}

function renderedPanels(query, data) {
    const source = readFileSync(new URL("../ui/app.js", import.meta.url), "utf8")
        .replace(/^import .*;\n/gm, "").replace(/\nmain\(\);\s*$/, "\n");
    const submitted = [];
    const context = vm.createContext({
        ...modelView, ...workspaceView, panelClearPatch, Intl,
        Node: Element, document: { querySelector: () => ({ content: "test" }), createElement: (tag) => new Element(tag) },
        localStorage: { getItem: () => null },
        query, data, submitted,
    });
    vm.runInContext(source, context);
    vm.runInContext("state = { query, results: { vscode: { argv: [] } } }; patchQuery = (patch) => submitted.push(patch);", context);
    const nodes = vm.runInContext(query.source === "vscode" ? "renderVscode(data)" : "renderSession(data)", context);
    const walk = (node) => node instanceof Element ? [node, ...node.children.flatMap(walk)] : [];
    const panels = nodes.flatMap(walk).filter((node) => node.tag === "section");
    return { nodes, panels, submitted, walk };
}

function filteredQuery(source = "vscode") {
    return normalizeQuery({
        source, scope: "cwd", path: "/repo", period: "7d", until: "2026-10-11",
        session: "parent", top: 3,
        filters: {
            vscode: { agent: ["Explore", "executionSubagent"], model: ["a", "b"], tool: ["read", "write"] },
            session: { operation: ["read", "write"], kind: ["shell", "read"], command: ["git", "go"],
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
    const data = { UsageSessions: 0, ByModelUsage: [], ByRoleUsage: [], ByDecisionSource: [], ByCWDUsage: [] };
    const { panels, submitted, walk } = renderedPanels(query, data);
    const expected = {
        Workspaces: "workspace", "Working directories": "workspace",
        "Decision sources": "decisionSource", "Read-only vs read-write": "operation",
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
    assert.deepEqual(titles.slice(workspace, workspace + 4), ["Workspaces", "User questions", "Agent usage", "Permissions"]);
    assert.equal(titles.filter((text) => text === "User questions").length, 1);
    const usageGrid = nodes.find((node) => node?.className === "grid");
    assert.deepEqual(usageGrid.children.filter((node) => node?.tag === "section").map(title).slice(-3),
        ["Workspaces", "User questions", "Agent usage"]);
    const permissions = panels.find((panel) => title(panel) === "Permissions");
    assert.equal(permissions.className, "panel wide");
    assert.equal(permissions.children[0].tag, "header");
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
        assert.equal(titles.filter((title) => title === "Permissions").length, 1);
        assert.equal(titles.filter((title) => title === "User questions").length, data.AskUser ? 1 : 0);
        assert.equal(panels[titles.indexOf("Permissions")].className, "panel wide");
    }
});
