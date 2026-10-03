// Dashboard UI. Talks to the extension's loopback server with a per-instance
// token and receives state pushes over a streamed /events response. All text
// from the server is inserted with textContent (never innerHTML) because keys
// come from local logs.

import { MODEL_UNITS, modelColumns, normalizeModelView, promptTokens, sessionModelEntry, sortModels, toggleModelSort, unitCost } from "./model-view.mjs";
import { WORKSPACE_UNITS, hasWorkspaceUsage, normalizeWorkspaceView, sortWorkspaces, toggleWorkspaceSort, workspaceColumns, workspaceEntry, workspaceUnitCost } from "./workspace-view.mjs";

const token = document.querySelector('meta[name="dashboard-token"]').content;
const $ = (id) => document.getElementById(id);

const LABELS = {
    session: "Copilot CLI",
    vscode: "VS Code",
    filters: {
        operation: "Operation",
        kind: "Kind",
        command: "Command",
        path: "Path",
        url: "URL",
        decisionSource: "Decision source",
        tool: "Tool",
        model: "Model",
        agent: "Agent",
    },
};

/** Filters the CLI matches exactly rather than as regular expressions. */
const EXACT_FILTERS = new Set(["kind", "command", "decisionSource"]);

let state = null;
let meta = null;
let formQueryKey = "";
let filtersKey = "";
let loadingTimer = null;

// ---------------------------------------------------------------- helpers

function h(tag, props, ...children) {
    const el = document.createElement(tag);
    for (const [k, v] of Object.entries(props ?? {})) {
        if (v === null || v === undefined || v === false) continue;
        if (k === "class") el.className = v;
        else if (k === "vars") for (const [name, value] of Object.entries(v)) el.style.setProperty(name, value);
        else if (k.startsWith("on")) el.addEventListener(k.slice(2), v);
        else el.setAttribute(k, v === true ? "" : String(v));
    }
    for (const child of children.flat(Infinity)) {
        if (child === null || child === undefined || child === false) continue;
        el.append(child instanceof Node ? child : String(child));
    }
    return el;
}

const fmt = new Intl.NumberFormat();
// Compact units are pinned to English so they match the rest of the UI.
const compact = new Intl.NumberFormat("en", { notation: "compact", maximumFractionDigits: 1 });

function num(n) {
    return fmt.format(Math.round(n ?? 0));
}

const aiuFmt = new Intl.NumberFormat("en", { minimumFractionDigits: 1, maximumFractionDigits: 1 });

function aiu(n) {
    return aiuFmt.format(n ?? 0);
}

// Premium requests can be fractional (model multipliers such as 0.5x).
const decimalFmt = new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 });

function decimal(n) {
    return decimalFmt.format(n ?? 0);
}

function big(n) {
    return Math.abs(n ?? 0) >= 10_000 ? compact.format(n) : num(n);
}

function pct(part, whole) {
    if (!whole) return "–";
    return `${((part / whole) * 100).toFixed(part / whole < 0.1 ? 1 : 0)}%`;
}

function duration(ms) {
    if (!Number.isFinite(ms)) return "–";
    if (ms < 1000) return `${Math.round(ms)} ms`;
    if (ms < 60_000) return `${(ms / 1000).toFixed(1)} s`;
    if (ms < 3_600_000) return `${(ms / 60_000).toFixed(1)} min`;
    return `${(ms / 3_600_000).toFixed(1)} h`;
}

function exactPattern(value) {
    return `^${String(value).replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}$`;
}

/** Quotes one argument for a POSIX shell so a copied command keeps its argv. */
function shellQuote(arg) {
    const value = String(arg);
    if (/^[A-Za-z0-9_@%+=:,./-]+$/.test(value)) return value;
    return `'${value.replace(/'/g, "'\\''")}'`;
}

function isZeroTime(value) {
    return !value || String(value).startsWith("0001-01-01");
}

async function api(method, path, body) {
    const res = await fetch(path, {
        method,
        headers: { "X-Dashboard-Token": token, ...(body ? { "Content-Type": "application/json" } : {}) },
        body: body ? JSON.stringify(body) : undefined,
    });
    const payload = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(payload.error || `${res.status} ${res.statusText}`);
    return payload;
}

// ---------------------------------------------------------------- query

function activeSource() {
    return state?.query.source ?? "session";
}

async function patchQuery(patch, options = {}) {
    showError(null);
    try {
        const snapshot = await api("POST", "/api/query", { query: patch, ...options });
        setState(snapshot);
    } catch (error) {
        showError(error.message, "client");
    }
}

function addFilter(name, value) {
    const source = activeSource();
    const current = state.query.filters[source][name] ?? [];
    if (current.includes(value)) return;
    patchQuery({ filters: { [source]: { [name]: [...current, value] } } });
}

function removeFilter(name, value) {
    const source = activeSource();
    const current = state.query.filters[source][name] ?? [];
    patchQuery({ filters: { [source]: { [name]: current.filter((v) => v !== value) } } });
}

