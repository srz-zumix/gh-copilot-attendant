import { RE2JS } from "../vendor/re2js.mjs";
import { buildArgv } from "./query.mjs";

export function vscodeFetchArgv(query) {
    return [
        ...buildArgv({
            ...query, top: 0,
            filters: { ...query.filters, vscode: { ...query.filters.vscode, agent: [] } },
        }, "vscode"),
        "--include-log-stats",
    ];
}

function number(row, field, integer = false) {
    const value = row[field];
    if (!Number.isFinite(value) || (integer && (!Number.isSafeInteger(value) || value < 0))) {
        throw new Error(`Invalid VS Code log detail: ${field}`);
    }
    return value;
}

function ordered(entries, count, top) {
    const rows = [...entries.values()].sort((a, b) =>
        count(b) - count(a) || Buffer.compare(Buffer.from(a.Key), Buffer.from(b.Key)));
    return top > 0 ? rows.slice(0, top) : rows;
}

function emptyQuestions() {
    return {
        Sessions: 0, Calls: 0, Answered: 0, Skipped: 0, Cancelled: 0, Failed: 0, Unknown: 0,
        TimedAnswers: 0, UntimedAnswers: 0, TotalWaitMs: 0, AvgWaitMs: null, MaxWaitMs: null,
    };
}

function addQuestions(target, row) {
    if (!row || typeof row !== "object") throw new Error("VS Code question log details are unavailable; update gh copilot-attendant");
    for (const field of ["Calls", "Answered", "Skipped", "Cancelled", "Failed", "Unknown", "TimedAnswers", "UntimedAnswers"]) {
        target[field] += number(row, field, true);
    }
    if (row.Calls !== row.Answered + row.Skipped + row.Cancelled + row.Failed + row.Unknown ||
        row.Answered !== row.TimedAnswers + row.UntimedAnswers) {
        throw new Error("Invalid VS Code question outcome counts");
    }
    const total = number(row, "TotalWaitMs");
    if (total < 0 || (!row.TimedAnswers && (total !== 0 || row.MaxWaitMs !== null || row.AvgWaitMs !== null))) {
        throw new Error("Invalid VS Code question timing");
    }
    target.TotalWaitMs += total;
    if (row.TimedAnswers) {
        const max = number(row, "MaxWaitMs");
        const avg = number(row, "AvgWaitMs");
        if (max < 0 || avg < 0) throw new Error("Invalid VS Code question timing");
        target.MaxWaitMs = target.MaxWaitMs === null ? max : Math.max(target.MaxWaitMs, max);
    }
}

