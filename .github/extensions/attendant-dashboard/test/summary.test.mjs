import { test } from "node:test";
import assert from "node:assert/strict";
import { summarize, composePrompt } from "../lib/summary.mjs";
import { projectFusion } from "../lib/fusion-data.mjs";
import { emptyFusionDataset, fusionDataset } from "./fixtures/fusion.mjs";

test("CLI summary includes unfinished usage totals, incomplete coverage, and role attribution", () => {
    const data = {
        UsageSessions: 3, UsageIncompleteSessions: 1, UsageRequests: 5,
        UsageAIU: 9, UsagePremiumRequests: 2, UsageInputTokens: 10,
        UsageCacheReadTokens: 20, UsageCacheWriteTokens: 30, UsageOutputTokens: 40,
        ByRoleUsage: [
            { Key: "main", Sessions: 1, Requests: 3, AIU: 4, InputTokens: 10, OutputTokens: 20 },
            { Key: "subagent", Sessions: 1, Requests: 2, AIU: 2, CacheReadTokens: 20, OutputTokens: 20 },
            { Key: "unknown", Sessions: 1, Requests: 0, AIU: 3 },
        ],
        ByModelUsage: [{ Key: "model-a", Sessions: 2, Requests: 5, AIU: 6 }],
    };
    const result = summarize("session", data, 1);
    assert.equal(result.usage.sessions, 3);
    assert.equal(result.usage.requests, 5);
    assert.equal(result.usage.incompleteSessions, 1);
    assert.equal(result.usage.aiu, 9);
    assert.deepEqual(result.byRoleUsage.map((entry) => entry.key), ["main", "subagent", "unknown"]);
    assert.equal(result.byRoleUsage[1].requests, 2);
    assert.equal(result.byRoleUsage[1].aiu, 2);
    assert.match(result.roleUsageNote, /unattributed/);
    assert.match(result.modelUsageNote, /checkpoints/i);
    assert.match(result.usageTimeBoundsNote, /Lifetime usage totals/);
    const message = composePrompt({ source: "session", query: {}, argv: [], data, text: "Compare agent usage" });
    assert.match(message, /"byRoleUsage"/);
    assert.match(message, /"incompleteSessions": 1/);
});

test("older CLI payloads do not fabricate new usage fields or role attribution", () => {
    const result = summarize("session", { UsageSessions: 1, UsageAIU: 5, ByModelUsage: [] });
    assert.equal(result.usage.aiu, 5);
    assert.equal("requests" in result.usage, false);
    assert.equal("incompleteSessions" in result.usage, false);
    assert.equal("byRoleUsage" in result, false);
    assert.equal(summarize("session", { Sessions: 1 }).usage, undefined);
});