function readForm() {
    const form = $("query");
    const data = new FormData(form);
    const patch = {
        scope: data.get("scope"),
        path: String(data.get("path") ?? "").trim(),
        period: data.get("period"),
        since: String(data.get("since") ?? "").trim(),
        until: String(data.get("until") ?? "").trim(),
        session: String(data.get("session") ?? "").trim(),
        top: data.get("top") === "" ? 10 : Number(data.get("top")),
    };
    if (patch.since) patch.period = "";
    return patch;
}

function fillForm(query) {
    const key = JSON.stringify([query.scope, query.path, query.period, query.since, query.until, query.session, query.top]);
    if (key === formQueryKey) return;
    formQueryKey = key;
    const form = $("query");
    form.scope.value = query.scope;
    form.path.value = query.path;
    form.path.placeholder = query.scope === "all" ? "not used" : query.scope === "cwd" ? "directory (required)" : state.cwd;
    form.path.disabled = query.scope === "all";
    const period = form.period;
    if (query.period && ![...period.options].some((o) => o.value === query.period)) {
        period.append(h("option", { value: query.period }, query.period));
    }
    period.value = query.period;
    form.since.value = query.since;
    form.until.value = query.until;
    form.session.value = query.session;
    form.top.value = String(query.top);
}

// ---------------------------------------------------------------- rendering

function setState(next) {
    if (next.meta) meta = next.meta;
    state = next;
    render();
}

/** Shows an error banner; client-side errors stay until the next query change. */
function showError(message, kind = "result") {
    const el = $("error");
    el.hidden = !message;
    el.textContent = message ?? "";
    el.dataset.kind = message ? kind : "";
}

function render() {
    if (!state || !meta) return;
    const source = activeSource();
    const result = state.results[source];

    for (const tab of document.querySelectorAll(".tabs button")) {
        const selected = tab.dataset.source === source;
        tab.setAttribute("aria-selected", String(selected));
        const other = state.results[tab.dataset.source];
        tab.title = other.status === "loading" ? "Loading…" : "";
    }

    fillForm(state.query);
    renderFilters(source);
    renderStatus(result);
    renderWarnings(result);
    if (result.status === "error") showError(result.error);
    else if ($("error").dataset.kind !== "client") showError(null);

    const content = $("content");
    content.classList.toggle("loading", result.status === "loading");
    content.replaceChildren(
        ...(result.data
            ? source === "session"
                ? renderSession(result.data)
                : renderVscode(result.data)
            : [h("div", { class: "empty" }, result.status === "loading" ? "Running gh copilot-attendant…" : "No data yet.")]),
    );
    $("refresh").disabled = false;
    $("command").textContent = `gh copilot-attendant ${result.command.map(shellQuote).join(" ")}`;
    renderPresets(source, result);
}

function renderStatus(result) {
    const el = $("status");
    const parts = [];
    clearInterval(loadingTimer);
    loadingTimer = null;
    if (result.status === "loading") {
        const started = result.startedAt ? Date.parse(result.startedAt) : Date.now();
        const label = h("span", null, "Running…");
        parts.push(h("span", { class: "spinner", "aria-hidden": "true" }), label);
        loadingTimer = setInterval(() => {
            label.textContent = `Running… ${Math.round((Date.now() - started) / 1000)}s`;
        }, 1000);
    } else if (result.ranAt) {
        parts.push(`Updated ${new Date(result.ranAt).toLocaleTimeString()} · ${duration(result.durationMs)}`);
        if (!result.fresh) parts.push(" · ", h("span", { class: "stale" }, "query changed — refresh to update"));
    }
    const runner = state.runner;
    if (runner?.label) parts.push(` · ${runner.label}`);
    if (runner?.error) parts.push(" · ", h("span", { class: "stale" }, `runner: ${runner.error}`));
    const data = result.data;
    if (data) {
        const since = isZeroTime(data.Since) ? "beginning" : new Date(data.Since).toLocaleDateString();
        const until = isZeroTime(data.Until) ? "now" : new Date(data.Until).toLocaleDateString();
        parts.push(` · ${since} → ${until}`);
    }
    el.replaceChildren(...parts);
    el.title = el.textContent;
}

function renderWarnings(result) {
    const el = $("warnings");
    const warnings = result.warnings ?? [];
    el.hidden = warnings.length === 0;
    if (!warnings.length) return;
    const counts = new Map();
    for (const w of warnings) counts.set(w, (counts.get(w) ?? 0) + 1);
    el.querySelector("summary").textContent = `${warnings.length} warning${warnings.length === 1 ? "" : "s"} while reading logs`;
    el.querySelector("ul").replaceChildren(...[...counts].map(([w, n]) => h("li", null, n > 1 ? `${w} (×${n})` : w)));
}

