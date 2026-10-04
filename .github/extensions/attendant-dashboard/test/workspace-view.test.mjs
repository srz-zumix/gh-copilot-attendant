import { test } from "node:test";
import assert from "node:assert/strict";
import { WORKSPACE_UNITS, hasWorkspaceUsage, normalizeWorkspaceView, sortWorkspaces, toggleWorkspaceSort, workspaceColumns, workspaceEntry, workspaceUnitCost } from "../ui/workspace-view.mjs";

const names = (entries) => entries.map((entry) => entry.Key);
const view = (unit, sort = "unit", direction = "asc") => ({ unit, sort, direction });

test("workspace units use sessions, API requests and prompt plus output tokens", () => {
    const cli = workspaceEntry({
        Key: "/repo", Sessions: 4, Requests: 2, PremiumRequests: 999, AIU: 2,
        InputTokens: 20, CacheReadTokens: 70, CacheWriteTokens: 10, OutputTokens: 100,
    }, "session");
    const vscode = workspaceEntry({
        Key: "/repo", Sessions: 4, LLMRequests: 2, ToolCalls: 999, Turns: 999, UsageAIU: 2,
        InputTokens: 100, CachedTokens: 70, OutputTokens: 100,
    }, "vscode");
    for (const entry of [cli, vscode]) {
        assert.equal(workspaceUnitCost(entry, "session"), 0.5);
        assert.equal(workspaceUnitCost(entry, "request"), 1);
        assert.equal(workspaceUnitCost(entry, "mtok"), 10000);
        assert.equal(entry.InputTokens, 100);
    }
    assert.equal(cli.UncachedInputTokens, 20);
    assert.equal(cli.CachedTokens, 70);
    assert.equal(cli.CacheWriteTokens, 10);
    assert.deepEqual(Object.keys(WORKSPACE_UNITS), ["session", "request", "mtok"]);
});

test("missing usage and zero denominators are unavailable, not zero cost", () => {
    for (const entry of [
        workspaceEntry({ Sessions: 1, Requests: 2, InputTokens: 10 }, "session"),
        workspaceEntry({ Sessions: 1, LLMRequests: 2, InputTokens: 10 }, "vscode"),
        workspaceEntry({ Sessions: 0, Requests: 0, AIU: 5 }, "session"),
    ]) {
        for (const unit of Object.keys(WORKSPACE_UNITS)) assert.equal(workspaceUnitCost(entry, unit), null);
    }
    const legacy = workspaceEntry({ AIU: 5, Sessions: 1, PremiumRequests: 100 }, "session");
    assert.equal(legacy.Requests, undefined);
    assert.equal(workspaceUnitCost(legacy, "request"), null);
    assert.equal(workspaceUnitCost(workspaceEntry({ UsageAIU: 1, LLMRequests: 2 }, "vscode"), "session"), null);
    const zero = workspaceEntry({ AIU: 0, Sessions: 1, Requests: 2, InputTokens: 10 }, "session");
    for (const unit of Object.keys(WORKSPACE_UNITS)) assert.equal(workspaceUnitCost(zero, unit), 0);
});

test("all workspace columns sort by raw values in both directions", () => {
    const cli = [
        { Key: "/Zeta", Sessions: 1, Requests: 2, PremiumRequests: 1.5, AIU: 10, InputTokens: 200, CacheReadTokens: 100, CacheWriteTokens: 20, OutputTokens: 10, APIDurationMs: 10 },
        { Key: "/Alpha", Sessions: 3, Requests: 10, PremiumRequests: 0.5, AIU: 20, InputTokens: 100, CacheReadTokens: 99, CacheWriteTokens: 1, OutputTokens: 90, APIDurationMs: 100 },
    ].map((entry) => workspaceEntry(entry, "session"));
    const vscode = [
        { Key: "/Zeta", Sessions: 1, LLMRequests: 2, Turns: 2, ToolCalls: 7, UsageAIU: 10, InputTokens: 200, CachedTokens: 100, OutputTokens: 10 },
        { Key: "/Alpha", Sessions: 3, LLMRequests: 10, Turns: 10, ToolCalls: 5, UsageAIU: 20, InputTokens: 100, CachedTokens: 99, OutputTokens: 90 },
    ].map((entry) => workspaceEntry(entry, "vscode"));
    const alphaFirst = new Set(["workspace", "premium", "tokens", "input", "cached", "cacheWrite", "uncached", "toolCalls", "turnCost", "unit"]);
    for (const [source, entries] of [["session", cli], ["vscode", vscode]]) {
        for (const column of workspaceColumns(source)) {
            const expected = alphaFirst.has(column.id) ? ["/Alpha", "/Zeta"] : ["/Zeta", "/Alpha"];
            assert.deepEqual(names(sortWorkspaces(entries, view("session", column.id), source)), expected, `${source} ${column.id}`);
            assert.deepEqual(names(sortWorkspaces(entries, view("session", column.id, "desc"), source)), [...expected].reverse(), `${source} ${column.id} reversed`);
        }
        assert.deepEqual(names(entries), ["/Zeta", "/Alpha"]);
        assert.deepEqual(names(sortWorkspaces(entries, view("request"), source)), ["/Alpha", "/Zeta"]);
        assert.deepEqual(names(sortWorkspaces(entries, view("mtok"), source)), ["/Zeta", "/Alpha"]);
    }
});