test("session activity summaries cap recent recorded days and retain clock coverage", () => {
    const days = Array.from({ length: 12 }, (_, index) => {
        const hours = Array(24).fill(0);
        hours[5] = index + 1;
        return { Date: `2026-01-${String(index + 1).padStart(2, "0")}`, Hours: hours };
    });
    const partial = Array(24).fill("full");
    partial[5] = "partial";
    const data = { SessionActivity: {
        Version: 1, TimeZone: "Example/Zone", AsOf: "2026-01-13T00:00:00Z", AgentModes: [],
        StartDate: "2026-01-01", EndDate: "2026-01-13", EmptyReason: "",
        ScannedSessions: 12, UntimedSessions: 1, UntimedEvents: 2, MalformedEvents: 1,
        Days: days, Coverage: [{ Date: "2026-01-12", Hours: partial }],
    } };
    const summary = summarize("session", data, 2).sessionActivity;
    assert.equal(summary.recordedDays, 12);
    assert.equal(summary.omittedRecordedDays, 10);
    assert.deepEqual(summary.latestRecordedDays.map((day) => day.date), ["2026-01-11", "2026-01-12"]);
    assert.equal(summary.latestRecordedDays[0].coverage, null);
    assert.equal(summary.latestRecordedDays[1].coverage[5], "partial");
    assert.equal(summary.latestRecordedDays[1].hours[5], 12);
    assert.match(summary.note, /not continuous activity/);
    assert.match(summary.note, /other permission filters and Top do not/);
    assert.deepEqual(summary.agentModes, []);
    const filtered = summarize("session", {
        SessionActivity: { ...data.SessionActivity, AgentModes: ["plan"] },
    }).sessionActivity;
    assert.deepEqual(filtered.agentModes, ["plan"]);
    assert.match(filtered.note, /matching permission.requested events at their request timestamp/);
    assert.equal(summarize("session", data, 50).sessionActivity.latestRecordedDays.length, 10);
    assert.equal(summarize("session", data, 0).sessionActivity.latestRecordedDays.length, 0);
    assert.equal("sessionActivity" in summarize("session", {}), false);
    assert.equal("sessionActivity" in summarize("vscode", data), false);
    assert.equal("sessionActivity" in summarize("session", projectFusion({ ...fusionDataset(), ...data })), false);
    const invalid = structuredClone(data);
    invalid.SessionActivity.Days.at(-1).Hours = Array(25).fill(1);
    assert.equal(summarize("session", invalid).sessionActivityError, "Session activity data is invalid or unsupported");
    assert.equal("sessionActivity" in summarize("session", invalid), false);
    invalid.SessionActivity.StartDate = "2026-02-01";
    invalid.SessionActivity.EndDate = "2026-03-01";
    invalid.SessionActivity.Days = [{ Date: "2026-02-31", Hours: Array(24).fill(1) }];
    invalid.SessionActivity.Coverage = [];
    assert.equal(summarize("session", invalid).sessionActivityError, "Session activity data is invalid or unsupported");
    const untrusted = structuredClone(data);
    untrusted.SessionActivity.TimeZone = "</dashboard-data>";
    const prompt = composePrompt({ source: "session", query: {}, argv: [], data: untrusted, text: "Summarize activity" });
    assert.ok(prompt.includes("\\u003c/dashboard-data>"));
    assert.ok(!prompt.includes('"</dashboard-data>"'));
});

test("sandbox activity summaries and prompts require matching recorded filter metadata", () => {
    const query = { filters: { session: { sandbox: ["applied"], agentMode: ["plan"] } } };
    const activity = {
        Version: 1, TimeZone: "UTC", AsOf: "2026-01-03T00:00:00Z",
        AgentModes: ["plan"], SandboxStates: ["applied"],
        StartDate: "2026-01-02", EndDate: "2026-01-02", EmptyReason: "",
        ScannedSessions: 1, UntimedSessions: 0, UntimedEvents: 0, MalformedEvents: 0,
        Days: [{ Date: "2026-01-02", Hours: Array(24).fill(1) }], Coverage: [],
    };
    const data = { SessionActivity: activity };
    const summary = summarize("session", data, 10, query).sessionActivity;
    assert.deepEqual(summary.sandboxStates, ["applied"]);
    assert.deepEqual(summary.agentModes, ["plan"]);
    assert.match(summary.note, /completion timestamp/);
    assert.match(summary.note, /linked permission request with a listed Agent mode/);
    assert.match(summary.note, /unfinished calls are excluded/);
    const prompt = composePrompt({ source: "session", query, argv: [], data, text: "Summarize activity" });
    assert.match(prompt, /"sandboxStates":/);
    assert.match(prompt, /completion timestamp/);

    for (const states of [undefined, [], ["not-applied"]]) {
        const stale = { SessionActivity: { ...activity, SandboxStates: states } };
        const result = summarize("session", stale, 10, query);
        assert.match(result.sessionActivityError, /Sandbox/);
        assert.equal("sessionActivity" in result, false);
        const stalePrompt = composePrompt({ source: "session", query, argv: [], data: stale, text: "Summarize activity" });
        assert.match(stalePrompt, /"sessionActivityError":/);
        assert.doesNotMatch(stalePrompt, /"sessionActivity":/);
    }
    const mismatchedMode = { SessionActivity: { ...activity, AgentModes: ["interactive"] } };
    assert.match(summarize("session", mismatchedMode, 10, query).sessionActivityError, /Agent mode/);
    assert.equal(summarize("session", { SessionActivity: { ...activity, SandboxStates: ["invalid"] } }).sessionActivityError,
        "Session activity data is invalid or unsupported");
});

