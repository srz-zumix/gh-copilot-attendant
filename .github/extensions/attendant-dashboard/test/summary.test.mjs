import { test } from "node:test";
import assert from "node:assert/strict";
import { summarize, composePrompt } from "../lib/summary.mjs";

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