function renderFilters(source) {
    const filters = state.query.filters[source];
    // Rebuilding would discard text the user is typing, so only do it on change.
    const key = JSON.stringify([source, filters]);
    if (key === filtersKey) return;
    filtersKey = key;
    const groups = meta.filters[source].map((name) => {
        const values = filters[name] ?? [];
        if (name === "operation") {
            return h(
                "div",
                { class: "filter" },
                h("span", { class: "name" }, LABELS.filters[name]),
                ["read", "write"].map((op) =>
                    h(
                        "label",
                        { class: "toggle" },
                        h("input", {
                            type: "checkbox",
                            checked: values.includes(op),
                            onchange: (e) => (e.target.checked ? addFilter(name, op) : removeFilter(name, op)),
                        }),
                        op,
                    ),
                ),
            );
        }
        const input = h("input", {
            type: "text",
            spellcheck: "false",
            placeholder: EXACT_FILTERS.has(name) ? "exact value" : "regex",
            "aria-label": `Add ${LABELS.filters[name]} filter`,
            onkeydown: (e) => {
                if (e.key !== "Enter") return;
                e.preventDefault();
                const value = e.target.value.trim();
                if (value) addFilter(name, value);
            },
        });
        return h(
            "div",
            { class: "filter" },
            h("span", { class: "name" }, LABELS.filters[name]),
            values.map((value) =>
                h(
                    "span",
                    { class: "chip", title: value },
                    h("span", null, value),
                    h("button", { type: "button", "aria-label": `Remove ${value}`, onclick: () => removeFilter(name, value) }, "×"),
                ),
            ),
            input,
        );
    });
    $("filters").replaceChildren(...groups);
}

function card(label, value, sub) {
    return h("div", { class: "card" }, h("div", { class: "label" }, label), h("div", { class: "value" }, value), sub ? h("div", { class: "sub" }, sub) : null);
}

function legend(items) {
    return h("div", { class: "legend" }, items.map(([label, color]) => h("span", { vars: { "--swatch": color } }, label)));
}

function stackedBar(segments, max) {
    const bar = h("div", { class: "bar" });
    for (const [value, color, title] of segments) {
        if (!(value > 0)) continue;
        const segment = h("i", { title: title || null });
        segment.style.width = `${(value / max) * 100}%`;
        segment.style.background = color;
        bar.append(segment);
    }
    return bar;
}

/** Shortens an absolute path to its last two segments for display. */
function shortPath(value) {
    const parts = String(value).split("/").filter(Boolean);
    return String(value).startsWith("/") && parts.length > 2 ? `…/${parts.slice(-2).join("/")}` : value;
}

function keyCell(key, onPick, pickTitle, display = key) {
    const text = display || "(empty)";
    if (!onPick) return h("span", { class: "key", title: key }, text);
    return h("button", { type: "button", class: "key", title: `${key}\n${pickTitle}`, onclick: () => onPick(key) }, text);
}

function panel(title, hint, body, { wide = false } = {}) {
    return h(
        "section",
        { class: `panel${wide ? " wide" : ""}` },
        h("header", null, h("h2", null, title), hint ? h("span", { class: "hint" }, hint) : null),
        body,
    );
}

// ---------------------------------------------------------------- Copilot CLI

const DECISION_COLORS = [
    ["Approved", "var(--c-approved)", "Approved"],
    ["Approved for location", "var(--c-location)", "ApprovedForLocation"],
    ["Denied", "var(--c-denied)", "Denied"],
    ["Unresolved", "var(--c-unresolved)", "Unresolved"],
];

function decisionRows(entries, { onPick, pickTitle, display } = {}) {
    if (!entries?.length) return h("div", { class: "muted small" }, "No entries.");
    const max = Math.max(...entries.map((e) => e.Total), 1);
    return h(
        "div",
        { class: "rows" },
        entries.map((e) => {
            const breakdown = DECISION_COLORS.map(([label, , field]) => `${label}: ${num(e[field])}`).join("\n");
            const bar = stackedBar(DECISION_COLORS.map(([label, color, field]) => [e[field], color, `${label}: ${num(e[field])}`]), max);
            // The bar is visual only; screen readers get the breakdown next to the total.
            bar.setAttribute("aria-hidden", "true");
            const row = h(
                "div",
                { class: "row", title: breakdown },
                keyCell(e.Key, onPick, pickTitle, display ? display(e.Key) : e.Key),
                bar,
                h("span", { class: "num" }, num(e.Total), h("span", { class: "visually-hidden" }, ` total (${breakdown.replaceAll("\n", ", ")})`)),
            );
            return row;
        }),
    );
}

const USAGE_TOKEN_COLORS = [
    ["Cache read", "var(--c-cached)", "CacheReadTokens"],
    ["Cache write", "var(--c-cache-write)", "CacheWriteTokens"],
    ["Uncached input", "var(--c-input)", "InputTokens"],
    ["Output", "var(--c-output)", "OutputTokens"],
];

function sectionHead(title, hint) {
    return h("div", { class: "section-head" }, h("h2", null, title), hint ? h("span", { class: "hint" }, hint) : null);
}

