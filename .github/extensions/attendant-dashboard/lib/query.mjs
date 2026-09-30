// Dashboard query model: validation, normalization, and argv construction for
// `gh copilot-attendant <source> stats`.
//
// Every value is passed as `--flag=value` in a single argv entry and the
// process is spawned without a shell, so no value can be reinterpreted as a
// different flag or as shell syntax. Validation mirrors the CLI's own
// constraints so errors surface in the dashboard before anything is spawned.

export const SOURCES = ["session", "vscode"];
export const SCOPES = ["worktree", "all", "cwd"];
export const OPERATIONS = ["read", "write"];

/** Repeatable filter flags accepted by each source's `stats` command. */
export const FILTERS = {
    session: ["operation", "kind", "command", "path", "url", "decisionSource"],
    vscode: ["tool", "model", "agent"],
};

/** CLI flag names for filters whose query key differs from the flag. */
const FILTER_FLAGS = {
    decisionSource: "decision-source",
};

/** Filters that the CLI interprets as regular expressions. */
const REGEX_FILTERS = new Set(["session.path", "session.url", "vscode.tool", "vscode.model", "vscode.agent"]);

export const MAX_TOP = 1000;
const MAX_VALUE_LENGTH = 512;
const MAX_FILTER_VALUES = 20;
const PERIOD_RE = /^\d+[smhdwy]$/;
const TIME_RE = /^(\d+[smhdwy]|\d{4}-\d{2}-\d{2}(T\d{2}:\d{2}(:\d{2}(\.\d+)?)?(Z|[+-]\d{2}:\d{2}))?)$/;
const SESSION_ID_RE = /^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/;

export class QueryError extends Error {
    constructor(message) {
        super(message);
        this.name = "QueryError";
    }
}

function emptyFilters() {
    return Object.fromEntries(SOURCES.map((s) => [s, Object.fromEntries(FILTERS[s].map((f) => [f, []]))]));
}

/** Returns the query used when nothing has been persisted yet. */
export function defaultQuery() {
    return {
        source: "session",
        scope: "worktree",
        path: "",
        session: "",
        period: "30d",
        since: "",
        until: "",
        top: 10,
        filters: emptyFilters(),
    };
}

function checkString(name, value) {
    if (value === undefined || value === null) return "";
    if (typeof value !== "string") throw new QueryError(`${name} must be a string`);
    const trimmed = value.trim();
    if (trimmed.length > MAX_VALUE_LENGTH) throw new QueryError(`${name} is too long`);
    // Control characters never appear in legitimate paths or patterns.
    if (/[\u0000-\u001f\u007f]/.test(trimmed)) throw new QueryError(`${name} must not contain control characters`);
    return trimmed;
}

function checkFilterValues(source, name, values) {
    const label = `filters.${source}.${name}`;
    if (values === undefined || values === null) return [];
    if (!Array.isArray(values)) throw new QueryError(`${label} must be an array`);
    if (values.length > MAX_FILTER_VALUES) throw new QueryError(`${label} has too many values`);
    const out = [];
    for (const raw of values) {
        const value = checkString(label, raw);
        if (!value) continue;
        if (source === "session" && name === "operation" && !OPERATIONS.includes(value)) {
            throw new QueryError(`${label} must be one of ${OPERATIONS.join(", ")}`);
        }
        if (REGEX_FILTERS.has(`${source}.${name}`)) {
            try {
                new RegExp(value);
            } catch {
                throw new QueryError(`${label} "${value}" is not a valid regular expression`);
            }
        }
        if (!out.includes(value)) out.push(value);
    }
    return out;
}

function checkFilterShape(filters) {
    if (filters === undefined) return {};
    if (filters === null || typeof filters !== "object" || Array.isArray(filters)) {
        throw new QueryError("filters must be an object");
    }
    for (const source of Object.keys(filters)) {
        if (!SOURCES.includes(source)) throw new QueryError(`unknown filter source "${source}"`);
        const group = filters[source];
        if (group === null || typeof group !== "object" || Array.isArray(group)) {
            throw new QueryError(`filters.${source} must be an object`);
        }
        for (const name of Object.keys(group)) {
            if (!FILTERS[source].includes(name)) throw new QueryError(`unknown ${source} filter "${name}"`);
        }
    }
    return filters;
}