test("unknown unit costs stay last, ties use workspace names, and zero costs sort correctly", () => {
    const entries = [
        { Key: "missing", Sessions: 0, Requests: 0, InputTokens: 0, UsageAIU: 5 },
        { Key: "paid", Sessions: 1, Requests: 1, InputTokens: 10, UsageAIU: 5 },
        { Key: "zero", Sessions: 1, Requests: 1, InputTokens: 10, UsageAIU: 0 },
    ];
    for (const unit of Object.keys(WORKSPACE_UNITS)) {
        assert.deepEqual(names(sortWorkspaces(entries, view(unit), "session")), ["zero", "paid", "missing"]);
        assert.deepEqual(names(sortWorkspaces(entries, view(unit, "unit", "desc"), "session")), ["paid", "zero", "missing"]);
    }
    for (const direction of ["asc", "desc"]) {
        assert.deepEqual(names(sortWorkspaces([{ ...entries[1], Key: "Z" }, { ...entries[1], Key: "A" }], view("session", "unit", direction), "vscode")), ["A", "Z"]);
    }
});

test("workspace preferences validate source columns, persist each unit and toggle column directions", () => {
    for (const source of ["session", "vscode"]) {
        const initial = normalizeWorkspaceView(undefined, source);
        assert.deepEqual(initial, { unit: "session", sort: "aiu", direction: "desc" });
        for (const unit of Object.keys(WORKSPACE_UNITS)) {
            const saved = view(unit, "unit", "asc");
            assert.deepEqual(normalizeWorkspaceView(JSON.parse(JSON.stringify(saved)), source), saved);
        }
        for (const column of workspaceColumns(source)) {
            const next = toggleWorkspaceSort(initial, column.id, source);
            assert.deepEqual(next, { sort: column.id, direction: column.id === "workspace" ? "asc" : column.id === "aiu" ? "asc" : "desc" });
            assert.notEqual(toggleWorkspaceSort({ ...initial, ...next }, column.id, source).direction, next.direction);
        }
        assert.deepEqual(normalizeWorkspaceView({ unit: "toString", sort: "invalid", direction: "invalid" }, source), initial);
    }
    assert.equal(normalizeWorkspaceView({ sort: "premium" }, "vscode").sort, "aiu");
    assert.equal(normalizeWorkspaceView({ sort: "turns" }, "session").sort, "aiu");
});

test("older VS Code workspace activity stays sortable without claiming usage", () => {
    const entries = [{ Key: "/Z", ToolCalls: 5, LLMRequests: 2, Turns: 3 }].map((entry) => workspaceEntry(entry, "vscode"));
    assert.equal(hasWorkspaceUsage(entries), false);
    assert.equal(hasWorkspaceUsage([workspaceEntry({ UsageAIU: 0, Sessions: 1 }, "vscode")]), true);
    assert.deepEqual(workspaceColumns("vscode", false).map((column) => column.id), ["workspace", "toolCalls", "requests", "turns"]);
    const saved = normalizeWorkspaceView({ unit: "request", sort: "aiu" }, "vscode", false);
    assert.deepEqual(saved, { unit: "request", sort: "toolCalls", direction: "desc" });
    assert.deepEqual(sortWorkspaces(entries, saved, "vscode", false), entries);
});