function renderSessionUsage(data) {
    // Older CLI builds do not report usage; keep the permission-only layout for them.
    if (data.UsageSessions === undefined) return [];
    const sessions = data.UsageSessions ?? 0;
    const total = data.UsageAIU ?? 0;
    const premium = data.UsagePremiumRequests ?? 0;
    const prompt = promptTokens({ InputTokens: data.UsageInputTokens, CacheReadTokens: data.UsageCacheReadTokens, CacheWriteTokens: data.UsageCacheWriteTokens });
    const perSession = (value, format) => (sessions ? `${format(value / sessions)} / session` : null);
    const filtered = Object.values(state.query.filters.session ?? {}).some((values) => values.length);
    const hints = ["from session.shutdown events", filtered ? "permission filters do not apply" : null].filter(Boolean).join(" · ");

    return [
        sectionHead("Usage", hints),
        h(
            "div",
            { class: "cards" },
            card("Sessions with usage", num(sessions)),
            card("Usage (AIU)", aiu(total), perSession(total, aiu)),
            card("Premium requests", decimal(premium), premium ? `${aiu(total / premium)} AIU / request` : null),
            card("Prompt tokens", big(prompt), `${pct(data.UsageCacheReadTokens, prompt)} cache hit`),
            card("Output tokens", big(data.UsageOutputTokens), perSession(data.UsageOutputTokens ?? 0, big)),
            card("API time", duration(data.UsageAPIDurationMs), perSession(data.UsageAPIDurationMs ?? 0, duration)),
        ),
        h(
            "div",
            { class: "grid" },
            ...(data.ByModelUsage !== undefined ? modelPanels((data.ByModelUsage ?? []).map(sessionModelEntry), "session") : []),
            ...workspacePanels(data.ByCWDUsage ?? [], "session"),
        ),
    ];
}

function renderSession(data) {
    const totals = { Approved: 0, ApprovedForLocation: 0, Denied: 0, Unresolved: 0 };
    for (const e of data.ByKind ?? []) for (const k of Object.keys(totals)) totals[k] += e[k] ?? 0;
    const approved = totals.Approved + totals.ApprovedForLocation;
    const requests = data.Requests ?? 0;
    // ByKind is truncated to --top; totals are exact only when it covers every request.
    const covered = (data.ByKind ?? []).reduce((n, e) => n + (e.Total ?? 0), 0);
    const partial = covered < requests ? ` · listed kinds only (${num(covered)} of ${num(requests)})` : "";
    const cwdPick = (key) => patchQuery({ scope: "cwd", path: key });

    return [
        ...renderSessionUsage(data),
        data.UsageSessions !== undefined ? sectionHead("Permissions") : null,
        h(
            "div",
            { class: "cards" },
            card(data.UsageSessions !== undefined ? "Sessions with requests" : "Sessions", num(data.Sessions)),
            card("Permission requests", num(requests), data.Sessions ? `${(requests / data.Sessions).toFixed(1)} / session` : null),
            card("Approved", num(approved), `${pct(approved, covered)}${partial}`),
            card("Denied", num(totals.Denied), `${pct(totals.Denied, covered)}${partial}`),
            card("Unresolved", num(totals.Unresolved), `${pct(totals.Unresolved, covered)}${partial}`),
        ),
        legend(DECISION_COLORS.map(([label, color]) => [label, color])),
        h(
            "div",
            { class: "grid" },
            panel("Results", null, decisionRows(data.ByResult)),
            data.ByDecisionSource
                ? panel(
                      "Decision sources",
                      "click to filter",
                      decisionRows(data.ByDecisionSource, { onPick: (k) => addFilter("decisionSource", k), pickTitle: "Filter by this decision source" }),
                  )
                : null,
            panel("Read-only vs read-write", "a request may count in both", decisionRows(data.ByReadOnly)),
            panel("Tool kinds", "click to filter", decisionRows(data.ByKind, { onPick: (k) => addFilter("kind", k), pickTitle: "Filter by this kind" })),
            panel("Commands", "click to filter", decisionRows(data.ByCommand, { onPick: (k) => addFilter("command", k), pickTitle: "Filter by this command" })),
            panel("Paths", "click to filter", decisionRows(data.ByPath, { onPick: (k) => addFilter("path", exactPattern(k)), pickTitle: "Filter by this path" })),
            panel("URLs", "click to filter", decisionRows(data.ByURL, { onPick: (k) => addFilter("url", exactPattern(k)), pickTitle: "Filter by this URL" })),
            panel("Working directories", "click to scope", decisionRows(data.ByCWD, { onPick: cwdPick, pickTitle: "Show only this directory", display: shortPath }), { wide: true }),
        ),
    ];
}

// ---------------------------------------------------------------- VS Code

function simpleRows(entries, value, { onPick, pickTitle, format = num, color = "var(--c-bar)" } = {}) {
    if (!entries?.length) return h("div", { class: "muted small" }, "No entries.");
    const max = Math.max(...entries.map(value), 1);
    return h(
        "div",
        { class: "rows" },
        entries.map((e) =>
            h("div", { class: "row" }, keyCell(e.Key, onPick, pickTitle), stackedBar([[value(e), color, format(value(e))]], max), h("span", { class: "num" }, format(value(e)))),
        ),
    );
}

