import { test } from "node:test";
import assert from "node:assert/strict";
import { MODEL_COLUMNS, modelColumns, normalizeModelView, promptTokens, sessionModelEntry, sortModels, toggleModelSort, unitCost } from "../ui/model-view.mjs";

const models = [
    { Key: "Zeta", Requests: 2, InputTokens: 200, CachedTokens: 100, OutputTokens: 10, AvgTTFTMs: 10, UsageAIU: 10 },
    { Key: "Alpha", Requests: 10, InputTokens: 100, CachedTokens: 99, OutputTokens: 90, AvgTTFTMs: 100, UsageAIU: 20 },
];
const names = (entries) => entries.map((entry) => entry.Key);

test("each Models column sorts by raw values in both directions without mutating the source", () => {
    const ascending = {
        model: ["Alpha", "Zeta"],
        requests: ["Zeta", "Alpha"],
        tokens: ["Alpha", "Zeta"],
        input: ["Alpha", "Zeta"],
        cached: ["Alpha", "Zeta"],
        output: ["Zeta", "Alpha"],
        cacheHit: ["Zeta", "Alpha"],
        ttft: ["Zeta", "Alpha"],
        aiu: ["Zeta", "Alpha"],
        unit: ["Alpha", "Zeta"],
    };
    for (const column of MODEL_COLUMNS) {
        assert.deepEqual(names(sortModels(models, { unit: "request", sort: column.id, direction: "asc" })), ascending[column.id], column.id);
        assert.deepEqual(names(sortModels(models, { unit: "request", sort: column.id, direction: "desc" })), [...ascending[column.id]].reverse(), column.id);
    }
    assert.deepEqual(names(models), ["Zeta", "Alpha"]);
});

test("unit cost sorting follows the selected denominator", () => {
    assert.equal(unitCost(models[0], "request"), 5);
    assert.equal(unitCost(models[1], "request"), 2);
    assert.deepEqual(names(sortModels(models, { unit: "mtok", sort: "unit", direction: "asc" })), ["Zeta", "Alpha"]);
    assert.deepEqual(names(sortModels(models, { unit: "mtok", sort: "unit", direction: "desc" })), ["Alpha", "Zeta"]);
});

test("undefined ratios stay last in both directions, and zero AIU remains a valid value", () => {
    const entries = [
        { Key: "missing", Requests: 0, InputTokens: 0, UsageAIU: 5 },
        { Key: "paid", Requests: 1, InputTokens: 10, CachedTokens: 5, UsageAIU: 5 },
        { Key: "zero", Requests: 1, InputTokens: 10, CachedTokens: 0, UsageAIU: 0 },
    ];
    for (const sort of ["unit", "cacheHit"]) {
        assert.deepEqual(names(sortModels(entries, { unit: "request", sort, direction: "asc" })), ["zero", "paid", "missing"]);
        assert.deepEqual(names(sortModels(entries, { unit: "request", sort, direction: "desc" })), ["paid", "zero", "missing"]);
    }
    for (const unit of ["request", "mtok"]) {
        assert.equal(unitCost(entries[0], unit), null);
        assert.equal(unitCost(entries[2], unit), 0);
    }
});

test("ties are deterministic by model name regardless of sort direction", () => {
    const entries = [{ Key: "Zeta", Requests: 3 }, { Key: "Alpha", Requests: 3 }];
    for (const direction of ["asc", "desc"]) {
        assert.deepEqual(names(sortModels(entries, { unit: "request", sort: "requests", direction })), ["Alpha", "Zeta"]);
    }
});

test("header clicks select a column's default direction and toggle an active column", () => {
    const view = normalizeModelView();
    assert.deepEqual(toggleModelSort(view, "requests"), { sort: "requests", direction: "asc" });
    assert.deepEqual(toggleModelSort({ ...view, direction: "asc" }, "requests"), { sort: "requests", direction: "desc" });
    for (const column of MODEL_COLUMNS) {
        if (column.id === "requests") continue;
        assert.deepEqual(toggleModelSort(view, column.id), { sort: column.id, direction: column.id === "model" ? "asc" : "desc" });
    }
});