/** Projects normalized log data, deduplicating logs and parent chats before applying Top. */
export function projectVscode(data, query) {
    const dataset = data?.LogDetails;
    if (dataset?.Version !== 1 || !Array.isArray(dataset.Sessions)) {
        throw new Error("VS Code log details are unavailable or unsupported; update gh copilot-attendant");
    }
    const patterns = query.filters.vscode.agent.map((pattern) => RE2JS.compile(pattern));
    const matches = (name) => !patterns.length || patterns.some((pattern) => pattern.matcher(name).find());
    const result = {
        Sessions: 0, Turns: 0, LLMRequests: 0, ToolCalls: 0,
        Since: data.Since, Until: data.Until, Scope: data.Scope,
    };
    if (data.UserQuestions != null) result.UserQuestions = emptyQuestions();
    const tools = new Map();
    const models = new Map();
    const agents = new Map();
    const workspaces = new Map();
    for (const session of dataset.Sessions) {
        if (typeof session.ID !== "string" || typeof session.Workspace !== "string" || !Array.isArray(session.Logs)) {
            throw new Error("Invalid VS Code session detail");
        }
        const seenLogs = new Set();
        const seenInvocations = new Set();
        let counted = false;
        let questionSession = false;
        const ws = {
            Key: session.Workspace, Sessions: 0, Turns: 0, LLMRequests: 0, ToolCalls: 0,
            InputTokens: 0, OutputTokens: 0, CachedTokens: 0, UsageAIU: 0,
        };
        for (const log of session.Logs) {
            if (typeof log.ID !== "string" || typeof log.Main !== "boolean" ||
                !Array.isArray(log.Agents) || !log.Agents.every((name) => typeof name === "string") ||
                !Array.isArray(log.ByTool) || !Array.isArray(log.ByModel)) {
                throw new Error("Invalid VS Code log detail");
            }
            if (seenLogs.has(log.ID) || (patterns.length && (log.Main || !log.Agents.some(matches)))) continue;
            seenLogs.add(log.ID);
            const turns = number(log, "Turns", true);
            const calls = number(log, "ToolCalls", true);
            const requests = number(log, "LLMRequests", true);
            result.Turns += turns;
            result.ToolCalls += calls;
            result.LLMRequests += requests;
            ws.Turns += turns;
            ws.ToolCalls += calls;
            ws.LLMRequests += requests;
            counted ||= turns + calls + requests > 0;
            if (result.UserQuestions) {
                addQuestions(result.UserQuestions, log.UserQuestions);
                questionSession ||= log.UserQuestions.Calls > 0;
            }
            for (const row of log.ByTool) {
                if (typeof row.Key !== "string") throw new Error("Invalid VS Code tool key");
                const target = tools.get(row.Key) ?? { Key: row.Key, Total: 0, OK: 0, Error: 0, TotalMs: 0, AvgMs: 0 };
                for (const field of ["Total", "OK", "Error"]) target[field] += number(row, field, true);
                target.TotalMs += number(row, "TotalMs");
                tools.set(row.Key, target);
            }
            for (const row of log.ByModel) {
                if (typeof row.Key !== "string") throw new Error("Invalid VS Code model key");
                const target = models.get(row.Key) ?? {
                    Key: row.Key, Requests: 0, InputTokens: 0, OutputTokens: 0, CachedTokens: 0,
                    AvgTTFTMs: 0, UsageAIU: 0, totalTTFT: 0, samples: 0,
                };
                target.Requests += number(row, "Requests", true);
                for (const field of ["InputTokens", "OutputTokens", "CachedTokens"]) {
                    const value = number(row, field, true);
                    target[field] += value;
                    ws[field] += value;
                }
                const usage = number(row, "UsageAIU");
                target.UsageAIU += usage;
                ws.UsageAIU += usage;
                target.totalTTFT += number(row, "TotalTTFTMs");
                target.samples += number(row, "TTFTSamples", true);
                models.set(row.Key, target);
            }
        }
        for (const invocation of session.Invocations ?? []) {
            if (typeof invocation.ID !== "string" || typeof invocation.Agent !== "string") {
                throw new Error("Invalid VS Code invocation detail");
            }
            if (!matches(invocation.Agent) || (invocation.ID && seenInvocations.has(invocation.ID))) continue;
            if (invocation.ID) seenInvocations.add(invocation.ID);
            const entry = agents.get(invocation.Agent) ?? { Key: invocation.Agent, Total: 0 };
            entry.Total++;
            agents.set(invocation.Agent, entry);
            counted = true;
        }
        if (questionSession) result.UserQuestions.Sessions++;
        if (counted) {
            result.Sessions++;
            const entry = workspaces.get(ws.Key) ?? { ...ws, Sessions: 0, Turns: 0, LLMRequests: 0, ToolCalls: 0, InputTokens: 0, OutputTokens: 0, CachedTokens: 0, UsageAIU: 0 };
            entry.Sessions++;
            for (const field of ["Turns", "LLMRequests", "ToolCalls", "InputTokens", "OutputTokens", "CachedTokens", "UsageAIU"]) {
                entry[field] += ws[field];
            }
            workspaces.set(ws.Key, entry);
        }
    }
    for (const entry of tools.values()) entry.AvgMs = entry.Total ? entry.TotalMs / entry.Total : 0;
    if (result.UserQuestions?.TimedAnswers) {
        result.UserQuestions.AvgWaitMs = result.UserQuestions.TotalWaitMs / result.UserQuestions.TimedAnswers;
    }
    for (const entry of models.values()) {
        entry.AvgTTFTMs = entry.samples ? entry.totalTTFT / entry.samples : 0;
        delete entry.totalTTFT;
        delete entry.samples;
    }
    result.ByTool = ordered(tools, (row) => row.Total, query.top);
    result.ByModel = ordered(models, (row) => row.Requests, query.top);
    result.ByAgent = ordered(agents, (row) => row.Total, query.top);
    result.ByWorkspace = ordered(workspaces, (row) => row.ToolCalls + row.LLMRequests + row.Turns, query.top);
    return result;
}