function toolTable(entries) {
    if (!entries?.length) return h("div", { class: "muted small" }, "No entries.");
    const max = Math.max(...entries.map((e) => e.Total), 1);
    return h(
        "table",
        null,
        h("thead", null, h("tr", null, h("th", null, "Tool"), h("th", null, "Calls"), h("th", null, ""), h("th", null, "Errors"), h("th", null, "Avg"), h("th", null, "Total time"))),
        h(
            "tbody",
            null,
            entries.map((e) =>
                h(
                    "tr",
                    null,
                    h("td", { class: "key" }, keyCell(e.Key, (k) => addFilter("tool", exactPattern(k)), "Filter by this tool")),
                    h("td", null, num(e.Total)),
                    h("td", null, stackedBar([[e.OK, "var(--c-ok)", `OK: ${num(e.OK)}`], [e.Error, "var(--c-error)", `Error: ${num(e.Error)}`]], max)),
                    h("td", null, e.Error ? `${num(e.Error)} (${pct(e.Error, e.Total)})` : "0"),
                    h("td", null, duration(e.AvgMs)),
                    h("td", null, duration(e.TotalMs)),
                ),
            ),
        ),
    );
}

const MODEL_VIEW_KEYS = {
    vscode: "attendant-dashboard.modelView",
    session: "attendant-dashboard.sessionModelView",
};
const WORKSPACE_VIEW_KEYS = {
    vscode: "attendant-dashboard.workspaceView",
    session: "attendant-dashboard.sessionWorkspaceView",
};

// Panel-local view state; persisted per browser profile so the choice survives reloads.
function loadView(key, normalize, source) {
    try {
        const saved = JSON.parse(localStorage.getItem(key) ?? "{}");
        return normalize(saved, source);
    } catch (error) {
        console.warn("Could not load table view preferences", error);
        return normalize(undefined, source);
    }
}

const modelViews = Object.fromEntries(Object.entries(MODEL_VIEW_KEYS).map(([source, key]) => [source, loadView(key, normalizeModelView, source)]));
const workspaceViews = Object.fromEntries(Object.entries(WORKSPACE_VIEW_KEYS).map(([source, key]) => [source, loadView(key, normalizeWorkspaceView, source)]));

function updateView(view, key, patch) {
    Object.assign(view, patch);
    try {
        localStorage.setItem(key, JSON.stringify(view));
    } catch (error) {
        console.warn("Could not save table view preferences; the choice lasts until reload", error);
    }
    render();
}

function setModelView(source, patch) {
    updateView(modelViews[source], MODEL_VIEW_KEYS[source], patch);
}

function setWorkspaceView(source, patch) {
    updateView(workspaceViews[source], WORKSPACE_VIEW_KEYS[source], patch);
}

function unitCostText(value) {
    if (value === null) return "–";
    if (value === 0) return "0";
    // aiu() keeps a single decimal, so anything under 0.05 would render as "0.0".
    return Math.abs(value) < 0.05 ? value.toPrecision(2) : aiu(value);
}

function segmented(label, options, value, onChange) {
    return h(
        "div",
        { class: "segmented", role: "group", "aria-label": label },
        h("span", { class: "hint" }, label),
        Object.entries(options).map(([key, text]) =>
            h("button", { type: "button", "aria-pressed": String(key === value), onclick: () => key !== value && onChange(key) }, text),
        ),
    );
}

function modelControls(source) {
    return usageControls(source, MODEL_UNITS, modelViews[source], (patch) => setModelView(source, patch));
}

function usageControls(source, units, view, onChange) {
    return h(
        "div",
        { class: "panel-controls" },
        legend(source === "session" ? USAGE_TOKEN_COLORS.map(([label, color]) => [label, color]) : VSCODE_TOKEN_COLORS),
        h(
            "div",
            { class: "controls" },
            segmented("Unit cost per", Object.fromEntries(Object.entries(units).map(([k, u]) => [k, u.label])), view.unit, (unit) => onChange({ unit })),
        ),
    );
}

function modelHeader(column, source) {
    const view = modelViews[source];
    const label = column.id === "unit" ? MODEL_UNITS[view.unit].header : column.label;
    const next = toggleModelSort(view, column.id, source);
    return sortableHeader(column, { view, label, next, attribute: "data-model-sort", onChange: (patch) => setModelView(source, patch) });
}

function sortableHeader(column, { view, label, next, attribute, onChange }) {
    const selected = view.sort === column.id;
    return h(
        "th",
        { scope: "col", "aria-sort": selected ? (view.direction === "asc" ? "ascending" : "descending") : "none" },
        h(
            "button",
            {
                type: "button",
                class: "sort-header",
                [attribute]: column.id,
                "aria-label": `${label}: sort ${next.direction === "asc" ? "ascending" : "descending"}`,
                title: `Sort by ${label} (${next.direction === "asc" ? "ascending" : "descending"})`,
                onclick: () => {
                    onChange(next);
                    document.querySelector(`[${attribute}="${column.id}"]`)?.focus({ preventScroll: true });
                },
            },
            label,
            h("span", { class: "sort-indicator", "aria-hidden": "true" }, selected ? (view.direction === "asc" ? "▲" : "▼") : "↕"),
        ),
    );
}

