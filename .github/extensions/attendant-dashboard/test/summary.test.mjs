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
