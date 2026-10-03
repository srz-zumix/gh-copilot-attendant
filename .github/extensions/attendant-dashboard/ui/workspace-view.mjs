import { MODEL_UNITS, modelColumns, sessionModelEntry, unitCost } from "./model-view.mjs";
import { normalizeTableView, sortTableEntries, toggleTableSort } from "./table-view.mjs";

export const WORKSPACE_UNITS = MODEL_UNITS;

export function workspaceEntry(e, source) {
    return {
        ...(source === "session" ? sessionModelEntry(e) : e),
        UsageAIU: source === "session" ? e.AIU : e.UsageAIU,
        Requests: source === "session" ? e.Requests : e.LLMRequests,
    };
}

export function workspaceUnitCost(e, unit) {
    return e.UsageAIU == null ? null : unitCost(e, unit, WORKSPACE_UNITS);
}

export function hasWorkspaceUsage(entries) {
    return (entries ?? []).some((e) => e.UsageAIU !== undefined);
}

const workspace = { id: "workspace", label: "Workspace", value: (e) => String(e.Key), direction: "asc" };
const sessions = { id: "sessions", label: "Sessions", value: (e) => e.Sessions ?? null };
const turns = { id: "turns", label: "Turns", value: (e) => e.Turns ?? 0 };
const requests = { id: "requests", label: "LLM requests", value: (e) => e.Requests ?? null };
const toolCalls = { id: "toolCalls", label: "Tool calls", value: (e) => e.ToolCalls ?? 0 };
const aiu = { id: "aiu", label: "AIU", value: (e) => e.UsageAIU ?? null };
const unit = { id: "unit", label: "Unit cost", value: workspaceUnitCost };
const activityColumns = [workspace, toolCalls, requests, turns];
const sessionMetrics = Object.fromEntries(modelColumns("session").map((column) => [column.id, column]));
const vscodeMetrics = Object.fromEntries(modelColumns("vscode").map((column) => [column.id, column]));
const sessionColumns = [
    workspace, sessions, { ...requests, label: "API requests" },
    sessionMetrics.premium, sessionMetrics.tokens, sessionMetrics.input,
    sessionMetrics.cached, sessionMetrics.cacheWrite, sessionMetrics.uncached,
    sessionMetrics.output, sessionMetrics.cacheHit,
    { id: "apiDuration", label: "API time", value: (e) => e.APIDurationMs ?? 0 },
    aiu, unit,
];
const vscodeColumns = [
    workspace, sessions, turns, requests, toolCalls,
    vscodeMetrics.tokens, vscodeMetrics.input, vscodeMetrics.cached,
    vscodeMetrics.output, vscodeMetrics.cacheHit, aiu,
    { id: "turnCost", label: "AIU / turn", value: (e) => e.UsageAIU != null && e.Turns > 0 ? e.UsageAIU / e.Turns : null },
    unit,
];

export function workspaceColumns(source, usage = true) {
    return source === "session" ? sessionColumns : usage ? vscodeColumns : activityColumns;
}

export function normalizeWorkspaceView(saved, source, usage = true) {
    return normalizeTableView(saved, {
        units: WORKSPACE_UNITS,
        columns: workspaceColumns(source, usage),
        defaultUnit: "session",
        defaultSort: usage ? "aiu" : "toolCalls",
    });
}

export function toggleWorkspaceSort(view, columnId, source, usage = true) {
    return toggleTableSort(view, columnId, workspaceColumns(source, usage));
}

export function sortWorkspaces(entries, view, source, usage = true) {
    return sortTableEntries(entries, view, workspaceColumns(source, usage));
}