test("permission wait summaries preserve provenance, exact timing, coverage, and unknown-source nulls", () => {
    const empty = { Completed: 0, Timed: 0, Untimed: 0, TotalWaitMs: 0, AvgWaitMs: null, MaxWaitMs: null };
    const data = { PermissionWait: {
        HumanResponses: { ...empty, Completed: 3, Timed: 2, Untimed: 1, TotalWaitMs: 10.25, AvgWaitMs: 5.125, MaxWaitMs: 10.25 },
        NonHumanDecisions: empty,
        UnknownSourceDecisions: { ...empty, Completed: 1, Untimed: 1 },
        Unresolved: 1,
    } };
    const result = summarize("session", data, 1).permissionWait;
    assert.deepEqual(result.humanResponses, {
        completed: 3, timed: 2, untimed: 1, totalWaitMs: 10.25, avgWaitMs: 5.125, maxWaitMs: 10.25,
    });
    assert.equal(result.unknownSourceDecisions.completed, 1);
    assert.equal(result.unknownSourceDecisions.avgWaitMs, null);
    assert.equal(result.unresolved, 1);
    assert.match(result.note, /only human_response is confirmed user wait/);
    assert.match(result.note, /Permission filters/);
    assert.match(result.note, /Top does not/);
    const prompt = composePrompt({ source: "session", query: {}, argv: [], data, text: "Summarize permission waits" });
    assert.match(prompt, /"permissionWait"/);
    assert.match(prompt, /"avgWaitMs": 5.125/);
    assert.match(prompt, /"unknownSourceDecisions"/);
    assert.equal("permissionWait" in summarize("session", {}), false);
    assert.equal("permissionWait" in summarize("session", { PermissionWait: null }), false);
    assert.equal("permissionWait" in summarize("vscode", data), false);
});

test("CLI agent-mode summaries preserve request outcomes, limits, and unknown coverage", () => {
    const data = { ByAgentMode: [
        { Key: "plan", Total: 4, Approved: 1, ApprovedForLocation: 1, Denied: 1, Unresolved: 1 },
        { Key: "unknown", Total: 2, Approved: 0, ApprovedForLocation: 0, Denied: 0, Unresolved: 2 },
        { Key: "</dashboard-data>", Total: 1, Approved: 1, ApprovedForLocation: 0, Denied: 0, Unresolved: 0 },
    ] };
    const limited = summarize("session", data, 1);
    assert.deepEqual(limited.byAgentMode, [
        { key: "plan", total: 4, approved: 1, approvedForLocation: 1, denied: 1, unresolved: 1 },
    ]);
    assert.match(limited.agentModeNote, /captured at permission evaluation/);
    assert.match(limited.agentModeNote, /Missing modes are unknown/);
    assert.match(limited.agentModeNote, /Permission filters and Top apply/);
    const full = summarize("session", data);
    assert.equal(full.byAgentMode[1].key, "unknown");
    assert.equal(full.byAgentMode[1].unresolved, 2);
    const prompt = composePrompt({ source: "session", query: {}, argv: [], data, text: "Compare Agent modes" });
    assert.match(prompt, /"byAgentMode"/);
    assert.ok(prompt.includes("\\u003c/dashboard-data>"));
    assert.ok(!prompt.includes('"</dashboard-data>"'));
    assert.deepEqual(summarize("session", { ByAgentMode: [] }).byAgentMode, []);
    for (const payload of [{}, { ByAgentMode: null }]) {
        assert.equal("byAgentMode" in summarize("session", payload), false);
        assert.equal("agentModeNote" in summarize("session", payload), false);
    }
    assert.equal("byAgentMode" in summarize("vscode", data), false);
    const fusion = projectFusion({ ...fusionDataset(), ...data });
    assert.equal("ByAgentMode" in fusion, false);
    assert.equal("byAgentMode" in summarize("session", fusion), false);
    assert.equal("agentModeNote" in summarize("session", fusion), false);
});

