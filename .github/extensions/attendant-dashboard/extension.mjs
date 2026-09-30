// Extension: attendant-dashboard
//
// Canvas that visualizes local GitHub Copilot usage (Copilot CLI session
// permission requests and usage, and VS Code Copilot Chat tool/model usage) by running
// `gh copilot-attendant <session|vscode> stats --format json`.
//
// This file only wires the canvas to the runtime; see lib/ for the query
// model, command runner, per-instance controller, and HTTP server.

import { joinSession, createCanvas, CanvasError } from "@github/copilot-sdk/extension";
import { Dashboard } from "./lib/dashboard.mjs";
import { QueryError, SOURCES, SCOPES, OPERATIONS, MAX_TOP, normalizeQuery } from "./lib/query.mjs";
import { RUNNERS } from "./lib/runner.mjs";
import { startServer } from "./lib/server.mjs";
import { QueryStore, storeKey } from "./lib/store.mjs";
import { composePrompt } from "./lib/summary.mjs";

const CANVAS_ID = "copilot-attendant-dashboard";
const TITLE = "Copilot attendant";

const stringArray = { type: "array", items: { type: "string" }, maxItems: 20 };
const QUERY_SCHEMA = {
    type: "object",
    additionalProperties: false,
    description: "Partial query; omitted fields keep their current value.",
    properties: {
        source: { type: "string", enum: SOURCES, description: "session = Copilot CLI, vscode = VS Code Copilot Chat" },
        scope: { type: "string", enum: SCOPES, description: "worktree (default: current git worktree), all, or cwd (exact directory)" },
        path: { type: "string", description: "Directory for scope cwd, or worktree path for scope worktree" },
        session: { type: "string", description: "Restrict to one session ID" },
        period: { type: "string", description: "Window back from now such as 7d, 3w, 6m, 1y; empty for none" },
        since: { type: "string", description: "RFC3339, YYYY-MM-DD, or relative duration; clears period" },
        until: { type: "string", description: "RFC3339, YYYY-MM-DD, or relative duration" },
        top: { type: "integer", minimum: 0, maximum: MAX_TOP, description: "Top N entries per statistic (0 = all)" },
        filters: {
            type: "object",
            additionalProperties: false,
            properties: {
                session: {
                    type: "object",
                    additionalProperties: false,
                    properties: {
                        operation: { type: "array", items: { type: "string", enum: OPERATIONS }, maxItems: 2 },
                        kind: stringArray,
                        command: stringArray,
                        path: { ...stringArray, description: "Regular expressions" },
                        url: { ...stringArray, description: "Regular expressions" },
                        decisionSource: {
                            ...stringArray,
                            description: "Exact decision sources, e.g. human_response, unattended_fallback, unknown",
                        },
                    },
                },
                vscode: {
                    type: "object",
                    additionalProperties: false,
                    properties: {
                        tool: { ...stringArray, description: "Regular expressions" },
                        model: { ...stringArray, description: "Regular expressions" },
                        agent: { ...stringArray, description: "Regular expressions" },
                    },
                },
            },
        },
    },
};

const SOURCE_SCHEMA = { type: "string", enum: SOURCES };

const store = new QueryStore();
/** instanceId → { dashboard, server, profile } */
const instances = new Map();

let session;

function log(message, level = "info") {
    Promise.resolve(session?.log(message, { level, ephemeral: true })).catch(() => {});
}

function workspaceCwd() {
    // The extension process runs in the session's working directory, which is
    // what the CLI's default "current worktree" scope resolves against.
    return process.cwd();
}

function requireInstance(instanceId) {
    const entry = instances.get(instanceId);
    if (!entry) throw new CanvasError("canvas_instance_not_found", `no dashboard is open as "${instanceId}"`);
    return entry;
}

function asCanvasError(error) {
    if (error instanceof CanvasError) return error;
    if (error instanceof QueryError) return new CanvasError("invalid_query", error.message);
    return new CanvasError("dashboard_error", error.message);
}

async function createInstance(instanceId, input) {
    const profile = input.profile || "default";
    const key = storeKey(workspaceCwd(), profile);
    const saved = await store.load(key);
    let base;
    try {
        base = normalizeQuery({}, saved ?? undefined);
    } catch {
        base = normalizeQuery({});
    }
    const dashboard = new Dashboard({
        query: base,
        runnerMode: input.runner ?? "auto",
        cwd: workspaceCwd(),
        persist: (query) => store.save(key, query),
        log,
    });
    const server = await startServer({
        dashboard,
        ask: async ({ source, text }) => {
            const result = dashboard.results[source];
            await session.send({
                prompt: composePrompt({ source, query: dashboard.query, argv: result.argv ?? [], data: result.data, text }),
            });
        },
    });
    const entry = { dashboard, server, profile };
    instances.set(instanceId, entry);
    return entry;
}

