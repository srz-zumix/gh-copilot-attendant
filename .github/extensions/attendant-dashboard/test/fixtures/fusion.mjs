function metric(Total, ObservedRecords, CompleteRecords = ObservedRecords, ExcludedRecords = 0) {
    return { Total, ObservedRecords, CompleteRecords, ExcludedRecords };
}

function metrics(values) {
    return {
        AIU: metric(...values.aiu),
        PremiumRequests: metric(...values.premium),
        PromptTokens: metric(...values.prompt),
        UncachedInputTokens: metric(...values.input),
        CacheReadTokens: metric(...values.read),
        CacheWriteTokens: metric(...values.write),
        OutputTokens: metric(...values.output),
        APIDurationMs: metric(...values.duration),
    };
}

export function fusionDataset(top = 0) {
    const first = metrics({
        aiu: [2, 1], premium: [0.5, 1, 1, 1], prompt: [100, 2, 1],
        input: [60, 1], read: [30, 1], write: [10, 1], output: [15, 1], duration: [100, 1],
    });
    const second = metrics({
        aiu: [0.000000001, 1], premium: [0, 1], prompt: [20, 1],
        input: [20, 1], read: [0, 1], write: [0, 1], output: [5, 1], duration: [10, 1],
    });
    const totals = metrics({
        aiu: [2.000000001, 2], premium: [0.5, 2, 2, 1], prompt: [120, 3, 2],
        input: [80, 2], read: [30, 2], write: [10, 2], output: [20, 2], duration: [110, 2],
    });
    const rows = (keys) => [
        { Key: keys[0], Sessions: 1, UsageRecords: 2, Metrics: first, Unrelated: "discard" },
        { Key: keys[1], Sessions: 1, UsageRecords: 1, Metrics: second },
    ].slice(0, top || undefined);
    return {
        Since: "2026-01-01T00:00:00Z",
        Until: "2026-02-01T00:00:00Z",
        Scope: { ExactCWD: "", UnderDir: "" },
        Sessions: 9,
        Requests: 99,
        UsageSessions: 9,
        UsageAIU: 123,
        ByCommand: [{ Key: "ordinary-command", Total: 99 }],
        AskUser: { Questions: 12 },
        Fusion: {
            Sessions: 3, ResolvedTurns: 40, UnidentifiedResolutions: 2,
            TurnsWithUsage: 2, PhasesWithUsage: 2, UsageRecords: 3, AIU: 2.000000001, AIURecords: 2,
            MissingFusionIDRecords: 1, MissingPhaseIdentityRecords: 1,
            ByPattern: [{ Key: "resolution-only", ResolvedTurns: 40 }],
            ByPhase: [
                { Pattern: "cascade", Phase: "primary", Role: "solver", Model: "model-a", Sessions: 1,
                    Turns: 1, Phases: 1, UsageRecords: 2, AIU: 2, AIURecords: 1,
                    MissingFusionIDRecords: 1, MissingPhaseIdentityRecords: 1, Unrelated: "discard" },
                { Pattern: "single", Phase: "judge", Role: "judge", Model: "model-b", Sessions: 1,
                    Turns: 1, Phases: 1, UsageRecords: 1, AIU: 0.000000001, AIURecords: 1,
                    MissingFusionIDRecords: 0, MissingPhaseIdentityRecords: 0 },
            ].slice(0, top || undefined),
            Usage: {
                Version: 1, Sessions: 2, UsageRecords: 3, Metrics: totals,
                ByModel: rows(["model-a", "model-b"]), ModelGroups: 2,
                ByCWD: rows(["/fictional/a", "/fictional/b"]), CWDGroups: 2,
                ByRole: rows(["main", "subagent"]), RoleGroups: 2,
                PhaseGroups: 2,
                Unrelated: "discard",
            },
        },
    };
}

export function emptyFusionDataset() {
    const data = fusionDataset();
    const empty = { Total: 0, ObservedRecords: 0, CompleteRecords: 0, ExcludedRecords: 0 };
    const usage = data.Fusion.Usage;
    usage.Sessions = usage.UsageRecords = usage.ModelGroups = usage.CWDGroups = usage.RoleGroups = usage.PhaseGroups = 0;
    usage.Metrics = Object.fromEntries(Object.keys(usage.Metrics).map((field) => [field, { ...empty }]));
    usage.ByModel = usage.ByCWD = usage.ByRole = [];
    Object.assign(data.Fusion, {
        Sessions: 1, ResolvedTurns: 1, TurnsWithUsage: 0, PhasesWithUsage: 0, UsageRecords: 0,
        AIU: 0, AIURecords: 0, MissingFusionIDRecords: 0, MissingPhaseIdentityRecords: 0, ByPhase: [],
    });
    return data;
}