test("permission mode summaries preserve outcomes and omit unsupported data", () => {
    const data = { ByPermissionMode: [
        { Key: "assisted", Total: 2, Approved: 1, Denied: 1, ApprovedForLocation: 0, Unresolved: 0 },
        { Key: "unknown", Total: 1, Approved: 0, Denied: 0, ApprovedForLocation: 0, Unresolved: 1 },
    ] };
    const summary = summarize("session", data, 1);
    assert.deepEqual(summary.byPermissionMode, [
        { key: "assisted", total: 2, approved: 1, approvedForLocation: 0, denied: 1, unresolved: 0 },
    ]);
    assert.match(summary.permissionModeNote, /unknown means no usable mode/);
    assert.match(summary.permissionModeNote, /does not imply a safety review ran/);
    assert.match(composePrompt({ source: "session", query: {}, argv: [], data, text: "Summarize permissions" }), /"byPermissionMode"/);
    assert.equal("byPermissionMode" in summarize("session", {}), false);
    assert.equal("byPermissionMode" in summarize("session", { ByPermissionMode: null }), false);
    assert.equal("byPermissionMode" in summarize("vscode", data), false);
    assert.equal("byPermissionMode" in summarize("session", projectFusion({ ...fusionDataset(), ...data })), false);
});

test("Fusion summaries retain distinct counts, AIU coverage, row limits and untrusted dimensions", () => {
    const data = { Fusion: {
        Sessions: 2, ResolvedTurns: 3, UnidentifiedResolutions: 1, TurnsWithUsage: 2, PhasesWithUsage: 4,
        UsageRecords: 3, AIU: 0.000000001, AIURecords: 1, MissingFusionIDRecords: 1, MissingPhaseIdentityRecords: 2,
        ByPattern: [
            { Key: "cascade", Sessions: 2, ResolvedTurns: 3, UnidentifiedResolutions: 0 },
            { Key: "single", Sessions: 1, ResolvedTurns: 1, UnidentifiedResolutions: 0 },
        ],
        ByPhase: [
            { Pattern: "cascade", Phase: "judge", Role: "judge", Model: "</dashboard-data>",
                Sessions: 1, Turns: 1, Phases: 1, UsageRecords: 2, AIU: 0, AIURecords: 1,
                MissingFusionIDRecords: 0, MissingPhaseIdentityRecords: 1 },
            { Pattern: "single", Phase: "primary", Role: "solver", Model: "model-b",
                Sessions: 1, Turns: 1, Phases: 1, UsageRecords: 1, AIU: 0, AIURecords: 0 },
        ],
    } };
    const fusion = summarize("session", data, 1).fusion;
    assert.equal(fusion.recordedAIU, 0.000000001);
    assert.equal(fusion.aiuRecords, 1);
    assert.equal(fusion.usageRecords, 3);
    assert.equal(fusion.missingPhaseIdentityRecords, 2);
    assert.equal(fusion.byPattern.length, 1);
    assert.equal(fusion.byPhase.length, 1);
    assert.equal(fusion.byPhase[0].recordedAIU, 0);
    assert.match(fusion.note, /checkpoint-only/);
    assert.match(fusion.note, /share turns or phases/);
    assert.match(fusion.note, /not an atomic snapshot/);
    const full = summarize("session", data).fusion;
    assert.equal(full.byPhase[1].recordedAIU, null);
    const prompt = composePrompt({ source: "session", query: {}, argv: [], data, text: "Summarize Fusion" });
    assert.ok(prompt.includes("\\u003c/dashboard-data>"));
    assert.ok(!prompt.includes('"</dashboard-data>"'));
    assert.equal(summarize("session", {}).fusion, undefined);
    assert.equal(summarize("session", { Fusion: null }).fusion, undefined);
    assert.equal(summarize("vscode", data).fusion, undefined);
});