function applyOpenInput(dashboard, input) {
    if (input.runner) dashboard.setRunnerMode(input.runner);
    const patch = { ...(input.query ?? {}) };
    if (input.source) patch.source = input.source;
    dashboard.applyQuery(patch);
}

const canvas = createCanvas({
    id: CANVAS_ID,
    displayName: "Copilot attendant dashboard",
    description:
        "Dashboard of local Copilot CLI permission requests and usage (AIU, tokens) plus VS Code Copilot Chat tool/model/token usage, powered by gh copilot-attendant.",
    inputSchema: {
        type: "object",
        additionalProperties: false,
        properties: {
            profile: {
                type: "string",
                pattern: "^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$",
                description: "Name under which the last query is remembered for this working directory (default: default)",
            },
            source: { ...SOURCE_SCHEMA, description: "Tab to show first" },
            runner: {
                type: "string",
                enum: RUNNERS,
                description: "auto (installed gh extension, else build from this repo), gh, or source",
            },
            query: QUERY_SCHEMA,
        },
    },
    actions: [
        {
            name: "refresh",
            description:
                "Optionally update the query, re-run the stats command for the active source, and return a compact summary.",
            inputSchema: {
                type: "object",
                additionalProperties: false,
                properties: {
                    query: QUERY_SCHEMA,
                    force: { type: "boolean", description: "Default true; set false to reuse a result that already matches the query" },
                },
            },
            handler: async (ctx) => {
                const { dashboard } = requireInstance(ctx.instanceId);
                try {
                    await dashboard.setQuery(ctx.input?.query ?? {}, { force: ctx.input?.force !== false });
                    return dashboard.summary();
                } catch (error) {
                    throw asCanvasError(error);
                }
            },
        },
        {
            name: "get_data",
            description: "Return the full JSON result last loaded for a source (defaults to the active tab).",
            inputSchema: {
                type: "object",
                additionalProperties: false,
                properties: { source: SOURCE_SCHEMA },
            },
            handler: async (ctx) => {
                const { dashboard } = requireInstance(ctx.instanceId);
                const source = ctx.input?.source ?? dashboard.query.source;
                const result = dashboard.results[source];
                return {
                    source,
                    status: result.status,
                    fresh: dashboard.isFresh(source),
                    command: result.argv ? `gh copilot-attendant ${result.argv.join(" ")}` : null,
                    ranAt: result.ranAt,
                    warnings: result.warnings?.slice(0, 20) ?? [],
                    error: result.error,
                    data: result.data,
                };
            },
        },
        {
            name: "get_summary",
            description: "Return a compact summary of the current result for a source without re-running it.",
            inputSchema: {
                type: "object",
                additionalProperties: false,
                properties: { source: SOURCE_SCHEMA },
            },
            handler: async (ctx) => {
                const { dashboard } = requireInstance(ctx.instanceId);
                return dashboard.summary(ctx.input?.source ?? dashboard.query.source);
            },
        },
        {
            name: "reset_query",
            description: "Reset the query to defaults (current worktree, last 30 days, top 10, no filters) and refresh.",
            inputSchema: {
                type: "object",
                additionalProperties: false,
                properties: { source: SOURCE_SCHEMA },
            },
            handler: async (ctx) => {
                const { dashboard } = requireInstance(ctx.instanceId);
                const defaults = normalizeQuery({ source: ctx.input?.source ?? dashboard.query.source });
                await dashboard.setQuery(defaults, { force: true });
                return dashboard.summary();
            },
        },
    ],
    open: async (ctx) => {
        const input = ctx.input ?? {};
        let entry = instances.get(ctx.instanceId);
        try {
            if (!entry) entry = await createInstance(ctx.instanceId, input);
            applyOpenInput(entry.dashboard, input);
        } catch (error) {
            throw asCanvasError(error);
        }
        entry.dashboard.ensureFresh().catch(() => {});
        return { title: TITLE, url: entry.server.url };
    },
    onClose: async (ctx) => {
        const entry = instances.get(ctx.instanceId);
        if (!entry) return;
        instances.delete(ctx.instanceId);
        entry.dashboard.close();
        await entry.server.close();
    },
});

session = await joinSession({ canvases: [canvas] });