/**
 * Merges `patch` onto `base` and validates the result.
 *
 * Filter arrays in a patch replace only the arrays they name; unspecified
 * filters are kept from `base`. Setting one of the mutually exclusive `period`
 * / `since` bounds clears the other. Throws {@link QueryError} on invalid input.
 *
 * @param {object} [patch]
 * @param {object} [base]
 * @returns {object} A fully populated, validated query.
 */
export function normalizeQuery(patch = {}, base = defaultQuery()) {
    if (patch === null || typeof patch !== "object" || Array.isArray(patch)) {
        throw new QueryError("query must be an object");
    }
    const known = new Set(["source", "scope", "path", "session", "period", "since", "until", "top", "filters"]);
    for (const key of Object.keys(patch)) {
        if (!known.has(key)) throw new QueryError(`unknown query field "${key}"`);
    }
    const patchFilters = checkFilterShape(patch.filters);
    const merged = { ...defaultQuery(), ...(base ?? {}), ...patch };
    const q = defaultQuery();

    q.source = merged.source;
    if (!SOURCES.includes(q.source)) throw new QueryError(`source must be one of ${SOURCES.join(", ")}`);

    q.scope = merged.scope;
    if (!SCOPES.includes(q.scope)) throw new QueryError(`scope must be one of ${SCOPES.join(", ")}`);
    q.path = q.scope === "all" ? "" : checkString("path", merged.path);
    if (q.scope === "cwd" && !q.path) throw new QueryError('scope "cwd" requires path');

    q.session = checkString("session", merged.session);
    if (q.session && !SESSION_ID_RE.test(q.session)) throw new QueryError("session must be a session ID");

    q.period = checkString("period", merged.period);
    q.since = checkString("since", merged.since);
    q.until = checkString("until", merged.until);
    if ("since" in patch && q.since && !("period" in patch)) q.period = "";
    if ("period" in patch && q.period && !("since" in patch)) q.since = "";
    if (q.period && q.since) throw new QueryError("period and since are mutually exclusive");
    if (q.period && !PERIOD_RE.test(q.period)) throw new QueryError("period must look like 7d, 3w, 6m, or 1y");
    if (q.since && !TIME_RE.test(q.since)) {
        throw new QueryError("since must be RFC3339, YYYY-MM-DD, or a relative duration like 7d");
    }
    if (q.until && !TIME_RE.test(q.until)) {
        throw new QueryError("until must be RFC3339, YYYY-MM-DD, or a relative duration like 7d");
    }

    const top = merged.top === "" || merged.top === undefined || merged.top === null ? 10 : Number(merged.top);
    if (!Number.isInteger(top) || top < 0 || top > MAX_TOP) {
        throw new QueryError(`top must be an integer between 0 and ${MAX_TOP}`);
    }
    q.top = top;

    const baseFilters = base?.filters ?? {};
    for (const source of SOURCES) {
        for (const name of FILTERS[source]) {
            const values = patchFilters[source]?.[name] ?? baseFilters[source]?.[name];
            q.filters[source][name] = checkFilterValues(source, name, values);
        }
    }
    return q;
}

/**
 * Builds the argv (without the executable prefix) for one source.
 *
 * @param {object} query A query returned by {@link normalizeQuery}.
 * @param {"session"|"vscode"} [source]
 * @returns {string[]}
 */
export function buildArgv(query, source = query.source) {
    if (!SOURCES.includes(source)) throw new QueryError(`source must be one of ${SOURCES.join(", ")}`);
    const argv = [source, "stats"];
    if (query.scope === "all") argv.push("--all");
    else if (query.scope === "cwd") argv.push(`--cwd=${query.path}`);
    else if (query.path) argv.push(`--worktree=${query.path}`);
    if (query.session) argv.push(`--session=${query.session}`);
    if (query.period) argv.push(`--period=${query.period}`);
    if (query.since) argv.push(`--since=${query.since}`);
    if (query.until) argv.push(`--until=${query.until}`);
    for (const name of FILTERS[source]) {
        const flag = FILTER_FLAGS[name] ?? name;
        for (const value of query.filters[source][name]) argv.push(`--${flag}=${value}`);
    }
    argv.push(`--top=${query.top}`, "--format", "json");
    return argv;
}

/** Escapes a literal so it can be used as an anchored regular-expression filter. */
export function exactPattern(value) {
    return `^${String(value).replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}$`;
}