function modelPanels(entries, source) {
    const share = aiuShare(entries);
    const session = source === "session";
    const note = session
        ? "Only recorded modelMetrics are included; shares cover listed models. A session may use multiple models. Permission filters do not apply."
        : null;
    const missingSessions = entries.some((e) => e.Sessions == null);
    return [
        share ? panel("Usage share by model", session ? "AIU · listed models" : "AIU", share, { wide: true }) : null,
        panel("Models", session ? "modelMetrics · API requests" : "click to filter", [
            note ? h("p", { class: "muted small" }, note) : null,
            missingSessions ? h("p", { class: "muted small" }, "Session counts are not reported for some models; their AIU / session is unavailable.") : null,
            modelControls(source),
            modelTable(entries, source),
        ], { wide: true }),
    ];
}

function usageTokenSegments(e, source) {
    if (source !== "session") return vscodeTokenSegments(e);
    const fields = ["CachedTokens", "CacheWriteTokens", "UncachedInputTokens", "OutputTokens"];
    return USAGE_TOKEN_COLORS.map(([label, color], index) => [e[fields[index]], color, `${label}: ${big(e[fields[index]])}`]);
}

function modelTable(entries, source) {
    const view = modelViews[source];
    return metricsTable(sortModels(entries ?? [], view, source), source, {
        view, columns: modelColumns(source), units: MODEL_UNITS, costOf: unitCost,
        header: (column) => modelHeader(column, source),
        pick: source === "vscode" ? (key) => addFilter("model", exactPattern(key)) : null,
        pickTitle: "Filter by this model",
        empty: source === "session" ? "No recorded modelMetrics for this query." : "No entries.",
    });
}

function metricsTable(entries, source, { view, columns, units, costOf, header, pick, pickTitle, displayKey, empty = "No entries." }) {
    if (!entries.length) return h("div", { class: "muted small" }, empty);
    const max = Math.max(...entries.map((e) => (e.InputTokens ?? 0) + (e.OutputTokens ?? 0)), 1);
    const maxUnit = Math.max(...entries.map((e) => costOf(e, view.unit) ?? 0), Number.MIN_VALUE);
    const unit = units[view.unit];
    const cell = (column, entry, cost) => {
        const value = column.value(entry, view.unit);
        if (value === null) return h("td", null, "–");
        switch (column.id) {
            case "model":
            case "workspace":
                return h("td", { class: "key" }, keyCell(entry.Key, pick, pickTitle, displayKey ? displayKey(entry.Key) : entry.Key));
            case "tokens":
                return h("td", null, stackedBar(usageTokenSegments(entry, source), max));
            case "input":
            case "cached":
            case "cacheWrite":
            case "uncached":
            case "output":
                return h("td", null, big(value));
            case "cacheHit":
                return h("td", null, pct(entry.CachedTokens, entry.InputTokens));
            case "ttft":
            case "apiDuration":
                return h("td", null, duration(value));
            case "aiu":
                return h("td", null, aiu(value));
            case "premium":
                return h("td", null, decimal(value));
            case "unit":
                return h("td", null, unitCostText(cost));
            case "turnCost":
                return h("td", null, unitCostText(value));
            default:
                return h("td", null, num(value));
        }
    };
    return h(
        "table",
        null,
        h(
            "thead",
            null,
            h(
                "tr",
                null,
                columns.map(header),
                h("th", { scope: "col", "aria-label": "Unit cost comparison" }),
            ),
        ),
        h(
            "tbody",
            null,
            entries.map((e) => {
                const cost = costOf(e, view.unit);
                return h(
                    "tr",
                    null,
                    columns.map((column) => cell(column, e, cost)),
                    h("td", null, stackedBar([[cost ?? 0, "var(--c-unit-cost)", `${unit.header}: ${unitCostText(cost)}`]], maxUnit)),
                );
            }),
        ),
    );
}

const VSCODE_TOKEN_COLORS = [
    ["Cached input", "var(--c-cached)"],
    ["Uncached input", "var(--c-input)"],
    ["Output", "var(--c-output)"],
];

/** Token bar segments for an entry with VS Code style Input/Cached/Output token fields. */
function vscodeTokenSegments(e) {
    const uncached = Math.max((e.InputTokens ?? 0) - (e.CachedTokens ?? 0), 0);
    return [
        [e.CachedTokens, VSCODE_TOKEN_COLORS[0][1], `Cached input: ${big(e.CachedTokens)}`],
        [uncached, VSCODE_TOKEN_COLORS[1][1], `Uncached input: ${big(uncached)}`],
        [e.OutputTokens, VSCODE_TOKEN_COLORS[2][1], `Output: ${big(e.OutputTokens)}`],
    ];
}

function workspaceHeader(column, source, usage) {
    const view = normalizeWorkspaceView(workspaceViews[source], source, usage);
    const label = column.id === "unit" ? WORKSPACE_UNITS[view.unit].header : column.label;
    return sortableHeader(column, {
        view, label, next: toggleWorkspaceSort(view, column.id, source, usage),
        attribute: "data-workspace-sort", onChange: (patch) => setWorkspaceView(source, patch),
    });
}