test("stored preferences preserve legacy sort choices and restore column direction", () => {
    for (const sort of ["requests", "aiu", "unit"]) {
        assert.deepEqual(normalizeModelView({ unit: "mtok", sort }), { unit: "mtok", sort, direction: "desc" });
    }
    const view = { unit: "mtok", sort: "cacheHit", direction: "asc" };
    assert.deepEqual(normalizeModelView(JSON.parse(JSON.stringify(view))), view);
    for (const saved of [null, false, "invalid", { unit: "toString", sort: "toString", direction: "invalid" }]) {
        assert.deepEqual(normalizeModelView(saved), { unit: "request", sort: "requests", direction: "desc" });
    }
});

test("CLI model tokens include cache reads and writes exactly once", () => {
    const raw = { Key: "m", Requests: 2, Sessions: 3, AIU: 1, PremiumRequests: 0.5, InputTokens: 10, CacheReadTokens: 80, CacheWriteTokens: 10, OutputTokens: 20 };
    const entry = sessionModelEntry(raw);
    assert.equal(promptTokens(raw), 100);
    assert.equal(entry.InputTokens, 100);
    assert.equal(entry.CachedTokens, 80);
    assert.equal(entry.UncachedInputTokens, 10);
    assert.equal(entry.CacheWriteTokens, 10);
    assert.equal(entry.PremiumRequests, 0.5);
    assert.equal(entry.UsageAIU, 1);
    assert.equal(unitCost(entry, "request"), 0.5);
    assert.equal(unitCost(entry, "mtok"), (1 / 120) * 1e6);
    assert.equal(modelColumns("session").find((column) => column.id === "cacheHit").value(entry), 0.8);
    assert.equal(raw.InputTokens, 10);
    const vscode = { Requests: 2, InputTokens: 100, CachedTokens: 80, OutputTokens: 20, UsageAIU: 1 };
    assert.equal(unitCost(entry, "mtok"), unitCost(vscode, "mtok"));
});

test("all CLI model columns sort by the displayed raw metric in either direction", () => {
    const entries = [
        { Key: "Zeta", Requests: 2, Sessions: 1, PremiumRequests: 1.5, InputTokens: 200, CacheReadTokens: 100, CacheWriteTokens: 20, OutputTokens: 10, AIU: 10 },
        { Key: "Alpha", Requests: 10, Sessions: 3, PremiumRequests: 0.5, InputTokens: 100, CacheReadTokens: 99, CacheWriteTokens: 1, OutputTokens: 90, AIU: 20 },
    ].map(sessionModelEntry);
    const ascending = {
        model: ["Alpha", "Zeta"],
        sessions: ["Zeta", "Alpha"],
        requests: ["Zeta", "Alpha"],
        premium: ["Alpha", "Zeta"],
        tokens: ["Alpha", "Zeta"],
        input: ["Alpha", "Zeta"],
        cached: ["Alpha", "Zeta"],
        cacheWrite: ["Alpha", "Zeta"],
        uncached: ["Alpha", "Zeta"],
        output: ["Zeta", "Alpha"],
        cacheHit: ["Zeta", "Alpha"],
        aiu: ["Zeta", "Alpha"],
        unit: ["Alpha", "Zeta"],
    };
    for (const column of modelColumns("session")) {
        assert.deepEqual(names(sortModels(entries, { unit: "request", sort: column.id, direction: "asc" }, "session")), ascending[column.id], column.id);
        assert.deepEqual(names(sortModels(entries, { unit: "request", sort: column.id, direction: "desc" }, "session")), [...ascending[column.id]].reverse(), column.id);
    }
    assert.deepEqual(names(sortModels(entries, { unit: "mtok", sort: "unit", direction: "asc" }, "session")), ["Zeta", "Alpha"]);
});

test("each source validates its own model sort settings", () => {
    assert.deepEqual(normalizeModelView(undefined, "session"), { unit: "request", sort: "aiu", direction: "desc" });
    assert.equal(normalizeModelView({ sort: "ttft" }, "session").sort, "aiu");
    assert.equal(normalizeModelView({ sort: "cacheWrite" }, "vscode").sort, "requests");
    assert.equal(normalizeModelView({ sort: "cacheWrite" }, "session").sort, "cacheWrite");
    const view = normalizeModelView(undefined, "session");
    assert.deepEqual(toggleModelSort(view, "premium", "session"), { sort: "premium", direction: "desc" });
    assert.deepEqual(toggleModelSort({ ...view, sort: "premium" }, "premium", "session"), { sort: "premium", direction: "asc" });
});
