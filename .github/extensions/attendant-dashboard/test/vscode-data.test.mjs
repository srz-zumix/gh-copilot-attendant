import { test } from "node:test";
import assert from "node:assert/strict";
import { normalizeQuery } from "../lib/query.mjs";
import { projectVscode, vscodeFetchArgv } from "../lib/vscode-data.mjs";
import { vscodeDataset } from "./fixtures/vscode.mjs";

const query = (agent = [], top = 0) => normalizeQuery({ source: "vscode", top, filters: { vscode: { agent } } });

test("fetch arguments ignore Agent and Top without dropping other filters", () => {
    const q = normalizeQuery({ source: "vscode", scope: "all", top: 1, filters: { vscode: { agent: ["Explore"], model: ["a"], tool: ["read"] } } });
    const argv = vscodeFetchArgv(q);
    assert.ok(argv.includes("--include-log-stats"));
    assert.ok(argv.includes("--top=0"));
    assert.ok(argv.includes("--model=a"));
    assert.ok(argv.includes("--tool=read"));
    assert.ok(!argv.some((arg) => arg.startsWith("--agent=")));
});

test("projection preserves totals and averages only positive TTFT samples", () => {
    const data = vscodeDataset();
    const original = structuredClone(data);
    const full = projectVscode(data, query());
    assert.deepEqual([full.Sessions, full.LLMRequests, full.ToolCalls, full.Turns], [2, 7, 7, 6]);
    assert.equal(full.ByModel[0].Requests, 4);
    assert.equal(full.ByModel[0].AvgTTFTMs, 30);
    const selected = projectVscode(data, query(["^Explore$"]));
    assert.deepEqual([selected.Sessions, selected.LLMRequests, selected.ToolCalls, selected.Turns], [2, 4, 4, 3]);
    assert.deepEqual(selected.ByModel.map((row) => row.Key), ["a"]);
    assert.equal(selected.ByModel[0].UsageAIU, 3.5);
    assert.equal(selected.ByModel[0].InputTokens, 150);
    assert.equal(selected.ByModel[0].CachedTokens, 65);
    assert.equal(selected.ByTool[0].AvgMs, 12.5);
    assert.equal(selected.ByTool[0].Error, 1);
    assert.deepEqual(selected.ByAgent, [{ Key: "Explore", Total: 5 }]);
    assert.equal(selected.ByWorkspace[0].Sessions, 1);
    assert.equal(selected.ByWorkspace[0].LLMRequests, 3);
    assert.deepEqual(data, original);
});

test("multiple names deduplicate shared logs and parent chats, then apply Top", () => {
    const full = projectVscode(vscodeDataset(), query(["Explore", "Other", "executionSubagent"]));
    assert.deepEqual([full.Sessions, full.LLMRequests, full.ToolCalls, full.Turns], [2, 5, 5, 4]);
    const limited = projectVscode(vscodeDataset(), query(["Explore", "Other", "executionSubagent"], 1));
    assert.equal(limited.LLMRequests, 5);
    assert.equal(limited.ByModel.length, 1);
    assert.equal(limited.ByModel[0].Requests, 4);
    assert.deepEqual(full.ByAgent, [
        { Key: "Explore", Total: 4 }, { Key: "Other", Total: 1 }, { Key: "executionSubagent", Total: 1 },
    ]);
});

test("Go-style regular expressions match locally; nonmatching agents have no activity", () => {
    for (const pattern of ["(?i)^explore$", "\\A\\QExplore\\E\\z", "(?i:explore)", "^Exp[[:alpha:]]+$", "(?P<name>Explore)"]) {
        assert.equal(projectVscode(vscodeDataset(), query([pattern])).LLMRequests, 4, pattern);
    }
    const empty = projectVscode(vscodeDataset(), query(["^Nobody$"]));
    assert.equal(empty.Sessions, 0);
    assert.deepEqual(empty.ByModel, []);
    assert.throws(() => query(["a{1001}"]), /regular expression/);
});

test("unsupported versions and invalid counters fail explicitly", () => {
    assert.throws(() => projectVscode({}, query()), /update gh/);
    const data = vscodeDataset();
    data.LogDetails.Version = 2;
    assert.throws(() => projectVscode(data, query()), /unsupported/);
    data.LogDetails.Version = 1;
    data.LogDetails.Sessions[0].Logs[0].Turns = -1;
    assert.throws(() => projectVscode(data, query()), /Turns/);
});
