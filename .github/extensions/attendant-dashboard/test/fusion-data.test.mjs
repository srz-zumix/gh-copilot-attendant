import { test } from "node:test";
import assert from "node:assert/strict";
import { projectFusion } from "../lib/fusion-data.mjs";
import { emptyFusionDataset, fusionDataset } from "./fixtures/fusion.mjs";

test("Fusion projection allows only tagged usage and preserves recorded coverage", () => {
    const input = fusionDataset(1);
    const before = structuredClone(input);
    const result = projectFusion(input);
    assert.deepEqual(input, before);
    assert.equal(result.FusionOnly, true);
    assert.equal(result.FusionUsage.Sessions, 2);
    assert.equal(result.Fusion.Sessions, 2, "resolution-only sessions are not usage sessions");
    assert.equal(result.FusionUsage.UsageRecords, 3);
    assert.deepEqual(result.FusionUsage.Metrics.AIU, {
        Total: 2.000000001, ObservedRecords: 2, CompleteRecords: 2, ExcludedRecords: 0,
    });
    assert.deepEqual(result.FusionUsage.Metrics.PremiumRequests, {
        Total: 0.5, ObservedRecords: 2, CompleteRecords: 2, ExcludedRecords: 1,
    });
    assert.equal(result.FusionUsage.ModelGroups, 2);
    assert.equal(result.FusionUsage.ByModel.length, 1);
    assert.equal(result.FusionUsage.PhaseGroups, 2);
    assert.equal(result.Fusion.ByPhase.length, 1);
    for (const field of ["Sessions", "Requests", "UsageAIU", "UsageSessions", "ByCommand", "AskUser"]) {
        assert.equal(field in result, false, field);
    }
    for (const field of ["ResolvedTurns", "UnidentifiedResolutions", "ByPattern", "Usage"]) {
        assert.equal(field in result.Fusion, false, field);
    }
    assert.equal("Unrelated" in result.FusionUsage, false);
    assert.equal("Unrelated" in result.FusionUsage.ByModel[0], false);
    assert.equal("Unrelated" in result.Fusion.ByPhase[0], false);
});

test("an empty supported dataset succeeds, whereas old or malformed data fail closed", () => {
    const empty = projectFusion(emptyFusionDataset());
    assert.equal(empty.FusionUsage.UsageRecords, 0);
    assert.equal(empty.Fusion.Sessions, 0);
    assert.deepEqual(empty.FusionUsage.ByModel, []);
    assert.throws(() => projectFusion({}), /update gh copilot-attendant/);
    const old = fusionDataset();
    delete old.Fusion.Usage;
    assert.throws(() => projectFusion(old), /update gh copilot-attendant/);
    const nextVersion = fusionDataset();
    nextVersion.Fusion.Usage.Version = 2;
    assert.throws(() => projectFusion(nextVersion), /unsupported/);
    const badCoverage = fusionDataset();
    badCoverage.Fusion.Usage.Metrics.AIU.CompleteRecords = 3;
    assert.throws(() => projectFusion(badCoverage), /AIU coverage/);
    const badExcluded = fusionDataset();
    badExcluded.Fusion.Usage.Metrics.AIU.ExcludedRecords = 1;
    assert.throws(() => projectFusion(badExcluded), /AIU coverage/);
    const badTotals = fusionDataset();
    badTotals.Fusion.AIU = 100;
    assert.throws(() => projectFusion(badTotals), /Fusion totals/);
    const badGroup = fusionDataset();
    badGroup.Fusion.Usage.ByModel[1].Key = "model-a";
    assert.throws(() => projectFusion(badGroup), /duplicate key/);
    const badPhase = fusionDataset();
    badPhase.Fusion.ByPhase[0].MissingPhaseIdentityRecords = 0;
    assert.throws(() => projectFusion(badPhase), /phase counts/);
});