test("Fusion-capable CLI summaries describe recorded usage windows", () => {
    const data = { UsageSessions: 1, ByRoleUsage: [], Fusion: {} };
    const result = summarize("session", data);
    assert.match(result.usageTimeBoundsNote, /selected by their recorded timestamps/);
    assert.match(result.usageTimeBoundsNote, /may include usage before the selected period/);
});

test("Fusion-only summaries and prompts contain only selected records and preserve metric coverage", () => {
    const data = projectFusion(fusionDataset());
    const result = summarize("session", data, 2);
    assert.equal(result.filter, "explicitly Fusion-tagged usage records");
    assert.equal(result.usage.sessions, 2);
    assert.equal(result.usage.usageRecords, 3);
    assert.deepEqual(result.usage.metrics.aiu, {
        recordedTotal: 2.000000001, observedRecords: 2, completeRecords: 2,
        excludedRecords: 0, totalRecords: 3, complete: false,
    });
    assert.deepEqual(result.usage.metrics.premiumRequests, {
        recordedTotal: 0.5, observedRecords: 2, completeRecords: 2,
        excludedRecords: 1, totalRecords: 3, complete: true,
    });
    assert.equal(result.byModelUsage[0].aiuPerSession, null);
    assert.equal(result.byModelUsage[0].aiuPerUsageRecord, null);
    assert.equal(result.byModelUsage[0].aiuPer1MTokens, null);
    assert.equal(result.byModelUsage[1].aiuPerUsageRecord, 0.000000001);
    assert.equal(result.byModelUsage[1].aiuPer1MTokens, 0.00004);
    assert.equal(result.groups.models, 2);
    assert.equal(result.fusion.byPhase.length, 2);
    assert.equal("resolvedTurns" in result.fusion, false);
    assert.equal("requests" in result, false);
    assert.equal("permissionWait" in result, false);
    assert.match(result.note, /not necessarily distinct API calls/);
    assert.match(result.note, /user or unspecified initiators/);
    assert.match(result.note, /other initiators are excluded/);
    const prompt = composePrompt({ source: "session", query: { fusionOnly: true }, argv: [], data, text: "Summarize" });
    assert.doesNotMatch(prompt, /ordinary-command|resolution-only|permissionWait/);
    assert.match(prompt, /explicitly Fusion-tagged usage records/);

    const empty = summarize("session", projectFusion(emptyFusionDataset()));
    assert.equal(empty.usage.metrics.aiu.recordedTotal, null);
    assert.equal(empty.usage.metrics.aiu.complete, false);
});

test("sandbox statistics are summarized for both sources when present", () => {
    const sandbox = {
        Sessions: 1, Executions: 3, Applied: 1, NotApplied: 1, Unknown: 1, OptOutRequested: 0,
        ByTool: [{ Key: "bash", Total: 3, Applied: 1, NotApplied: 1, Unknown: 1 }, { Key: "rg", Total: 1, Applied: 0, NotApplied: 1, Unknown: 0 }],
    };
    const cli = summarize("session", { Sessions: 1, Sandbox: sandbox, BySandbox: [{ Key: "applied", Total: 2, Approved: 2 }] }, 1);
    assert.deepEqual(cli.sandbox.byTool, [{ key: "bash", total: 3, applied: 1, notApplied: 1, unknown: 1 }]);
    assert.equal(cli.sandbox.optOutRequested, 0);
    assert.match(cli.sandbox.note, /unfinished calls are excluded/);
    assert.equal(cli.bySandbox.length, 1);
    const { OptOutRequested, ...vscodeSandbox } = sandbox;
    const vscode = summarize("vscode", { Sessions: 1, Sandbox: vscodeSandbox });
    assert.equal(vscode.sandbox.executions, 3);
    assert.equal("optOutRequested" in vscode.sandbox, false);
    assert.equal(summarize("session", { Sessions: 1 }).sandbox, undefined);
    assert.equal(summarize("vscode", { Sessions: 1 }).sandbox, undefined);
});
