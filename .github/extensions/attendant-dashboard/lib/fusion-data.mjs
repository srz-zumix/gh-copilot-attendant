const METRICS = {
    AIU: false,
    PremiumRequests: false,
    PromptTokens: true,
    UncachedInputTokens: true,
    CacheReadTokens: true,
    CacheWriteTokens: true,
    OutputTokens: true,
    APIDurationMs: false,
};

function invalid(field) {
    throw new Error(`Invalid Fusion-only usage details: ${field}`);
}

function count(value, field) {
    if (!Number.isSafeInteger(value) || value < 0) invalid(field);
    return value;
}

function nonnegative(value, field, integer = false) {
    if (!Number.isFinite(value) || value < 0 || (integer && !Number.isSafeInteger(value))) invalid(field);
    return value;
}

function text(value, field) {
    if (typeof value !== "string") invalid(field);
    return value;
}

function metrics(value, records) {
    if (!value || typeof value !== "object" || Array.isArray(value)) invalid("metrics");
    return Object.fromEntries(Object.entries(METRICS).map(([field, integer]) => {
        const metric = value[field];
        if (!metric || typeof metric !== "object" || Array.isArray(metric)) invalid(field);
        const Total = nonnegative(metric.Total, `${field}.Total`, integer);
        const ObservedRecords = count(metric.ObservedRecords, `${field}.ObservedRecords`);
        const CompleteRecords = count(metric.CompleteRecords, `${field}.CompleteRecords`);
        const ExcludedRecords = count(metric.ExcludedRecords, `${field}.ExcludedRecords`);
        if (CompleteRecords > ObservedRecords || ObservedRecords + ExcludedRecords > records ||
            (field !== "PremiumRequests" && ExcludedRecords !== 0) || (!ObservedRecords && Total !== 0)) {
            invalid(`${field} coverage`);
        }
        return [field, { Total, ObservedRecords, CompleteRecords, ExcludedRecords }];
    }));
}

function groups(value, groupCount, records, name) {
    if (!Array.isArray(value) || value.length > groupCount || groupCount > records) invalid(name);
    const keys = new Set();
    return value.map((entry) => {
        if (!entry || typeof entry !== "object") invalid(name);
        const Key = text(entry.Key, `${name}.Key`);
        if (keys.has(Key)) invalid(`${name} duplicate key`);
        keys.add(Key);
        const Sessions = count(entry.Sessions, `${name}.Sessions`);
        const UsageRecords = count(entry.UsageRecords, `${name}.UsageRecords`);
        if (!Sessions || !UsageRecords || Sessions > UsageRecords || UsageRecords > records) invalid(`${name} counts`);
        return { Key, Sessions, UsageRecords, Metrics: metrics(entry.Metrics, UsageRecords) };
    });
}

function phase(entry, records) {
    if (!entry || typeof entry !== "object") invalid("phase");
    const out = {};
    for (const field of ["Pattern", "Phase", "Role", "Model"]) out[field] = text(entry[field], `phase.${field}`);
    for (const field of ["Sessions", "Turns", "Phases", "UsageRecords", "AIURecords", "MissingFusionIDRecords", "MissingPhaseIdentityRecords"]) {
        out[field] = count(entry[field], `phase.${field}`);
    }
    out.AIU = nonnegative(entry.AIU, "phase.AIU");
    if (!out.UsageRecords || !out.Sessions || out.UsageRecords > records || out.Sessions > out.UsageRecords ||
        out.Turns > out.UsageRecords || out.Phases > out.UsageRecords || out.AIURecords > out.UsageRecords ||
        out.MissingFusionIDRecords > out.MissingPhaseIdentityRecords || out.MissingPhaseIdentityRecords > out.UsageRecords ||
        (!out.AIURecords && out.AIU !== 0)) invalid("phase counts");
    return out;
}

/** Returns only usage records explicitly marked Fusion, never the full-session statistics. */
export function projectFusion(data) {
    const fusion = data?.Fusion;
    const dataset = fusion?.Usage;
    if (dataset?.Version !== 1) {
        throw new Error("Fusion-only usage details are unavailable or unsupported; update gh copilot-attendant or choose the source runner");
    }
    const Sessions = count(dataset.Sessions, "Sessions");
    const UsageRecords = count(dataset.UsageRecords, "UsageRecords");
    if (Sessions > UsageRecords || (UsageRecords > 0 && !Sessions)) invalid("session counts");
    const Metrics = metrics(dataset.Metrics, UsageRecords);
    const ModelGroups = count(dataset.ModelGroups, "ModelGroups");
    const CWDGroups = count(dataset.CWDGroups, "CWDGroups");
    const RoleGroups = count(dataset.RoleGroups, "RoleGroups");
    const PhaseGroups = count(dataset.PhaseGroups, "PhaseGroups");
    const usage = {
        Version: 1, Sessions, UsageRecords, Metrics,
        ByModel: groups(dataset.ByModel, ModelGroups, UsageRecords, "ByModel"), ModelGroups,
        ByCWD: groups(dataset.ByCWD, CWDGroups, UsageRecords, "ByCWD"), CWDGroups,
        ByRole: groups(dataset.ByRole, RoleGroups, UsageRecords, "ByRole"), RoleGroups,
        PhaseGroups,
    };
    if (!Array.isArray(fusion.ByPhase) || fusion.ByPhase.length > PhaseGroups || PhaseGroups > UsageRecords) invalid("ByPhase");
    const selected = {
        Sessions,
        TurnsWithUsage: count(fusion.TurnsWithUsage, "TurnsWithUsage"),
        PhasesWithUsage: count(fusion.PhasesWithUsage, "PhasesWithUsage"),
        UsageRecords: count(fusion.UsageRecords, "Fusion.UsageRecords"),
        AIU: nonnegative(fusion.AIU, "Fusion.AIU"),
        AIURecords: count(fusion.AIURecords, "Fusion.AIURecords"),
        MissingFusionIDRecords: count(fusion.MissingFusionIDRecords, "MissingFusionIDRecords"),
        MissingPhaseIdentityRecords: count(fusion.MissingPhaseIdentityRecords, "MissingPhaseIdentityRecords"),
        ByPhase: fusion.ByPhase.map((entry) => phase(entry, UsageRecords)),
    };
    if (selected.UsageRecords !== UsageRecords || selected.AIU !== Metrics.AIU.Total ||
        selected.AIURecords !== Metrics.AIU.ObservedRecords || selected.TurnsWithUsage > UsageRecords ||
        selected.PhasesWithUsage > UsageRecords || selected.MissingFusionIDRecords > selected.MissingPhaseIdentityRecords ||
        selected.MissingPhaseIdentityRecords > UsageRecords) invalid("Fusion totals");

    const result = { FusionOnly: true, FusionUsage: usage, Fusion: selected };
    for (const field of ["Since", "Until"]) {
        if (data[field] !== undefined) result[field] = text(data[field], field);
    }
    if (data.Scope !== undefined) {
        if (!data.Scope || typeof data.Scope !== "object" || Array.isArray(data.Scope)) invalid("Scope");
        result.Scope = {
            ExactCWD: text(data.Scope.ExactCWD, "Scope.ExactCWD"),
            UnderDir: text(data.Scope.UnderDir, "Scope.UnderDir"),
        };
    }
    return result;
}
