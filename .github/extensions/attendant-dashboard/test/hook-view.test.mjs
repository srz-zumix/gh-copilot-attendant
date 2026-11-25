import { test } from "node:test";
import assert from "node:assert/strict";
import { hookActivityView } from "../ui/hook-view.mjs";
import { hookDataset } from "./fixtures/hook.mjs";

test("hook activity preserves separate decisions, output, processing and coverage", () => {
    const raw = hookDataset();
    const activity = hookActivityView(raw);
    assert.equal(activity.invocations, 7);
    assert.deepEqual(activity.events, { permissionRequests: 2, preToolUses: 5 });
    assert.deepEqual(activity.decisions, { allow: 5, deny: 2 });
    assert.deepEqual(activity.outputs, { writeSucceeded: 6, writeFailed: 1 });
    assert.deepEqual(activity.processing, { success: 6, outputError: 1 });
    assert.deepEqual(activity.unidentified, { missing: 1, unmatched: 2, ambiguous: 1, unscopable: 0, unavailable: 0 });
    assert.equal(activity.malformedRecords, 3);
    assert.equal(activity.unsupportedVersions, 1);
    assert.deepEqual(activity.byTool.map((entry) => entry.key), ["view", "bash", "", "Other", "write"]);
    assert.deepEqual(activity.byDecisionTool, {
        allow: [{ key: "view", total: 2 }, { key: "Other", total: 1 }, { key: "", total: 1 }],
        deny: [{ key: "write", total: 1 }],
    });
    assert.match(activity.note, /not permission requests or proof/);
    assert.match(activity.note, /permission filters do not/);
    assert.match(activity.note, /no-decision records are excluded/);
    assert.match(activity.note, /whole time window/);
    assert.match(activity.note, /unscoped totals already include them, while scoped totals exclude them/);
    assert.deepEqual(activity.byTarget.map((entry) => [entry.decision, entry.tool, entry.target, entry.truncated]), [
        ["allow", "view", "/work/src/main.go", false],
        ["allow", "bash", "git status <b>--short</b>", true],
        ["deny", "write", "", false],
    ]);
    assert.match(activity.note, /command, paths, or URL/);
    assert.deepEqual(raw, hookDataset());
    delete raw.Data.ByTarget;
    assert.deepEqual(hookActivityView(raw).byTarget, []);
    raw.State = "partial";
    assert.match(hookActivityView(raw).note, /incomplete/);
});

test("missing, unsafe, unsupported and old datasets are not presented as zero invocations", () => {
    for (const state of ["missing", "unreadable", "unsupported"]) {
        const activity = hookActivityView({ Version: 1, State: state, Data: null });
        assert.equal(activity.state, state);
        assert.equal("invocations" in activity, false);
        assert.match(activity.note, /unavailable|does not establish/);
    }
    assert.equal(hookActivityView(undefined), null);
    assert.equal(hookActivityView(null), null);
    assert.match(hookActivityView({ Version: 2 }).error, /unsupported/);
    const old = hookDataset();
    delete old.Data.ByDecisionTool;
    assert.match(hookActivityView(old).error, /updated gh copilot-attendant.*source runner/);
});

test("an empty current aggregate has empty decision breakdowns", () => {
    const raw = hookDataset();
    Object.assign(raw.Data, {
        Invocations: 0, PermissionRequests: 0, PreToolUses: 0,
        Decisions: { Allow: 0, Deny: 0, None: 0 },
        Outputs: { NotAttempted: 0, WriteSucceeded: 0, WriteFailed: 0 },
        Processing: { Success: 0, InputError: 0, OutputError: 0 },
        ByTool: [], ByDecisionTool: { Allow: [], Deny: [] }, ByTarget: [],
    });
    assert.deepEqual(hookActivityView(raw).byDecisionTool, { allow: [], deny: [] });
});

test("invalid or inconsistent hook aggregates are rejected without inventing counts", () => {
    const mutations = [
        (raw) => { raw.Data.Invocations = -1; },
        (raw) => { raw.Data.Decisions.Allow = 10; },
        (raw) => { raw.Data.Outputs.WriteFailed = 0; },
        (raw) => { raw.Data.ByTool[0].Key = "</dashboard-data>"; },
        (raw) => { raw.Data.ByTool.push({ Key: "view", Total: 1 }); },
        (raw) => { raw.Data.ByTool[0].Total = 8; },
        (raw) => { raw.Data.Unidentified.Missing = null; },
        (raw) => { raw.Data.Attribution = "unknown"; },
        (raw) => { raw.State = "missing"; },
        (raw) => { raw.Data.ByTarget = {}; },
        (raw) => { raw.Data.ByTarget[0].Decision = "none"; },
        (raw) => { raw.Data.ByTarget[0].Tool = "</dashboard-data>"; },
        (raw) => { raw.Data.ByTarget[0].Target = 1; },
        (raw) => { raw.Data.ByTarget[0].Target = "x".repeat(1025); },
        (raw) => { raw.Data.ByTarget[0].Truncated = "no"; },
        (raw) => { raw.Data.ByTarget[0].Total = 0; },
        (raw) => { raw.Data.ByTarget.push({ ...raw.Data.ByTarget[0] }); },
        (raw) => { raw.Data.ByTarget[2].Total = 3; },
        (raw) => { raw.Data.ByDecisionTool = null; },
        (raw) => { delete raw.Data.ByDecisionTool.Allow; },
        (raw) => { raw.Data.ByDecisionTool.Deny = []; },
        (raw) => { raw.Data.ByDecisionTool.Allow.push({ Key: "view", Total: 1 }); },
        (raw) => { raw.Data.ByDecisionTool.Allow[0].Total = 6; },
        (raw) => {
            raw.Data.Decisions.Allow--;
            raw.Data.Decisions.None++;
            raw.Data.Outputs.WriteSucceeded--;
            raw.Data.Outputs.NotAttempted++;
            raw.Data.Processing.Success--;
            raw.Data.Processing.InputError++;
        },
    ];
    for (const mutate of mutations) {
        const raw = hookDataset();
        mutate(raw);
        const activity = hookActivityView(raw);
        assert.match(activity.error, /invalid or unsupported/);
        assert.equal("invocations" in activity, false);
    }
});
