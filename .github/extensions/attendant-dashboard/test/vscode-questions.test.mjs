import { test } from "node:test";
import assert from "node:assert/strict";
import { normalizeQuery } from "../lib/query.mjs";
import { projectVscode } from "../lib/vscode-data.mjs";
import { Dashboard } from "../lib/dashboard.mjs";
import { summarize, composePrompt } from "../lib/summary.mjs";
import { vscodeDataset } from "./fixtures/vscode.mjs";

function questions(waits = [], { untimed = 0, skipped = 0, cancelled = 0, failed = 0, unknown = 0 } = {}) {
    const answered = waits.length + untimed;
    const calls = answered + skipped + cancelled + failed + unknown;
    const total = waits.reduce((a, b) => a + b, 0);
    return {
        Sessions: calls ? 1 : 0, Calls: calls, Answered: answered, Skipped: skipped,
        Cancelled: cancelled, Failed: failed, Unknown: unknown, TimedAnswers: waits.length,
        UntimedAnswers: untimed, TotalWaitMs: total,
        AvgWaitMs: waits.length ? total / waits.length : null,
        MaxWaitMs: waits.length ? Math.max(...waits) : null,
    };
}

function dataset() {
    const data = vscodeDataset();
    data.UserQuestions = questions();
    const [a, b] = data.LogDetails.Sessions;
    a.Logs[0].UserQuestions = questions([300]);
    a.Logs[1].UserQuestions = questions([10, 0], { skipped: 1, unknown: 1 });
    a.Logs[2].UserQuestions = questions([100]);
    a.Logs[3].UserQuestions = questions([], { cancelled: 1, failed: 1 });
    b.Logs[0].UserQuestions = questions([], { untimed: 1 });
    b.Logs[1].UserQuestions = questions([20]);
    return data;
}

const query = (agent = [], top = 0) => normalizeQuery({ source: "vscode", top, filters: { vscode: { agent } } });

test("question projection includes zero waits, exact outcome coverage, and parent-session deduplication", () => {
    const data = dataset();
    const before = structuredClone(data);
    const full = projectVscode(data, query()).UserQuestions;
    assert.deepEqual(full, {
        Sessions: 2, Calls: 10, Answered: 6, Skipped: 1, Cancelled: 1, Failed: 1, Unknown: 1,
        TimedAnswers: 5, UntimedAnswers: 1, TotalWaitMs: 430, AvgWaitMs: 86, MaxWaitMs: 300,
    });
    const selected = projectVscode(data, query(["Explore"])).UserQuestions;
    assert.deepEqual(selected, {
        Sessions: 2, Calls: 6, Answered: 4, Skipped: 1, Cancelled: 0, Failed: 0, Unknown: 1,
        TimedAnswers: 4, UntimedAnswers: 0, TotalWaitMs: 130, AvgWaitMs: 32.5, MaxWaitMs: 100,
    });
    assert.deepEqual(data, before);
    data.LogDetails.Sessions[0].Logs.push(structuredClone(data.LogDetails.Sessions[0].Logs[1]));
    assert.deepEqual(projectVscode(data, query(["Explore", "Other"], 1)).UserQuestions, selected);
});

test("no selected questions retains known zero counts, while legacy datasets omit metrics", () => {
    const empty = projectVscode(dataset(), query(["Nobody"])).UserQuestions;
    assert.deepEqual(empty, { ...questions(), Sessions: 0 });
    const legacy = projectVscode(vscodeDataset(), query());
    assert.equal("UserQuestions" in legacy, false);
    assert.equal("userQuestions" in summarize("vscode", legacy), false);
});

test("missing and corrupt question details fail explicitly", () => {
    const missing = dataset();
    delete missing.LogDetails.Sessions[0].Logs[0].UserQuestions;
    assert.throws(() => projectVscode(missing, query()), /question log details/);
    const wrong = dataset();
    wrong.LogDetails.Sessions[0].Logs[0].UserQuestions.Answered = 2;
    assert.throws(() => projectVscode(wrong, query()), /outcome counts/);
    const invalid = dataset();
    invalid.LogDetails.Sessions[0].Logs[0].UserQuestions.MaxWaitMs = NaN;
    assert.throws(() => projectVscode(invalid, query()), /MaxWaitMs/);
});

test("Agent changes and Top keep cached question metrics and summary without launching gh", async () => {
    let calls = 0;
    const dashboard = new Dashboard({
        query: query(), cwd: "/repo",
        deps: {
            resolveRunner: async () => ({ cmd: "fake", prefix: [], label: "fake" }),
            runStats: async () => {
                calls++;
                return { data: dataset(), warnings: [], durationMs: 1 };
            },
        },
    });
    try {
        await dashboard.refresh("vscode");
        const fetched = dashboard.summary("vscode").ranAt;
        dashboard.applyQuery({ top: 1, filters: { vscode: { agent: ["Explore"] } } });
        await dashboard.ensureFresh("vscode");
        assert.equal(calls, 1);
        const summary = dashboard.summary("vscode");
        assert.equal(summary.ranAt, fetched);
        assert.equal(summary.summary.userQuestions.calls, 6);
        assert.equal(summary.summary.userQuestions.avgWaitMs, 32.5);
        dashboard.applyQuery({ filters: { vscode: { agent: [] } } });
        await dashboard.ensureFresh("vscode");
        assert.equal(calls, 1);
        assert.equal(dashboard.summary("vscode").summary.userQuestions.calls, 10);
    } finally {
        dashboard.close();
    }
});

test("VS Code summaries and Ask-agent prompts preserve raw timing, call units, and missing coverage", () => {
    const data = { UserQuestions: questions([10.25, 0], { untimed: 1, skipped: 1, unknown: 1 }) };
    const result = summarize("vscode", data).userQuestions;
    assert.equal(result.calls, 5);
    assert.equal(result.avgWaitMs, 5.125);
    assert.equal(result.maxWaitMs, 10.25);
    assert.equal(result.untimedAnswers, 1);
    assert.equal("unresolved" in result, false);
    assert.match(result.note, /multi-question batches/);
    assert.match(result.note, /In-flight questions are unavailable/);
    const prompt = composePrompt({ source: "vscode", data, query: {}, argv: [], text: "Summarize waits" });
    assert.match(prompt, /"userQuestions"/);
    assert.match(prompt, /"avgWaitMs": 5.125/);
    const unavailable = summarize("vscode", { UserQuestions: questions([], { untimed: 1 }) });
    assert.equal(unavailable.userQuestions.avgWaitMs, null);
    assert.equal(unavailable.userQuestions.maxWaitMs, null);
});
