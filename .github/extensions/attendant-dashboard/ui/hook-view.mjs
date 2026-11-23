const NOTE = "Counts are recorded allow and deny hook decisions, not permission requests or proof that the CLI accepted a decision; missed hooks, no-decision invocations, and failed logging are not counted. Older no-decision records are excluded without changing the log. Session and directory scope and record-time bounds apply; permission filters do not. Top limits each tool breakdown and target rows. Allow and deny records include the command, paths, or URL (truncated to fit); older records show the target as not recorded. Unidentified counts cover the whole time window; unscoped totals already include them, while scoped totals exclude them. Malformed and unsupported records are counted across the file. Attribution uses readable session metadata only.";
const INVALID = "Hook activity data is invalid or unsupported; update the dashboard and gh copilot-attendant.";
const OLD_CLI = "Hook tool shares require an updated gh copilot-attendant. Update the CLI or use the source runner.";
const UNAVAILABLE = {
    missing: "The hook log does not exist. This does not establish that no hooks ran.",
    unreadable: "The hook log is unsafe or cannot be read. Invocation counts are unavailable.",
    unsupported: "Private hook logging is unsupported in this build. Invocation counts are unavailable.",
};

function isCount(value) {
    return Number.isSafeInteger(value) && value >= 0;
}

function counts(value, fields) {
    if (!value || typeof value !== "object" || Array.isArray(value)) return null;
    if (!fields.every((field) => isCount(value[field]))) return null;
    return Object.fromEntries(fields.map((field) => [field[0].toLowerCase() + field.slice(1), value[field]]));
}

const TOOL = /^[A-Za-z0-9_.-]{1,128}$/;
const MAX_TARGET_LENGTH = 1024;

function tools(raw, total) {
    if (!Array.isArray(raw)) return null;
    const seen = new Set();
    let listed = 0;
    const rows = [];
    for (const entry of raw) {
        if (!entry || typeof entry.Key !== "string" ||
            (entry.Key !== "" && !TOOL.test(entry.Key)) ||
            seen.has(entry.Key) || !isCount(entry.Total) || entry.Total === 0) return null;
        seen.add(entry.Key);
        listed += entry.Total;
        if (!Number.isSafeInteger(listed) || listed > total) return null;
        rows.push({ key: entry.Key, total: entry.Total });
    }
    return total > 0 && rows.length === 0 ? null : rows;
}

// ByTarget is optional because older CLI builds do not report it.
function targets(raw, decisions) {
    if (raw === undefined) return [];
    if (!Array.isArray(raw)) return null;
    const seen = new Set();
    const listed = { allow: 0, deny: 0 };
    const rows = [];
    for (const entry of raw) {
        if (!entry || typeof entry !== "object" || Array.isArray(entry) ||
            !Object.hasOwn(listed, entry.Decision) ||
            typeof entry.Tool !== "string" || (entry.Tool !== "" && !TOOL.test(entry.Tool)) ||
            typeof entry.Target !== "string" || entry.Target.length > MAX_TARGET_LENGTH ||
            typeof entry.Truncated !== "boolean" || !isCount(entry.Total) || entry.Total === 0) return null;
        const key = JSON.stringify([entry.Decision, entry.Tool, entry.Target, entry.Truncated]);
        if (seen.has(key)) return null;
        seen.add(key);
        listed[entry.Decision] += entry.Total;
        if (listed[entry.Decision] > decisions[entry.Decision]) return null;
        rows.push({ decision: entry.Decision, tool: entry.Tool, target: entry.Target, truncated: entry.Truncated, total: entry.Total });
    }
    return rows;
}

/** Validates and projects the versioned hook aggregate for both the UI and summaries. */
export function hookActivityView(raw) {
    if (raw == null) return null;
    const invalid = () => ({ error: INVALID });
    if (typeof raw !== "object" || Array.isArray(raw) || raw.Version !== 1) return invalid();
    if (Object.hasOwn(UNAVAILABLE, raw.State)) {
        return raw.Data === null ? { state: raw.State, note: UNAVAILABLE[raw.State] } : invalid();
    }
    if (!["available", "partial"].includes(raw.State)) return invalid();
    const data = raw.Data;
    if (!data || typeof data !== "object" || Array.isArray(data) ||
        !["Invocations", "PermissionRequests", "PreToolUses", "MalformedRecords", "UnsupportedVersions"].every((key) => isCount(data[key])) ||
        !["available", "unavailable"].includes(data.Attribution)) return invalid();
    if (!Object.hasOwn(data, "ByDecisionTool")) return { error: OLD_CLI };
    const decisions = counts(data.Decisions, ["Allow", "Deny", "None"]);
    const outputs = counts(data.Outputs, ["NotAttempted", "WriteSucceeded", "WriteFailed"]);
    const processing = counts(data.Processing, ["Success", "InputError", "OutputError"]);
    const unidentified = counts(data.Unidentified, ["Missing", "Unmatched", "Ambiguous", "Unscopable", "Unavailable"]);
    if (!decisions || !outputs || !processing || !unidentified ||
        data.PermissionRequests + data.PreToolUses !== data.Invocations ||
        decisions.allow + decisions.deny + decisions.none !== data.Invocations ||
        outputs.notAttempted + outputs.writeSucceeded + outputs.writeFailed !== data.Invocations ||
        processing.success + processing.inputError + processing.outputError !== data.Invocations ||
        decisions.none !== outputs.notAttempted || processing.outputError !== outputs.writeFailed ||
        decisions.none !== 0 || outputs.notAttempted !== 0 || processing.inputError !== 0) return invalid();
    const byTool = tools(data.ByTool, data.Invocations);
    const breakdown = data.ByDecisionTool;
    if (!breakdown || typeof breakdown !== "object" || Array.isArray(breakdown)) return invalid();
    const allow = tools(breakdown.Allow, decisions.allow);
    const deny = tools(breakdown.Deny, decisions.deny);
    if (!byTool || !allow || !deny) return invalid();
    const byTarget = targets(data.ByTarget, decisions);
    if (!byTarget) return invalid();
    return {
        state: raw.State,
        invocations: data.Invocations,
        events: { permissionRequests: data.PermissionRequests, preToolUses: data.PreToolUses },
        decisions: { allow: decisions.allow, deny: decisions.deny },
        outputs: { writeSucceeded: outputs.writeSucceeded, writeFailed: outputs.writeFailed },
        processing: { success: processing.success, outputError: processing.outputError },
        unidentified,
        attribution: data.Attribution,
        malformedRecords: data.MalformedRecords,
        unsupportedVersions: data.UnsupportedVersions,
        byTool,
        byDecisionTool: { allow, deny },
        byTarget,
        note: raw.State === "partial" ? `The log read was incomplete; counts cover only the readable part. ${NOTE}` : NOTE,
    };
}
