import { normalizeTableView, sortTableEntries, toggleTableSort } from "./table-view.mjs";

export const MODEL_UNITS = {
    session: { label: "Session", header: "AIU / session", per: (e) => e.Sessions ?? 0, scale: 1 },
    request: { label: "Request", header: "AIU / request", per: (e) => e.Requests ?? 0, scale: 1 },
    mtok: { label: "1M tokens", header: "AIU / 1M tokens", per: (e) => (e.InputTokens ?? 0) + (e.OutputTokens ?? 0), scale: 1e6 },
};

export function promptTokens(e) {
    return (e.InputTokens ?? 0) + (e.CacheReadTokens ?? 0) + (e.CacheWriteTokens ?? 0);
}

// CLI input excludes cache reads/writes; VS Code input already includes cached tokens.
export function sessionModelEntry(e) {
    return {
        ...e,
        InputTokens: promptTokens(e),
        UncachedInputTokens: e.InputTokens ?? 0,
        CachedTokens: e.CacheReadTokens ?? 0,
        UsageAIU: e.AIU ?? 0,
    };
}

export function unitCost(e, unit, units = MODEL_UNITS) {
    const { per, scale } = units[unit];
    const denominator = per(e);
    return denominator > 0 ? ((e.UsageAIU ?? 0) / denominator) * scale : null;
}

export const MODEL_COLUMNS = [
    { id: "model", label: "Model", value: (e) => String(e.Key), direction: "asc" },
    { id: "requests", label: "Requests", value: (e) => e.Requests ?? 0 },
    { id: "tokens", label: "Tokens", value: (e) => (e.InputTokens ?? 0) + (e.OutputTokens ?? 0) },
    { id: "input", label: "Input", value: (e) => e.InputTokens ?? 0 },
    { id: "cached", label: "Cached", value: (e) => e.CachedTokens ?? 0 },
    { id: "output", label: "Output", value: (e) => e.OutputTokens ?? 0 },
    { id: "cacheHit", label: "Cache hit", value: (e) => (e.InputTokens > 0 ? (e.CachedTokens ?? 0) / e.InputTokens : null) },
    { id: "ttft", label: "Avg TTFT", value: (e) => e.AvgTTFTMs ?? 0 },
    { id: "aiu", label: "AIU", value: (e) => e.UsageAIU ?? 0 },
    { id: "unit", label: "Unit cost", value: (e, unit) => unitCost(e, unit) },
];

const sharedColumns = Object.fromEntries(MODEL_COLUMNS.map((column) => [column.id, column]));
const sessionColumns = [
    sharedColumns.model,
    { id: "sessions", label: "Sessions", value: (e) => e.Sessions ?? 0 },
    { ...sharedColumns.requests, label: "API requests" },
    { id: "premium", label: "Premium req.", value: (e) => e.PremiumRequests ?? 0 },
    sharedColumns.tokens,
    { ...sharedColumns.input, label: "Prompt" },
    { ...sharedColumns.cached, label: "Cache read" },
    { id: "cacheWrite", label: "Cache write", value: (e) => e.CacheWriteTokens ?? 0 },
    { id: "uncached", label: "Uncached", value: (e) => e.UncachedInputTokens ?? 0 },
    sharedColumns.output,
    sharedColumns.cacheHit,
    sharedColumns.aiu,
    sharedColumns.unit,
];

export function modelColumns(source = "vscode") {
    return source === "session" ? sessionColumns : MODEL_COLUMNS;
}

export function normalizeModelView(saved, source = "vscode") {
    return normalizeTableView(saved, {
        units: MODEL_UNITS,
        columns: modelColumns(source),
        defaultUnit: "request",
        defaultSort: source === "session" ? "aiu" : "requests",
    });
}

export function toggleModelSort(view, columnId, source = "vscode") {
    return toggleTableSort(view, columnId, modelColumns(source));
}

export function sortModels(entries, view, source = "vscode") {
    return sortTableEntries(entries, view, modelColumns(source));
}