function workspacePanels(rawEntries, source, topNote = state.query.top ? `top ${state.query.top}` : null) {
    const entries = rawEntries.map((e) => workspaceEntry(e, source));
    const usage = source === "session" || hasWorkspaceUsage(entries);
    const share = usage ? shareBar(entries, (e) => e.UsageAIU ?? 0, (e) => shortPath(e.Key)) : null;
    const notes = [];
    if (!usage && entries.length) notes.push("Workspace usage is not reported by this CLI build; update gh copilot-attendant to compare AIU.");
    if (usage && entries.some((e) => e.Requests === undefined)) notes.push("Request counts are not reported for some workspaces; their AIU / request is unavailable.");
    if (usage && entries.some((e) => e.Sessions === undefined)) notes.push("Session counts are not reported for some workspaces; their AIU / session is unavailable.");
    return [
        share ? panel("Usage share by workspace", topNote ? `AIU · ${topNote}` : "AIU", share, { wide: true }) : null,
        panel("Workspaces", topNote ? `${topNote} · click to scope` : "click to scope", [
            ...notes.map((note) => h("p", { class: "muted small" }, note)),
            usage ? usageControls(source, WORKSPACE_UNITS, workspaceViews[source], (patch) => setWorkspaceView(source, patch)) : null,
            workspaceTable(entries, source, usage),
        ], { wide: usage }),
    ];
}

function workspaceTable(entries, source, usage) {
    if (!entries?.length) return h("div", { class: "muted small" }, "No entries.");
    const pick = (k) => patchQuery({ scope: "cwd", path: k });
    const keyTd = (e) => h("td", { class: "key" }, keyCell(e.Key, pick, "Show only this workspace", shortPath(e.Key)));
    const view = normalizeWorkspaceView(workspaceViews[source], source, usage);
    const columns = workspaceColumns(source, usage);
    const sorted = sortWorkspaces(entries, view, source, usage);
    if (!usage) {
        const max = Math.max(...entries.map((e) => e.ToolCalls), 1);
        return h(
            "table",
            null,
            h("thead", null, h("tr", null, columns.flatMap((column) => [
                workspaceHeader(column, source, usage),
                column.id === "toolCalls" ? h("th", { scope: "col", "aria-label": "Tool calls comparison" }) : null,
            ]))),
            h(
                "tbody",
                null,
                sorted.map((e) =>
                    h(
                        "tr",
                        null,
                        keyTd(e),
                        h("td", null, num(e.ToolCalls)),
                        h("td", null, stackedBar([[e.ToolCalls, "var(--c-bar)", `Tool calls: ${num(e.ToolCalls)}`]], max)),
                        h("td", null, num(e.Requests)),
                        h("td", null, num(e.Turns)),
                    ),
                ),
            ),
        );
    }
    return metricsTable(sorted, source, {
        view, columns, units: WORKSPACE_UNITS, costOf: workspaceUnitCost,
        header: (column) => workspaceHeader(column, source, usage),
        pick, pickTitle: "Show only this workspace", displayKey: shortPath,
    });
}

const SHARE_COLORS = ["var(--c-input)", "var(--c-output)", "var(--c-cached)", "var(--c-unresolved)", "var(--c-denied)", "var(--text-color-muted, #59636e)"];

/** Renders a proportional share bar and legend for entries with a positive value. */
function shareBar(entries, value, label = (e) => e.Key) {
    const withValue = (entries ?? []).filter((e) => value(e) > 0);
    const total = withValue.reduce((n, e) => n + value(e), 0);
    if (!total) return null;
    const color = (i) => SHARE_COLORS[i % SHARE_COLORS.length];
    const segments = withValue.map((e, i) => [value(e), color(i), `${label(e)}: ${aiu(value(e))} AIU (${pct(value(e), total)})`]);
    const bar = stackedBar(segments, total);
    bar.classList.add("share");
    return [bar, legend(withValue.map((e, i) => [`${label(e)} ${pct(value(e), total)}`, color(i)]))];
}

function aiuShare(models) {
    return shareBar(models, (m) => m.UsageAIU ?? 0);
}

