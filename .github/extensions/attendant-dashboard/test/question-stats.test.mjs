import { test } from "node:test";
import assert from "node:assert/strict";
import { summarize, composePrompt } from "../lib/summary.mjs";

test("question summaries and Ask-agent prompts retain exact recorded timing and outcome coverage", () => {
    const data = { AskUser: {
        Sessions: 1, Questions: 8, Answered: 4, Cancelled: 1, Failed: 1, Unresolved: 1, Unknown: 1,
        TimedAnswers: 3, UntimedAnswers: 1, TotalWaitMs: 654227, AvgWaitMs: 654227 / 3, MaxWaitMs: 452502,
    } };
    const result = summarize("session", data);
    assert.deepEqual(result.askUser, {
        sessions: 1, questions: 8, answered: 4, cancelled: 1, failed: 1, unresolved: 1, unknown: 1,
        timedAnswers: 3, untimedAnswers: 1, totalWaitMs: 654227, avgWaitMs: 654227 / 3, maxWaitMs: 452502,
        note: "Question start-time bounds; permission filters and Top do not apply. Answered paired tool intervals include UI/transport overhead; overlapping waits are summed, not unique wall-clock idle time",
    });
    const prompt = composePrompt({ source: "session", query: {}, argv: [], data, text: "Summarize waiting" });
    assert.match(prompt, /"askUser"/);
    assert.match(prompt, /"totalWaitMs": 654227/);
    assert.match(prompt, /"untimedAnswers": 1/);
});

test("no samples remain unavailable, while absent legacy data does not invent zeros", () => {
    const data = { AskUser: { Questions: 1, Unresolved: 1, TimedAnswers: 0, TotalWaitMs: 0, AvgWaitMs: null, MaxWaitMs: null } };
    const result = summarize("session", data);
    assert.equal(result.askUser.avgWaitMs, null);
    assert.equal(result.askUser.maxWaitMs, null);
    assert.equal(result.askUser.totalWaitMs, 0);
    assert.equal("askUser" in summarize("session", {}), false);
    assert.equal("askUser" in summarize("session", { AskUser: null }), false);
    assert.equal("askUser" in summarize("vscode", data), false);
});