function renderVscode(data) {
    const models = data.ByModel ?? [];
    const sum = (field) => models.reduce((n, m) => n + (m[field] ?? 0), 0);
    const input = sum("InputTokens");
    const cached = sum("CachedTokens");
    const tools = data.ByTool ?? [];
    const toolErrors = tools.reduce((n, t) => n + (t.Error ?? 0), 0);
    const toolTotal = tools.reduce((n, t) => n + (t.Total ?? 0), 0);
    const workspaces = data.ByWorkspace ?? [];
    const topNote = state.query.top ? `top ${state.query.top}` : null;
    // ByModel is truncated to --top, so model-derived totals may be partial.
    const top = state.results.vscode.argv ? Number(state.results.vscode.argv.find((a) => a.startsWith("--top="))?.slice(6)) : state.query.top;
    const modelNote = top > 0 && models.length >= top ? " · listed models only" : "";

    return [
        h(
            "div",
            { class: "cards" },
            card("Sessions", num(data.Sessions)),
            card("Turns", num(data.Turns), data.Sessions ? `${(data.Turns / data.Sessions).toFixed(1)} / session` : null),
            card("LLM requests", num(data.LLMRequests), data.Turns ? `${(data.LLMRequests / data.Turns).toFixed(1)} / turn` : null),
            card("Tool calls", num(data.ToolCalls), toolTotal ? `${pct(toolErrors, toolTotal)} errors (listed tools)` : null),
            card("Input tokens", big(input), `${pct(cached, input)} cached${modelNote}`),
            card("Output tokens", big(sum("OutputTokens")), modelNote ? modelNote.slice(3) : null),
            card("Usage (AIU)", aiu(sum("UsageAIU")), modelNote ? modelNote.slice(3) : null),
        ),
        h(
            "div",
            { class: "grid" },
            ...modelPanels(models, "vscode"),
            ...workspacePanels(workspaces, "vscode", topNote),
            panel("Tools", topNote ? `${topNote} · click to filter` : "click to filter", toolTable(tools), { wide: true }),
            panel("Subagents", "click to filter", simpleRows(data.ByAgent, (e) => e.Total, { onPick: (k) => addFilter("agent", exactPattern(k)), pickTitle: "Filter by this agent" })),
        ),
    ];
}

// ---------------------------------------------------------------- ask agent

function renderPresets(source, result) {
    const presets = meta.presets.filter((p) => p.sources.includes(source));
    const disabled = !(result.data && result.status === "ok" && result.fresh);
    $("presets").replaceChildren(
        ...presets.map((p) => h("button", { type: "button", class: "btn small", disabled, onclick: () => ask({ preset: p.id }) }, p.label)),
    );
    $("ask-send").disabled = disabled;
}

async function ask({ preset } = {}) {
    const text = $("ask-text").value.trim();
    if (!preset && !text) return;
    const status = $("ask-status");
    status.textContent = "Sending…";
    try {
        await api("POST", "/api/ask", { source: activeSource(), preset, text });
        status.textContent = "Sent to the agent.";
        $("ask-text").value = "";
    } catch (error) {
        status.textContent = `Could not send: ${error.message}`;
    }
}

// ---------------------------------------------------------------- events

async function stream() {
    for (;;) {
        try {
            const res = await fetch("/events", { headers: { "X-Dashboard-Token": token } });
            if (!res.ok || !res.body) throw new Error(`events: ${res.status}`);
            const reader = res.body.pipeThrough(new TextDecoderStream()).getReader();
            let buffer = "";
            for (;;) {
                const { value, done } = await reader.read();
                if (done) break;
                buffer += value;
                let index;
                while ((index = buffer.indexOf("\n\n")) >= 0) {
                    const block = buffer.slice(0, index);
                    buffer = buffer.slice(index + 2);
                    const data = block
                        .split("\n")
                        .filter((line) => line.startsWith("data: "))
                        .map((line) => line.slice(6))
                        .join("\n");
                    if (data) setState({ ...JSON.parse(data), meta });
                }
            }
        } catch {
            // Reconnect below; the extension may be reloading.
        }
        await new Promise((r) => setTimeout(r, 2000));
    }
}

function wire() {
    for (const tab of document.querySelectorAll(".tabs button")) {
        tab.addEventListener("click", () => patchQuery({ source: tab.dataset.source }));
    }
    $("refresh").addEventListener("click", () => patchQuery({ source: activeSource() }, { force: true }));
    $("reset").addEventListener("click", () => {
        const source = activeSource();
        patchQuery({
            source,
            scope: "worktree",
            path: "",
            session: "",
            period: "30d",
            since: "",
            until: "",
            top: 10,
            filters: Object.fromEntries(meta.sources.map((s) => [s, Object.fromEntries(meta.filters[s].map((f) => [f, []]))])),
        });
    });
    const form = $("query");
    form.addEventListener("submit", (e) => {
        e.preventDefault();
        patchQuery(readForm());
    });
    form.scope.addEventListener("change", () => {
        form.path.disabled = form.scope.value === "all";
        if (form.scope.value === "cwd" && !form.path.value) form.path.focus();
    });
    form.period.addEventListener("change", () => {
        if (form.period.value) form.since.value = "";
    });
    form.since.addEventListener("input", () => {
        if (form.since.value.trim()) form.period.value = "";
    });
    $("ask-send").addEventListener("click", () => ask());
    $("ask-text").addEventListener("keydown", (e) => {
        if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) ask();
    });
    $("copy").addEventListener("click", async () => {
        const text = $("command").textContent;
        try {
            await navigator.clipboard.writeText(text);
            $("copy").textContent = "Copied";
        } catch {
            const range = document.createRange();
            range.selectNodeContents($("command"));
            getSelection().removeAllRanges();
            getSelection().addRange(range);
            $("copy").textContent = "Selected";
        }
        setTimeout(() => ($("copy").textContent = "Copy"), 1500);
    });
}

async function main() {
    wire();
    try {
        setState(await api("GET", "/api/state"));
    } catch (error) {
        showError(error.message, "client");
    }
    stream();
}

main();
