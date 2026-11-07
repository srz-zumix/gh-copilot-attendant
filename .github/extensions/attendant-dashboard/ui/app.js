// Dashboard UI. Talks to the extension's loopback server with a per-instance
// token and receives state pushes over a streamed /events response. All text
// from the server is inserted with textContent (never innerHTML) because keys
// come from local logs.

import { MODEL_UNITS, modelColumns, normalizeModelView, sessionModelEntry, sortModels, toggleModelSort, unitCost } from "./model-view.mjs";
import { WORKSPACE_UNITS, hasWorkspaceUsage, normalizeWorkspaceView, sortWorkspaces, toggleWorkspaceSort, workspaceColumns, workspaceEntry, workspaceUnitCost } from "./workspace-view.mjs";
import { normalizeTableView, sortTableEntries, toggleTableSort } from "./table-view.mjs";
import { panelClearPatch } from "./filter-view.mjs";

const token = document.querySelector('meta[name="dashboard-token"]').content;
const $ = (id) => document.getElementById(id);

const LABELS = {
    session: "Copilot CLI",
    vscode: "VS Code",
    filters: {
        operation: "Operation",
        agentMode: "Agent mode",
        result: "Result",
        commandAccess: "Command access",
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
const EXACT_FILTERS = new Set(["agentMode", "result", "kind", "command", "decisionSource"]);

let state = null;
let meta = null;
let formQueryKey = "";
let filtersKey = "";
let loadingTimer = null;
const disclosureState = {
    activity: true, fusion: true, usage: true, permissions: true, "fusion-usage": true, "vscode-usage": true, "vscode-tools": true,
};
const EXCLUDED_BULK_DISCLOSURES = new Set(["usage", "permissions"]);

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

const aiuFmt = new Intl.NumberFormat("en", { maximumFractionDigits: 2 });

function aiu(n) {
    const value = n ?? 0;
    return value > 0 && value < 0.01 ? "<0.01" : aiuFmt.format(value);
}

// Premium requests can be fractional (model multipliers such as 0.5x).
const decimalFmt = new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 });

function decimal(n) {
    return decimalFmt.format(n ?? 0);
}

function big(n) {
    return Math.abs(n ?? 0) >= 10_000 ? compact.format(n) : num(n);
}

function tokenCount(n, partial = false) {
    if (!partial || n < 10_000) return big(n);
    const unit = n >= 1e12 ? 1e12 : n >= 1e9 ? 1e9 : n >= 1e6 ? 1e6 : 1e3;
    const step = unit / 10;
    // A compact lower bound must not round above the recorded subtotal.
    return big(n - (n % step));
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

function calendarPeriodBounds(period, now = new Date()) {
    if (period !== "this-month" && period !== "last-month") return null;
    const year = now.getFullYear();
    const month = now.getMonth();
    return {
        period: "",
        since: new Date(year, month - (period === "last-month" ? 1 : 0), 1).toISOString(),
        until: period === "last-month" ? new Date(year, month, 1).toISOString() : "",
    };
}

function periodSelection(query, now = new Date()) {
    if (query.period) return query.period;
    for (const period of ["this-month", "last-month"]) {
        const bounds = calendarPeriodBounds(period, now);
        if (query.since === bounds.since && query.until === bounds.until) return period;
    }
    return "";
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
        top: data.get("top") === "" ? 20 : Number(data.get("top")),
    };
    if (patch.since || calendarPeriodBounds(patch.period)) patch.period = "";
    return patch;
}

function fillForm(query) {
    const selection = periodSelection(query);
    const key = JSON.stringify([query.scope, query.path, query.period, query.since, query.until, query.session, query.top, selection]);
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
    period.value = selection;
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
    for (const name of Object.keys(disclosureState)) {
        const disclosure = document.querySelector(`[data-${name}-disclosure]`);
        if (disclosure) disclosureState[name] = disclosure.open;
    }
    const tableStates = [
        "data-fusion-phase-sort", "data-agent-sort", "data-fusion-model-sort", "data-fusion-workspace-sort",
        "data-activity-page", "data-clear-panel",
        ...Object.keys(disclosureState).map((name) => `data-${name}-toggle`),
    ].map((attribute) => ({
        attribute,
        focus: document.activeElement?.getAttribute(attribute),
        scrollLeft: document.querySelector(`[${attribute}]`)?.closest(".panel")?.scrollLeft,
    }));
    const source = activeSource();
    const result = state.results[source];
    $("vscode-harness-warning").hidden = source !== "vscode";

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
            : [h("div", { class: "empty" }, result.status === "loading" ? "Running gh copilot-attendant…" : "No data yet.")]).filter((node) => node !== null),
    );
    $("refresh").disabled = false;
    $("command").textContent = `gh copilot-attendant ${result.command.map(shellQuote).join(" ")}`;
    $("command-label").textContent = source === "session" && state.query.fusionOnly
        ? "Fetch command · Fusion filter applied locally"
        : result.localFiltering ? "Equivalent command" : "Command";
    $("command").title = source === "session" && state.query.fusionOnly
        ? "The command fetches all session statistics; the dashboard applies the Fusion-only filter locally."
        : result.fetchArgv ? `Data fetch: gh copilot-attendant ${result.fetchArgv.map(shellQuote).join(" ")}` : "";
    renderPresets(source, result);
    for (const { attribute, focus, scrollLeft } of tableStates) {
        if (scrollLeft != null) {
            const panel = document.querySelector(`[${attribute}]`)?.closest(".panel");
            if (panel) panel.scrollLeft = scrollLeft;
        }
        if (focus) {
            let target = document.querySelector(`[${attribute}="${focus}"]`);
            if (attribute === "data-clear-panel" && target?.disabled) {
                target = document.querySelector(`[data-${focus}-toggle]`);
            }
            target?.focus({ preventScroll: true });
        }
    }
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
        parts.push(`${result.localFiltering ? "Data fetched" : "Updated"} ${new Date(result.ranAt).toLocaleTimeString()} · ${duration(result.durationMs)}`);
        if (result.localFiltering) parts.push(" · cached data / local filters");
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
    const supportsFusionOnly = typeof state.query.fusionOnly === "boolean";
    const fusionOnly = source === "session" && state.query.fusionOnly === true;
    // Rebuilding would discard text the user is typing, so only do it on change.
    const key = JSON.stringify([source, filters, fusionOnly, supportsFusionOnly]);
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
                            disabled: fusionOnly,
                            onchange: (e) => (e.target.checked ? addFilter(name, op) : removeFilter(name, op)),
                        }),
                        op,
                    ),
                ),
            );
        }
        if (name === "commandAccess") {
            return h(
                "div",
                { class: "filter" },
                h("span", { class: "name" }, LABELS.filters[name]),
                ["read-only", "read-write"].map((access) =>
                    h(
                        "label",
                        { class: "toggle" },
                        h("input", {
                            type: "checkbox",
                            checked: values.includes(access),
                            disabled: fusionOnly,
                            onchange: (e) => (e.target.checked ? addFilter(name, access) : removeFilter(name, access)),
                        }),
                        access,
                    ),
                ),
            );
        }
        const input = h("input", {
            type: "text",
            spellcheck: "false",
            disabled: fusionOnly,
            placeholder: EXACT_FILTERS.has(name) ? "exact value" : "regex",
            "aria-label": `Add ${LABELS.filters[name]} filter`,
            onkeydown: (e) => {
                if (e.key !== "Enter") return;
                e.preventDefault();
                const value = name === "agentMode" ? e.target.value : e.target.value.trim();
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
                    h("button", { type: "button", disabled: fusionOnly, "aria-label": `Remove ${value}`, onclick: () => removeFilter(name, value) }, "×"),
                ),
            ),
            input,
        );
    });
    if (source === "session") {
        groups.unshift(
            h("label", { class: "toggle fusion-filter" },
                h("input", {
                    type: "checkbox", checked: fusionOnly, disabled: !supportsFusionOnly,
                    onchange: (e) => patchQuery({ fusionOnly: e.target.checked }),
                }),
                "Fusion usage only"),
            !supportsFusionOnly ? h("span", { class: "muted small" }, "Reload the dashboard extension to enable Fusion-only filtering.")
                : fusionOnly ? h("span", { class: "muted small" }, "Permission filters are inactive in Fusion-only mode.") : null,
        );
    }
    $("filters").replaceChildren(...groups.filter(Boolean));
}

function card(label, value, sub, title) {
    return h("div", { class: "card", title },
        h("div", { class: "label" }, label),
        h("div", { class: "value", "aria-label": title ? `${value}; ${title}` : null }, value),
        sub ? h("div", { class: "sub" }, sub) : null);
}

function availableNumber(value) {
    return Number.isFinite(value) && value >= 0;
}

function perUsageSession(value, sessions, format) {
    const result = availableNumber(value) && availableNumber(sessions) && sessions > 0 ? value / sessions : null;
    return availableNumber(result) ? `${format(result)} / session` : "– / session";
}

function premiumUnitText(aiuValue, premium) {
    const result = availableNumber(aiuValue) && availableNumber(premium) && premium > 0 ? aiuValue / premium : null;
    return availableNumber(result) ? `${fusionNumber(result)} AIU / premium request` : "– AIU / premium request";
}

function cacheHitText(cacheRead, prompt) {
    return availableNumber(cacheRead) && availableNumber(prompt) && prompt > 0 && cacheRead <= prompt
        ? `${pct(cacheRead, prompt)} cache hit` : "– cache hit";
}

function recordedTokenCard(label, value, sub, partial = false) {
    const recorded = availableNumber(value);
    const text = recorded ? `${partial ? "≥ " : ""}${tokenCount(value, partial)}` : "–";
    const title = recorded
        ? Number.isSafeInteger(value) ? `Recorded${partial ? " subtotal" : ""}: ${num(value)} tokens`
            : "Exact token count exceeds numeric precision"
        : "Token data is unavailable";
    return card(label, text, sub, title);
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

function panel(title, hint, body, { wide = false, filter } = {}) {
    const key = title.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
    return disclosurePanel(`panel-${activeSource()}-${key}`, title, hint, body, { wide, filter });
}

function toggleAllDisclosures(event) {
    if (!event.altKey) return;
    event.preventDefault();
    const disclosures = [...document.querySelectorAll(".panel-disclosure, .category-disclosure")]
        .filter((disclosure) => ![...EXCLUDED_BULK_DISCLOSURES].some((name) =>
            disclosure.hasAttribute(`data-${name}-disclosure`)));
    if (!disclosures.length) return;
    const open = !disclosures.every((disclosure) => disclosure.open);
    for (const disclosure of disclosures) {
        const name = Object.keys(disclosureState).find((key) => disclosure.hasAttribute(`data-${key}-disclosure`));
        if (name) disclosureState[name] = open;
        disclosure.open = open;
    }
}

function disclosurePanel(name, title, hint, body, { wide = true, filter } = {}) {
    if (!Object.hasOwn(disclosureState, name)) disclosureState[name] = true;
    const patch = filter ? panelClearPatch(state.query, activeSource(), filter) : null;
    const bodyId = `panel-body-${name}`;
    // Keep Clear separate from the summary; CSS hides the body when details closes.
    return h("section",
        { class: `panel${wide ? " wide" : ""}` },
        h("details", { class: "panel-disclosure", [`data-${name}-disclosure`]: true, open: disclosureState[name] },
            h("summary", { [`data-${name}-toggle`]: name, "aria-controls": bodyId, onclick: toggleAllDisclosures }, h("h2", null, title))),
        h("div", { class: "panel-actions" },
            hint ? h("span", { class: "hint" }, hint) : null,
            filter ? h("button", {
                type: "button", class: "btn subtle small", disabled: !patch,
                "data-clear-filter": filter, "data-clear-panel": name,
                "aria-label": `Clear ${title} ${filter === "workspace" ? "scope" : "filters"}`,
                title: filter === "workspace" ? "Show all workspaces; keep other filters" : `Clear ${LABELS.filters[filter]} filters only`,
                onclick: () => { if (patch) patchQuery(patch); },
            }, "Clear") : null),
        h("div", { class: "panel-body", id: bodyId }, body));
}

// ---------------------------------------------------------------- Copilot CLI

const ACTIVITY_DAY_MS = 24 * 60 * 60 * 1000;
const ACTIVITY_PAGE_DAYS = 30;
const ACTIVITY_COVERAGE = new Set(["full", "partial", "outside", "nonexistent"]);
let activityRangeKey = "";
let activityPageEnd = null;

function activitySection(hint, body) {
    return disclosurePanel("activity", "Session activity", null, [
        hint ? h("p", { class: "muted small" }, hint) : null,
        body,
    ]);
}

function activityDayNumber(value) {
    if (typeof value !== "string" || !/^\d{4}-\d{2}-\d{2}$/.test(value)) return null;
    const number = Date.parse(`${value}T00:00:00.000Z`);
    return Number.isFinite(number) && new Date(number).toISOString().slice(0, 10) === value ? number : null;
}

function activityDate(number) {
    return new Date(number).toISOString().slice(0, 10);
}

function activityAverage(total, days) {
    const denominator = BigInt(days);
    // Round exactly before formatting, since the total may exceed a safe integer.
    const hundredths = (total * 100n + denominator / 2n) / denominator;
    const whole = fmt.format(hundredths / 100n);
    const remainder = hundredths % 100n;
    if (remainder === 0n) return whole;
    const fraction = decimalFmt.formatToParts(Number(remainder) / 100)
        .filter((part) => part.type === "decimal" || part.type === "fraction")
        .map((part) => part.value).join("");
    return whole + fraction;
}

function activitySummaryRow(label, totals) {
    const maximum = totals.reduce((highest, current) => {
        if (!current.days) return highest;
        if (!highest || (label === "Total"
            ? current.total > highest.total
            : current.total * BigInt(highest.days) > highest.total * BigInt(current.days))) return current;
        return highest;
    }, null);
    return h("tr", { "data-activity-summary": label.toLowerCase() },
        h("th", { scope: "row" }, label),
        totals.map(({ total, days, partial }, hour) => {
            const hourLabel = `${String(hour).padStart(2, "0")}:00`;
            if (!days) {
                const description = `${label} for ${hourLabel}: no covered days on this page`;
                return h("td", { title: description, "aria-label": description }, "–");
            }
            const value = label === "Total" ? fmt.format(total) : activityAverage(total, days);
            const level = total === 0n ? 0 : label === "Total"
                ? Number((4n * total + maximum.total - 1n) / maximum.total)
                : Number((4n * total * BigInt(maximum.days) +
                    maximum.total * BigInt(days) - 1n) / (maximum.total * BigInt(days)));
            const units = label === "Total" ? "summed daily session counts" : "sessions per covered day";
            const description = `${label} for ${hourLabel}: ${value} ${units}; ${num(days)} covered ${days === 1 ? "day" : "days"}${partial ? "; includes partially covered hours" : ""}`;
            return h("td", {
                class: `activity-cell activity-level-${level} activity-summary-cell${partial ? " activity-partial" : ""}`,
                title: description,
                "aria-label": description,
            },
                value, partial ? "*" : null);
        }));
}

function prepareActivity(activity) {
    const count = (value) => Number.isSafeInteger(value) && value >= 0;
    if (typeof activity.TimeZone !== "string" || !activity.TimeZone ||
        typeof activity.AsOf !== "string" || !Number.isFinite(Date.parse(activity.AsOf)) ||
        (activity.AgentModes !== undefined &&
            (!Array.isArray(activity.AgentModes) || !activity.AgentModes.every((mode) => typeof mode === "string" && mode.length > 0))) ||
        !["", "empty-window", "no-dated-events"].includes(activity.EmptyReason) ||
        !["ScannedSessions", "UntimedSessions", "UntimedEvents", "MalformedEvents"].every((key) => count(activity[key])) ||
        activity.UntimedSessions > activity.ScannedSessions ||
        !Array.isArray(activity.Days) || !Array.isArray(activity.Coverage)) return null;
    const empty = !activity.StartDate && !activity.EndDate;
    const first = empty ? null : activityDayNumber(activity.StartDate);
    const last = empty ? null : activityDayNumber(activity.EndDate);
    if ((empty && (!activity.EmptyReason || activity.Days.length || activity.Coverage.length)) ||
        (!empty && (first === null || last === null || first > last || activity.EmptyReason))) return null;
    const days = new Map();
    const coverage = new Map();
    let maximum = 0;
    for (const entry of activity.Days) {
        const date = activityDayNumber(entry?.Date);
        if (date === null || date < first || date > last || days.has(entry.Date) ||
            !Array.isArray(entry.Hours) || entry.Hours.length !== 24 || !entry.Hours.every(count)) return null;
        days.set(entry.Date, entry.Hours);
        for (const value of entry.Hours) maximum = Math.max(maximum, value);
    }
    for (const entry of activity.Coverage) {
        const date = activityDayNumber(entry?.Date);
        if (date === null || date < first || date > last || coverage.has(entry.Date) ||
            !Array.isArray(entry.Hours) || entry.Hours.length !== 24 ||
            !entry.Hours.every((value) => ACTIVITY_COVERAGE.has(value))) return null;
        const counts = days.get(entry.Date);
        if (counts?.some((value, hour) => value > 0 && ["outside", "nonexistent"].includes(entry.Hours[hour]))) return null;
        coverage.set(entry.Date, entry.Hours);
    }
    return { first, last, days, coverage, maximum };
}

function activityPanel(activity) {
    const selectedModes = state?.query?.filters?.session?.agentMode ?? [];
    if (activity == null) {
        return activitySection(null, h("p", { class: "muted small" },
            "Session activity is unavailable in this CLI result. Update gh copilot-attendant or choose the source runner."));
    }
    if (selectedModes.length > 0 && !Array.isArray(activity.AgentModes)) {
        return activitySection(null, h("p", { class: "muted small" },
            "This result does not confirm that Session activity used the selected Agent mode filter. Refresh with an up-to-date CLI or choose the source runner."));
    }
    if (Array.isArray(activity.AgentModes)) {
        const appliedModes = [...new Set(activity.AgentModes)].sort();
        const expectedModes = [...new Set(selectedModes)].sort();
        if (JSON.stringify(appliedModes) !== JSON.stringify(expectedModes)) {
            const applied = appliedModes.length ? appliedModes.map((mode) => JSON.stringify(mode)).join(", ") : "no Agent mode filter";
            const expected = expectedModes.length ? expectedModes.map((mode) => JSON.stringify(mode)).join(", ") : "no Agent mode filter";
            return activitySection(null, h("p", { class: "muted small" },
                `This Session activity result was collected with ${applied}; the current selection is ${expected}. Refresh to show matching data.`));
        }
    }
    if (activity.Version !== 1) {
        return activitySection(null, h("p", { class: "muted small" },
            "This session activity format is unsupported. Update the dashboard and gh copilot-attendant."));
    }
    const prepared = prepareActivity(activity);
    if (!prepared) {
        return activitySection(null, h("p", { class: "muted small" },
            "The session activity data is invalid and cannot be displayed."));
    }
    const details = [
        h("p", { class: "muted small" }, selectedModes.length
            ? "Each cell counts distinct sessions with a matching permission.requested event in that local hour. The Agent mode is read from the request and its timestamp determines the hour; repeated clock hours are combined. Other permission filters and Top do not apply."
            : "Each cell counts distinct sessions with at least one recorded event in that local hour, not continuous active time. Repeated clock hours are combined. Other permission filters and Top do not apply."),
        h("p", { class: "muted small" }, `CLI host time zone: ${activity.TimeZone} · scanned as of ${activity.AsOf}`),
        activity.UntimedEvents || activity.MalformedEvents
            ? h("p", { class: "muted small" }, `${num(activity.UntimedEvents)} ${selectedModes.length ? "matching permission request" : "included event"}${activity.UntimedEvents === 1 ? "" : "s"} in ${num(activity.UntimedSessions)} scoped ${activity.UntimedSessions === 1 ? "session" : "sessions"} ${activity.UntimedEvents === 1 ? "has" : "have"} no usable timestamp; ${num(activity.MalformedEvents)} event ${activity.MalformedEvents === 1 ? "envelope" : "envelopes"} across the scoped logs could not be parsed. Untimed records cannot be placed on the chart, including when a time window is selected.`)
            : null,
    ];
    if (prepared.first === null) {
        details.push(h("p", { class: "muted small" }, activity.EmptyReason === "empty-window"
            ? "The selected time window is empty: Since must be earlier than Until."
            : "No dated events are available to establish a display range."));
        return activitySection(null, details);
    }

    const key = `${activity.TimeZone}\n${activity.StartDate}\n${activity.EndDate}`;
    if (activityRangeKey !== key || activityPageEnd < prepared.first || activityPageEnd > prepared.last) {
        activityRangeKey = key;
        activityPageEnd = prepared.last;
    }
    const pageStart = Math.max(prepared.first, activityPageEnd - (ACTIVITY_PAGE_DAYS - 1) * ACTIVITY_DAY_MS);
    const startLabel = activityDate(pageStart);
    const endLabel = activityDate(activityPageEnd);
    const rows = [];
    const totals = Array.from({ length: 24 }, () => ({ total: 0n, days: 0, partial: false }));
    for (let day = activityPageEnd; day >= pageStart; day -= ACTIVITY_DAY_MS) {
        const date = activityDate(day);
        const counts = prepared.days.get(date);
        const coverage = prepared.coverage.get(date);
        rows.push(h("tr", null,
            h("th", { scope: "row" }, date),
            Array.from({ length: 24 }, (_, hour) => {
                const hourLabel = `${String(hour).padStart(2, "0")}:00`;
                const value = counts?.[hour] ?? 0;
                const status = coverage?.[hour] ?? "full";
                if (status === "outside" || status === "nonexistent") {
                    const meaning = status === "outside" ? "outside the selected time window" : "local clock hour did not exist";
                    return h("td", { class: `activity-cell activity-${status}`, title: `${date} ${hourLabel}: ${meaning}`, "aria-label": `${date} ${hourLabel}: ${meaning}` },
                        status === "outside" ? "–" : "×");
                }
                totals[hour].total += BigInt(value);
                totals[hour].days++;
                totals[hour].partial ||= status === "partial";
                const level = value === 0 ? 0 : Math.max(1, Math.ceil(4 * value / prepared.maximum));
                const partial = status === "partial";
                const subject = selectedModes.length
                    ? value === 1 ? "with a matching permission request" : "with matching permission requests"
                    : "with recorded events";
                const label = `${date} ${hourLabel}: ${num(value)} ${value === 1 ? "session" : "sessions"} ${subject}${partial ? "; selected window covers only part of this hour" : ""}`;
                return h("td", { class: `activity-cell activity-level-${level}${partial ? " activity-partial" : ""}`, title: label, "aria-label": label },
                    num(value), partial ? "*" : null);
            })));
    }
    details.push(
        h("div", { class: "activity-controls" },
            h("button", {
                type: "button", class: "btn small", "data-activity-page": "earlier", disabled: pageStart <= prepared.first,
                onclick: () => { activityPageEnd = pageStart - ACTIVITY_DAY_MS; render(); },
            }, "Earlier"),
            h("span", { class: "muted small", "aria-live": "polite" }, `${startLabel} to ${endLabel}`),
            h("button", {
                type: "button", class: "btn small", "data-activity-page": "later", disabled: activityPageEnd >= prepared.last,
                onclick: () => { activityPageEnd = Math.min(prepared.last, activityPageEnd + ACTIVITY_PAGE_DAYS * ACTIVITY_DAY_MS); render(); },
            }, "Later")),
        h("p", { class: "muted small" }, "0: no events recorded as of the scan · *: partial time window · –: outside time window · ×: nonexistent local hour. Darker cells have more sessions."),
        h("p", { class: "muted small", id: "activity-summary-note" },
            "Totals sum daily session counts on this page, so a session can be counted on multiple days. Averages include zeros and partial hours once per covered day; outside and nonexistent hours are excluded. A summary asterisk means it includes partial hours. Summary colors scale relative to the highest value in each row."),
        h("table", {
            class: "activity-table",
            "aria-label": `Recorded sessions by local date and hour, ${startLabel} to ${endLabel}`,
            "aria-describedby": "activity-summary-note",
        },
            h("thead", null, h("tr", null,
                h("th", { scope: "col" }, "Date"),
                Array.from({ length: 24 }, (_, hour) => h("th", { scope: "col" }, String(hour).padStart(2, "0"))))),
            h("tbody", null, rows),
            h("tfoot", null, activitySummaryRow("Total", totals), activitySummaryRow("Average", totals))));
    return activitySection("recorded sessions by date and hour", details);
}

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

const CLI_TOKEN_FIELDS = {
    tokens: ["InputTokens", "CacheReadTokens", "CacheWriteTokens", "OutputTokens"],
    input: ["InputTokens", "CacheReadTokens", "CacheWriteTokens"],
    cached: ["CacheReadTokens"],
    cacheWrite: ["CacheWriteTokens"],
    uncached: ["InputTokens"],
    output: ["OutputTokens"],
};

// A collapsible top-level category grouping related cards and panels.
function categorySection(name, title, hint, children) {
    return h("details", { class: "category-disclosure", [`data-${name}-disclosure`]: true, open: disclosureState[name] },
        h("summary", { class: "section-head", [`data-${name}-toggle`]: name, onclick: toggleAllDisclosures },
            h("h2", null, title),
            hint ? h("span", { class: "hint" }, hint) : null),
        ...children);
}

// Describes how the selected permission filters affect shared session usage. Agent mode
// filters restrict usage to sessions with a matching request when the CLI confirms it.
function sessionUsageFilterNote(data) {
    const filters = state?.query?.filters?.session ?? {};
    const modes = [...new Set(filters.agentMode ?? [])].sort();
    const others = Object.entries(filters).some(([name, values]) => name !== "agentMode" && values?.length);
    if (!modes.length) return others ? "permission filters do not apply" : null;
    if (!Array.isArray(data.UsageAgentModes)) {
        return "this CLI result does not restrict usage by Agent mode; permission filters do not apply";
    }
    const applied = [...new Set(data.UsageAgentModes)].sort();
    if (JSON.stringify(applied) !== JSON.stringify(modes)) {
        return "collected with a different Agent mode selection; refresh to restrict usage";
    }
    return others
        ? "limited to sessions with a matching Agent mode request; other permission filters do not apply"
        : "limited to sessions with a matching Agent mode request";
}

function renderSessionUsage(data) {
    // Older CLI builds do not report usage; keep the permission-only layout for them.
    if (data.UsageSessions === undefined) return [];
    const sessions = data.UsageSessions ?? 0;
    const total = data.UsageAIU;
    const premium = data.UsagePremiumRequests;
    const components = [data.UsageInputTokens, data.UsageCacheReadTokens, data.UsageCacheWriteTokens];
    const recordedComponents = components.filter(availableNumber);
    const prompt = recordedComponents.length ? recordedComponents.reduce((sum, value) => sum + value, 0) : null;
    const partialPrompt = recordedComponents.length !== components.length;
    const filterNote = sessionUsageFilterNote(data);
    const hints = [
        data.ByRoleUsage !== undefined ? "recorded usage · includes unfinished sessions" : "from session.shutdown events",
        filterNote,
    ].filter(Boolean).join(" · ");
    const [modelSharePanel, modelUsagePanel] = data.ByModelUsage !== undefined
        ? modelPanels((data.ByModelUsage ?? []).map(sessionUsageEntry), "session", filterNote)
        : [];

    return [categorySection("usage", "Usage", hints, [
        data.ByRoleUsage !== undefined
            ? h("p", { class: "muted small" }, data.Fusion != null
                ? "Time filters select recorded usage updates. Cumulative snapshots are differenced against earlier recorded totals and may include usage from before the selected period."
                : "Time filters select sessions by their latest recorded usage update. Totals cover those sessions' lifetime usage, not only requests made within the selected period.")
            : null,
        data.UsageIncompleteSessions > 0
            ? h("p", { class: "muted small" }, `${num(data.UsageIncompleteSessions)} sessions have incomplete model, request, or token breakdowns. Checkpoint totals are included; missing details are not inferred. Request and token counts are recorded subtotals.`)
            : null,
        h(
            "div",
            { class: "cards usage-cards" },
            card("Sessions with usage", num(sessions)),
            card("Usage (AIU)", availableNumber(total) ? fusionNumber(total) : "–", perUsageSession(total, sessions, fusionNumber)),
            card("Recorded API requests", availableNumber(data.UsageRequests) ? num(data.UsageRequests) : "–"),
            card("Premium requests", availableNumber(premium) ? fusionNumber(premium) : "–", premiumUnitText(total, premium)),
            recordedTokenCard("Prompt tokens", prompt, cacheHitText(partialPrompt ? null : data.UsageCacheReadTokens, prompt), partialPrompt),
            recordedTokenCard("Cache read", data.UsageCacheReadTokens),
            recordedTokenCard("Cache write", data.UsageCacheWriteTokens),
            recordedTokenCard("Uncached input", data.UsageInputTokens),
            recordedTokenCard("Output tokens", data.UsageOutputTokens, perUsageSession(data.UsageOutputTokens, sessions, big)),
            card("API time", availableNumber(data.UsageAPIDurationMs) ? fusionDuration(data.UsageAPIDurationMs) : "–",
                perUsageSession(data.UsageAPIDurationMs, sessions, fusionDuration)),
        ),
        h(
            "div",
            { class: "grid" },
            modelSharePanel,
            data.Fusion != null ? fusionPanel(data.Fusion) : null,
            modelUsagePanel,
            ...workspacePanels(data.ByCWDUsage ?? [], "session"),
            data.AskUser != null ? askUserPanel(data.AskUser) : null,
            data.ByRoleUsage !== undefined ? agentUsagePanel(data) : null,
        ),
    ])];
}

const AGENT_ROLE_LABELS = { main: "Main agent", subagent: "Subagents", unknown: "Unknown" };

function agentRoleLabel(key) {
    return Object.hasOwn(AGENT_ROLE_LABELS, key) ? AGENT_ROLE_LABELS[key] : key;
}

function agentSubtotalValue(entry, value, incomplete) {
    const numeric = value ?? 0;
    return entry.Key === "unknown" && incomplete && !(numeric > 0) ? null : numeric;
}

function sessionUsageEntry(entry) {
    return { ...sessionModelEntry(entry), _rawUsage: entry };
}

function agentHasFields(entry, fields) {
    return fields.every((field) => availableNumber(entry._rawUsage?.[field]));
}

function normalAgentUnitCost(entry, unit, uncertain) {
    if (uncertain || !agentHasFields(entry, ["AIU"])) return null;
    const fields = unit === "session" ? ["Sessions"] : unit === "request" ? ["Requests"]
        : CLI_TOKEN_FIELDS.tokens;
    if (!fields.every((field) => Number.isSafeInteger(entry._rawUsage?.[field]) && entry._rawUsage[field] >= 0)) return null;
    if (unit === "mtok" && !Number.isSafeInteger(entry.InputTokens + entry.OutputTokens)) return null;
    const value = unitCost(entry, unit);
    return value !== null && Number.isFinite(value) ? value : null;
}

function agentUsageColumns(incomplete = false, unitUncertain = incomplete) {
    const subtotal = (field, fields) => (entry) =>
        agentHasFields(entry, fields) ? agentSubtotalValue(entry, entry[field], incomplete) : null;
    return [
        { id: "role", label: "Role", value: (e) => String(agentRoleLabel(e.Key) ?? ""), direction: "asc" },
        { id: "sessions", label: "Sessions", value: (e) => agentHasFields(e, ["Sessions"]) ? e.Sessions : null },
        { id: "requests", label: "API requests", value: subtotal("Requests", ["Requests"]) },
        { id: "premium", label: "Premium req.", value: (e) => agentHasFields(e, ["PremiumRequests"]) ? e.PremiumRequests : null },
        { id: "prompt", label: "Prompt", value: subtotal("InputTokens", ["InputTokens", "CacheReadTokens", "CacheWriteTokens"]) },
        { id: "cacheRead", label: "Cache read", value: subtotal("CachedTokens", ["CacheReadTokens"]) },
        { id: "cacheWrite", label: "Cache write", value: subtotal("CacheWriteTokens", ["CacheWriteTokens"]) },
        { id: "uncached", label: "Uncached", value: subtotal("UncachedInputTokens", ["InputTokens"]) },
        { id: "output", label: "Output", value: subtotal("OutputTokens", ["OutputTokens"]) },
        { id: "aiu", label: "AIU", value: (e) => agentHasFields(e, ["AIU"]) ? e.UsageAIU : null },
        { id: "sessionCost", label: "AIU / session", value: (e) => normalAgentUnitCost(e, "session", unitUncertain) },
        { id: "requestCost", label: "AIU / request", value: (e) => normalAgentUnitCost(e, "request", unitUncertain) },
        { id: "tokenCost", label: "AIU / 1M tokens", value: (e) => normalAgentUnitCost(e, "mtok", unitUncertain) },
    ];
}

function normalizeAgentUsageView(saved, mode) {
    const fusionOnly = mode === "fusion";
    const columns = fusionOnly ? FUSION_AGENT_COLUMNS : agentUsageColumns();
    const view = normalizeTableView(saved, { columns, defaultSort: fusionOnly ? "aiu" : "role" });
    if (saved?.direction !== "asc" && saved?.direction !== "desc") view.direction = fusionOnly ? "desc" : "asc";
    return view;
}

function agentUsagePanel(data) {
    const incomplete = Boolean(data.UsageIncompleteSessions);
    const unitUncertain = !availableNumber(data.UsageIncompleteSessions) || incomplete;
    const columns = agentUsageColumns(incomplete, unitUncertain);
    const entries = sortTableEntries((data.ByRoleUsage ?? []).map(sessionUsageEntry), agentViews.normal, columns, () => 0);
    const cell = (entry, column) => {
        const value = column.value(entry);
        if (value === null) return h("td", null, "–");
        if (column.id === "role") return h("td", { class: "key" }, value);
        const partial = entry.Key === "unknown" && incomplete;
        if (column.id === "requests") return h("td", null, `${partial ? "≥ " : ""}${num(value)}`);
        if (["prompt", "cacheRead", "cacheWrite", "uncached", "output"].includes(column.id)) {
            const text = `${partial ? "≥ " : ""}${tokenCount(value, partial)}`;
            const title = Number.isSafeInteger(value) ? `Recorded${partial ? " subtotal" : ""}: ${num(value)} tokens` : null;
            return h("td", { title, "aria-label": title ? `${text}; ${title}` : null }, text);
        }
        return h("td", null, column.id === "sessions" ? num(value) : fusionNumber(value));
    };
    return panel("Agent usage", "main / subagent / unknown", [
        h("p", { class: "muted small" }, "Attribution uses recorded request metadata, not model names. Unknown includes usage that cannot be attributed. For incomplete breakdowns, ≥ marks a recorded subtotal and – means unavailable. Unit costs use recorded totals and require available inputs with no reported incomplete session breakdowns. Click a heading to sort."),
        entries.length
            ? h("table", null,
                  h("thead", null, h("tr", null,
                      columns.map((column) => agentUsageHeader(column, "normal", columns)))),
                  h("tbody", null, entries.map((entry) => h("tr", null,
                      columns.map((column) => cell(entry, column))))))
            : h("div", { class: "muted small" }, "No recorded agent usage."),
    ], { wide: true });
}

const FUSION_PHASE_COLUMNS = [
    { id: "pattern", label: "Pattern", value: (e) => e.Pattern ?? "", direction: "asc" },
    { id: "phase", label: "Phase", value: (e) => e.Phase ?? "", direction: "asc" },
    { id: "role", label: "Role", value: (e) => e.Role ?? "", direction: "asc" },
    { id: "model", label: "Usage model", value: (e) => e.Model || "Unknown model", direction: "asc" },
    { id: "sessions", label: "Sessions", value: (e) => e.Sessions ?? 0 },
    { id: "turns", label: "Turns", value: (e) => e.Turns ?? 0 },
    { id: "phases", label: "Phases", value: (e) => e.Phases ?? 0 },
    { id: "records", label: "Records", value: (e) => e.UsageRecords ?? 0 },
    { id: "aiu", label: "AIU", value: (e) => e.AIURecords > 0 && Number.isFinite(e.AIU) ? e.AIU : null },
    { id: "coverage", label: "AIU coverage", value: (e) => e.UsageRecords > 0 && Number.isFinite(e.AIURecords) ? e.AIURecords / e.UsageRecords : null },
    { id: "missingIdentity", label: "Missing phase identity", value: (e) => e.MissingPhaseIdentityRecords ?? 0 },
];

function normalizeFusionPhaseView(saved) {
    return normalizeTableView(saved, { columns: FUSION_PHASE_COLUMNS, defaultSort: "records" });
}

function toggleFusionPhaseSort(view, columnId) {
    return toggleTableSort(view, columnId, FUSION_PHASE_COLUMNS);
}

function sortFusionPhases(entries, view) {
    // Equal values retain the CLI's original phase order.
    return sortTableEntries(entries, view, FUSION_PHASE_COLUMNS, () => 0);
}

function fusionNumber(value) {
    return aiu(value);
}

function fusionLowerBoundNumber(value) {
    const scaled = value * 100;
    const lowerBound = Number.isFinite(scaled) ? Math.floor(scaled + Number.EPSILON * Math.abs(scaled)) / 100 : value;
    const formatted = aiuFmt.format(lowerBound);
    return Number(formatted.replaceAll(",", "")) > value ? aiuFmt.format(Math.max(0, lowerBound - 0.01)) : formatted;
}

const fusionDurationFmt = new Intl.NumberFormat("en", { maximumFractionDigits: 9 });

function fusionDurationNumber(value) {
    return value !== 0 && Math.abs(value) < 1e-9 ? value.toExponential(3) : fusionDurationFmt.format(value);
}

function fusionLowerBoundDurationNumber(value) {
    const formatted = fusionDurationNumber(value);
    return Number(formatted.replaceAll(",", "")) > value ? String(value) : formatted;
}

function fusionDuration(ms, partial = false) {
    if (partial) {
        if (ms < 1) return `${fusionLowerBoundDurationNumber(ms)} ms`;
        if (ms < 1000) return `${Math.floor(ms)} ms`;
        const [scale, unit] = ms < 60_000 ? [1000, "s"] : ms < 3_600_000 ? [60_000, "min"] : [3_600_000, "h"];
        const value = Math.floor(ms / scale * 10) / 10;
        return Number.isFinite(value) && value * scale <= ms
            ? `${value.toFixed(1)} ${unit}` : `${fusionLowerBoundDurationNumber(ms)} ms`;
    }
    return ms > 0 && ms < 1 ? `${fusionDurationNumber(ms)} ms` : duration(ms);
}

function fusionAIU(entry) {
    if (!(entry.AIURecords > 0)) return "–";
    const partial = entry.AIURecords < entry.UsageRecords;
    return `${partial ? "≥ " : ""}${partial ? fusionLowerBoundNumber(entry.AIU) : fusionNumber(entry.AIU)}`;
}

function fusionPanel(fusion, usageOnly = false) {
    const patterns = fusion.ByPattern ?? [];
    const phases = sortFusionPhases(fusion.ByPhase ?? [], fusionPhaseView);
    return disclosurePanel("fusion", "Copilot Fusion", usageOnly ? "tagged usage only" : "recorded events only", [
        h("p", { class: "muted small" }, usageOnly
            ? "Only explicitly Fusion-tagged usage records are included. Phase/model rows may share turns or phases; Top limits rows, not totals."
            : "Resolution and usage timestamps are filtered separately. Permission filters do not apply; Top limits rows, not totals. Planned phases and checkpoint-only usage are not attributed. Phase/model rows may share turns or phases. Active sessions can change during scanning, so totals are not an atomic snapshot."),
        h("div", { class: "cards fusion-cards" },
            card(usageOnly ? "Sessions with usage" : "Sessions with records", num(fusion.Sessions)),
            usageOnly ? null : card("Resolved turns", num(fusion.ResolvedTurns), fusion.UnidentifiedResolutions ? `${num(fusion.UnidentifiedResolutions)} resolutions without an ID` : null),
            card("Turns with usage", num(fusion.TurnsWithUsage)),
            card("Phases with usage", num(fusion.PhasesWithUsage)),
            card("Usage records", num(fusion.UsageRecords)),
            card("Recorded AIU", fusionAIU(fusion), `${num(fusion.AIURecords)} / ${num(fusion.UsageRecords)} records report AIU`)),
        fusion.MissingFusionIDRecords || fusion.MissingPhaseIdentityRecords
            ? h("p", { class: "muted small" }, `${num(fusion.MissingFusionIDRecords)} usage records lack a Fusion ID; ${num(fusion.MissingPhaseIdentityRecords)} lack a complete phase identity. They are excluded from distinct turn or phase counts as applicable.`)
            : null,
        !fusion.Sessions ? h("p", { class: "muted small" }, usageOnly
            ? "No Fusion-tagged usage records were found in the selected window. Missing tags cannot establish whether Fusion was used."
            : "No Fusion metadata was recorded in the selected window. Older or incomplete logs cannot establish whether Fusion was used.") : null,
        usageOnly ? null : h("h3", null, "Resolved patterns"),
        usageOnly ? null : patterns.length
            ? h("table", null,
                  h("thead", null, h("tr", null, ["Pattern", "Sessions", "Resolved turns", "Without ID"].map((label) => h("th", { scope: "col" }, tableHeading(label))))),
                  h("tbody", null, patterns.map((entry) => h("tr", null,
                      h("td", { class: "key" }, entry.Key), h("td", null, num(entry.Sessions)),
                      h("td", null, num(entry.ResolvedTurns)), h("td", null, num(entry.UnidentifiedResolutions))))))
            : h("div", { class: "muted small" }, "No recorded pattern resolutions."),
        h("h3", null, "Phase usage"),
        phases.length
            ? h("p", { class: "muted small" }, "Click a heading to sort the listed rows. Top selects phase groups by record count before sorting; set Top to 0 for all groups. AIU coverage sorts by the fraction of records reporting AIU.")
            : null,
        phases.length
            ? h("table", null,
                  h("thead", null, h("tr", null,
                      FUSION_PHASE_COLUMNS.map(fusionPhaseHeader))),
                  h("tbody", null, phases.map((entry) => h("tr", null,
                      h("td", { class: "key" }, entry.Pattern), h("td", null, entry.Phase),
                      h("td", null, entry.Role), h("td", null, entry.Model || "Unknown model"),
                      h("td", null, num(entry.Sessions)), h("td", null, num(entry.Turns)),
                      h("td", null, num(entry.Phases)), h("td", null, num(entry.UsageRecords)),
                      h("td", null, fusionAIU(entry)),
                      h("td", null, `${num(entry.AIURecords)} / ${num(entry.UsageRecords)}`),
                      h("td", null, num(entry.MissingPhaseIdentityRecords))))))
            : h("div", { class: "muted small" }, "No recorded Fusion usage."),
    ]);
}

function fusionMetricText(metric, records, format) {
    if (!metric.ObservedRecords) return "–";
    const partial = metric.CompleteRecords + metric.ExcludedRecords < records;
    const value = partial && format === fusionNumber ? fusionLowerBoundNumber(metric.Total) : format(metric.Total, partial);
    return `${partial ? "≥ " : ""}${value}`;
}

function fusionMetricCoverage(metric, records) {
    const applicable = records - metric.ExcludedRecords;
    const complete = `${num(metric.CompleteRecords)} / ${num(applicable)} records complete` +
        (metric.ExcludedRecords ? ` · ${num(metric.ExcludedRecords)} excluded` : "");
    return metric.ObservedRecords !== metric.CompleteRecords
        ? `${complete} · ${num(metric.ObservedRecords)} have partial or complete values`
        : complete;
}

function fusionMetricTooltip(metric, records, format) {
    const coverage = fusionMetricCoverage(metric, records);
    return format === tokenCount && metric.ObservedRecords
        ? `Recorded: ${num(metric.Total)} tokens · ${coverage}`
        : coverage;
}

function fusionMetricCard(label, metric, records, format, detail) {
    const sub = [detail, fusionMetricCoverage(metric, records)].filter(Boolean).join(" · ");
    return card(label, fusionMetricText(metric, records, format), sub,
        format === tokenCount ? fusionMetricTooltip(metric, records, format) : null);
}

function fusionMetricComplete(metric, records) {
    return records > 0 && metric.ObservedRecords > 0 && metric.CompleteRecords + metric.ExcludedRecords === records;
}

function fusionUnitCost(entry, unit) {
    const metrics = entry.Metrics;
    const records = entry.UsageRecords;
    const complete = (metric) => fusionMetricComplete(metric, records);
    if (!complete(metrics.AIU)) return null;
    let value;
    if (unit === "session") value = entry.Sessions > 0 ? metrics.AIU.Total / entry.Sessions : null;
    else if (unit === "record") value = metrics.AIU.Total / records;
    else {
        if (!complete(metrics.PromptTokens) || !complete(metrics.OutputTokens)) return null;
        const tokens = metrics.PromptTokens.Total + metrics.OutputTokens.Total;
        value = Number.isSafeInteger(tokens) && tokens > 0 ? (metrics.AIU.Total / tokens) * 1e6 : null;
    }
    return availableNumber(value) ? value : null;
}

function fusionTokenBreakdown(entry) {
    const { Metrics: metrics, UsageRecords: records } = entry;
    const fields = ["CacheReadTokens", "CacheWriteTokens", "UncachedInputTokens", "OutputTokens"];
    if (!fusionMetricComplete(metrics.PromptTokens, records) ||
        !fields.every((field) => fusionMetricComplete(metrics[field], records))) return null;
    const values = fields.map((field) => metrics[field].Total);
    const prompt = values[0] + values[1] + values[2];
    const total = metrics.PromptTokens.Total + values[3];
    if (!Number.isSafeInteger(prompt) || prompt !== metrics.PromptTokens.Total || !Number.isSafeInteger(total)) return null;
    return { values, total };
}

function fusionCacheHit(entry) {
    const { Metrics: metrics, UsageRecords: records } = entry;
    const prompt = metrics.PromptTokens;
    const cache = metrics.CacheReadTokens;
    if (!fusionMetricComplete(prompt, records) || !fusionMetricComplete(cache, records) ||
        !prompt.Total || cache.Total > prompt.Total) return null;
    return cache.Total / prompt.Total;
}

const FUSION_USAGE_METRICS = [
    { id: "premium", label: "Premium req.", field: "PremiumRequests", format: fusionNumber },
    { id: "prompt", label: "Prompt", field: "PromptTokens", format: tokenCount },
    { id: "cacheRead", label: "Cache read", field: "CacheReadTokens", format: tokenCount },
    { id: "cacheWrite", label: "Cache write", field: "CacheWriteTokens", format: tokenCount },
    { id: "uncached", label: "Uncached", field: "UncachedInputTokens", format: tokenCount },
    { id: "output", label: "Output", field: "OutputTokens", format: tokenCount },
    { id: "apiTime", label: "API time", field: "APIDurationMs", format: fusionDuration },
    { id: "aiu", label: "AIU", field: "AIU", format: fusionNumber },
];

const FUSION_MODEL_UNITS = {
    session: { label: "Session", header: "AIU / session" },
    record: { label: "Usage record", header: "AIU / record" },
    mtok: { label: "1M tokens", header: "AIU / 1M tokens" },
};

const FUSION_METRIC_COLUMNS = Object.fromEntries(FUSION_USAGE_METRICS.map((metric) => [
    metric.id,
    { ...metric, value: (e) => e.Metrics[metric.field].ObservedRecords > 0 ? e.Metrics[metric.field].Total : null },
]));

function fusionUsageColumns(breakdown) {
    return [
        { id: "sessions", label: "Sessions", value: (e) => e.Sessions },
        { id: "records", label: "Usage records", value: (e) => e.UsageRecords },
        FUSION_METRIC_COLUMNS.premium,
        ...(breakdown ? [{ id: "tokens", label: "Tokens", value: (e) => fusionTokenBreakdown(e)?.total ?? null }] : []),
        FUSION_METRIC_COLUMNS.prompt,
        FUSION_METRIC_COLUMNS.cacheRead,
        FUSION_METRIC_COLUMNS.cacheWrite,
        FUSION_METRIC_COLUMNS.uncached,
        FUSION_METRIC_COLUMNS.output,
        ...(breakdown ? [{ id: "cacheHit", label: "Cache hit", value: fusionCacheHit }] : []),
        FUSION_METRIC_COLUMNS.apiTime,
        FUSION_METRIC_COLUMNS.aiu,
    ];
}

const FUSION_MODEL_COLUMNS = [
    { id: "model", label: "Model", value: (e) => e.Key || "Unknown model", direction: "asc" },
    ...fusionUsageColumns(true),
    { id: "unit", label: "Unit cost", value: (e, unit) => fusionUnitCost(e, unit) },
];

const FUSION_WORKSPACE_COLUMNS = [
    { id: "workspace", label: "Workspace", value: (e) => e.Key || "Unknown workspace", direction: "asc" },
    ...fusionUsageColumns(true),
    { id: "unit", label: "Unit cost", value: (e, unit) => fusionUnitCost(e, unit) },
];

function normalizeFusionModelView(saved) {
    return normalizeTableView(saved, {
        units: FUSION_MODEL_UNITS, columns: FUSION_MODEL_COLUMNS, defaultUnit: "record", defaultSort: "aiu",
    });
}

function normalizeFusionWorkspaceView(saved) {
    return normalizeTableView(saved, {
        units: FUSION_MODEL_UNITS, columns: FUSION_WORKSPACE_COLUMNS, defaultUnit: "record", defaultSort: "aiu",
    });
}

const FUSION_AGENT_COLUMNS = [
    { id: "role", label: "Role", value: (e) => String(agentRoleLabel(e.Key) || "(empty)"), direction: "asc" },
    ...fusionUsageColumns(false),
    { id: "sessionCost", label: "AIU / session", value: (e) => fusionUnitCost(e, "session") },
    { id: "recordCost", label: "AIU / record", value: (e) => fusionUnitCost(e, "record") },
    { id: "tokenCost", label: "AIU / 1M tokens", value: (e) => fusionUnitCost(e, "mtok") },
];

function fusionUsageTable(entries, kind) {
    if (!entries.length) return h("div", { class: "muted small" }, "No Fusion-tagged usage records.");
    const roles = kind === "role";
    const models = kind === "model";
    const workspaces = kind === "workspace";
    const view = roles ? agentViews.fusion : models ? fusionModelView : fusionWorkspaceView;
    const columns = roles ? FUSION_AGENT_COLUMNS : models ? FUSION_MODEL_COLUMNS : FUSION_WORKSPACE_COLUMNS;
    const rows = roles ? sortTableEntries(entries, view, columns, () => 0) : sortTableEntries(entries, view, columns);
    const maxUnit = models || workspaces ? Math.max(...rows.map((entry) => fusionUnitCost(entry, view.unit) ?? 0), Number.MIN_VALUE) : null;
    const maxTokens = rows.reduce((max, entry) => Math.max(max, fusionTokenBreakdown(entry)?.total ?? 0), 1);
    const headers = roles
        ? FUSION_AGENT_COLUMNS.map((column) => agentUsageHeader(column, "fusion", FUSION_AGENT_COLUMNS))
        : [
            ...columns.map((column) => fusionUsageHeader(column, kind)),
            h("th", { scope: "col", "aria-label": "Unit cost comparison" }),
        ];
    return h("table", null,
        h("thead", null, h("tr", null, headers)),
        h("tbody", null, rows.map((entry) => {
            const display = workspaces ? (entry.Key ? shortPath(entry.Key) : "Unknown workspace")
                : roles ? agentRoleLabel(entry.Key) : (entry.Key || "Unknown model");
            const onPick = workspaces && entry.Key ? (key) => patchQuery({ scope: "cwd", path: key }) : null;
            const cell = (column) => {
                if (["model", "workspace", "role"].includes(column.id)) {
                    return h("td", { class: "key" }, keyCell(entry.Key, onPick, "Show only this directory", display));
                }
                const value = column.value(entry, view.unit);
                if (column.id === "tokens") {
                    const breakdown = fusionTokenBreakdown(entry);
                    if (!breakdown) {
                        const title = "Token breakdown requires complete and consistent prompt, cache, uncached input, and output totals.";
                        return h("td", { title, "aria-label": `Unavailable; ${title}` }, "–");
                    }
                    const descriptions = USAGE_TOKEN_COLORS.map(([label], index) => `${label}: ${num(breakdown.values[index])}`);
                    const segments = USAGE_TOKEN_COLORS.map(([, color], index) => [breakdown.values[index], color, descriptions[index]]);
                    return h("td", { title: descriptions.join(" · "), "aria-label": descriptions.join("; ") }, stackedBar(segments, maxTokens));
                }
                if (column.id === "cacheHit") {
                    const title = value === null
                        ? "Cache hit requires complete and consistent cache read and prompt totals."
                        : `${num(entry.Metrics.CacheReadTokens.Total)} / ${num(entry.Metrics.PromptTokens.Total)} prompt tokens`;
                    return h("td", { title }, value === null ? "–" : pct(value, 1));
                }
                if (column.field) {
                    const metric = entry.Metrics[column.field];
                    const text = fusionMetricText(metric, entry.UsageRecords, column.format);
                    const title = fusionMetricTooltip(metric, entry.UsageRecords, column.format);
                    return h("td", { title, "aria-label": column.format === tokenCount ? `${text}; ${title}` : null }, text);
                }
                if (value === null) return h("td", null, "–");
                return h("td", null, ["sessions", "records"].includes(column.id) ? num(value) : fusionNumber(value));
            };
            return h("tr", null,
                columns.map(cell),
                models || workspaces ? h("td", null, unitCostBar(
                    fusionUnitCost(entry, view.unit),
                    FUSION_MODEL_UNITS[view.unit].header, maxUnit, fusionNumber,
                )) : null);
        })));
}

function fusionUsageBreakdown(title, entries, total, kind) {
    const hint = entries.length < total ? `${num(entries.length)} of ${num(total)} groups` : `${num(total)} groups`;
    return panel(title, hint, [
        h("p", { class: "muted small" }, "≥ marks a recorded subtotal; – means unavailable. Unit costs require complete inputs. Hover over a metric for coverage and exact token counts."),
        kind === "role" && entries.length
            ? h("p", { class: "muted small" }, "Click a heading to sort the listed rows. Top selects role groups by recorded AIU before sorting; set Top to 0 for all groups.")
            : null,
        kind === "model" || kind === "workspace" ? [
            h("p", { class: "muted small" }, "Token bars require complete, consistent component totals; cache hit requires complete prompt and cache-read totals."),
            h("p", { class: "muted small" }, kind === "model"
                ? "Select a unit cost to compare models. Usage records are not API requests. Click a heading to sort; Top selects models by recorded AIU before sorting. Set Top to 0 for all models."
                : "Select a unit cost to compare workspaces. Usage records are not API requests. Click a heading to sort; Top selects workspaces by recorded AIU before sorting. Set Top to 0 for all workspaces."),
            h("div", { class: "panel-controls" },
                legend(USAGE_TOKEN_COLORS.map(([label, color]) => [label, color])),
                unitCostControls(FUSION_MODEL_UNITS, kind === "model" ? fusionModelView : fusionWorkspaceView,
                    (patch) => kind === "model"
                        ? updateView(fusionModelView, FUSION_MODEL_VIEW_KEY, patch)
                        : updateView(fusionWorkspaceView, FUSION_WORKSPACE_VIEW_KEY, patch))),
        ] : null,
        fusionUsageTable(entries, kind),
    ], { wide: true, ...(kind === "workspace" ? { filter: "workspace" } : {}) });
}

function renderFusionOnly(data) {
    const usage = data.FusionUsage;
    const metrics = usage.Metrics;
    const records = usage.UsageRecords;
    const completeValue = (metric) => fusionMetricComplete(metric, records) ? metric.Total : null;
    const usageShare = (entries, label) => shareBar(
        entries,
        (entry) => entry.Metrics.AIU.ObservedRecords ? entry.Metrics.AIU.Total : 0,
        label,
        {
            formatValue: (_value, entry) => fusionMetricText(entry.Metrics.AIU, entry.UsageRecords, fusionNumber),
            formatShare: usageSharePercent,
        },
    );
    const modelShare = usageShare(usage.ByModel, (entry) => entry.Key || "Unknown model");
    const workspaceShare = usageShare(usage.ByCWD, (entry) => entry.Key ? shortPath(entry.Key) : "Unknown workspace");
    return [categorySection("fusion-usage", "Fusion usage", "explicitly tagged records only", [
        h("p", { class: "muted small" }, "Usage records are not necessarily distinct API calls. Resolution events, permission data, and checkpoint-only usage are excluded. ≥ marks a recorded subtotal; – means unavailable. Hover over token totals for exact recorded counts. Premium requests sum cost multipliers for user or unspecified initiators; other initiators are excluded, following the shared collector."),
        h("div", { class: "cards fusion-cards" },
            card("Sessions with usage", num(usage.Sessions)),
            fusionMetricCard("Usage (AIU)", metrics.AIU, records, fusionNumber,
                perUsageSession(completeValue(metrics.AIU), usage.Sessions, fusionNumber)),
            card("Usage records", num(records)),
            fusionMetricCard("Premium requests", metrics.PremiumRequests, records, fusionNumber,
                premiumUnitText(completeValue(metrics.AIU), completeValue(metrics.PremiumRequests))),
            fusionMetricCard("Prompt tokens", metrics.PromptTokens, records, tokenCount,
                cacheHitText(completeValue(metrics.CacheReadTokens), completeValue(metrics.PromptTokens))),
            fusionMetricCard("Cache read", metrics.CacheReadTokens, records, tokenCount),
            fusionMetricCard("Cache write", metrics.CacheWriteTokens, records, tokenCount),
            fusionMetricCard("Uncached input", metrics.UncachedInputTokens, records, tokenCount),
            fusionMetricCard("Output tokens", metrics.OutputTokens, records, tokenCount,
                perUsageSession(completeValue(metrics.OutputTokens), usage.Sessions, big)),
            fusionMetricCard("API time", metrics.APIDurationMs, records, fusionDuration,
                perUsageSession(completeValue(metrics.APIDurationMs), usage.Sessions, fusionDuration))),
        h("div", { class: "grid" },
            panel("Usage share by model", "recorded AIU · listed models", [
                h("p", { class: "muted small" }, "Shares compare recorded AIU subtotals for listed models. Missing AIU is excluded, so incomplete coverage can affect proportions."),
                modelShare ?? h("div", { class: "muted small" }, "No positive recorded model AIU to compare."),
            ], { wide: true }),
            fusionPanel(data.Fusion, true),
            fusionUsageBreakdown("Models", usage.ByModel, usage.ModelGroups, "model"),
            panel("Usage share by workspace", "recorded AIU · listed workspaces", [
                h("p", { class: "muted small" }, "Shares compare recorded AIU subtotals for listed workspaces. Missing AIU is excluded, so incomplete coverage can affect proportions."),
                workspaceShare ?? h("div", { class: "muted small" }, "No positive recorded workspace AIU to compare."),
            ], { wide: true, filter: "workspace" }),
            fusionUsageBreakdown("Workspaces", usage.ByCWD, usage.CWDGroups, "workspace"),
            fusionUsageBreakdown("Agent usage", usage.ByRole, usage.RoleGroups, "role")),
    ])];
}

function renderSession(data) {
    if (data.FusionOnly) return renderFusionOnly(data);
    const totals = { Approved: 0, ApprovedForLocation: 0, Denied: 0, Unresolved: 0 };
    for (const e of data.ByKind ?? []) for (const k of Object.keys(totals)) totals[k] += e[k] ?? 0;
    const approved = totals.Approved + totals.ApprovedForLocation;
    const requests = data.Requests ?? 0;
    // ByKind is truncated to --top; totals are exact only when it covers every request.
    const covered = (data.ByKind ?? []).reduce((n, e) => n + (e.Total ?? 0), 0);
    const partial = covered < requests ? ` · listed kinds only (${num(covered)} of ${num(requests)})` : "";
    const cwdPick = (key) => patchQuery({ scope: "cwd", path: key });
    const modeFilterSupported = meta?.filters?.session?.includes("agentMode") === true;

    return [
        ...renderSessionUsage(data),
        data.UsageSessions === undefined && data.Fusion != null ? h("div", { class: "grid" }, fusionPanel(data.Fusion)) : null,
        data.UsageSessions === undefined && data.AskUser != null
            ? h("div", { class: "grid" }, askUserPanel(data.AskUser))
            : null,
        categorySection("permissions", "Permissions", "permission requests", [h(
            "div",
            { class: "grid" },
            panel("Summary", null, [
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
                data.PermissionWait != null ? permissionWaitDetails(data.PermissionWait) : null,
            ], { wide: true }),
            data.ByAgentMode != null
                ? panel("Agent modes", modeFilterSupported ? "click to filter" : "permission requests", h("div", null,
                    h("p", { class: "muted small" }, "Recorded at permission evaluation, not main/subagent roles. Missing modes are unknown. Only logged requests are counted; permission filters and Top apply."),
                    !modeFilterSupported
                        ? h("p", { class: "muted small" }, "Reload the dashboard extension to enable Agent mode filtering.")
                        : null,
                    decisionRows(data.ByAgentMode, {
                        onPick: modeFilterSupported ? (k) => addFilter("agentMode", k) : undefined,
                        pickTitle: "Filter by this Agent mode",
                    })), { filter: modeFilterSupported ? "agentMode" : undefined })
                : null,
            panel("Results", "click to filter", decisionRows(data.ByResult, {
                onPick: (k) => addFilter("result", k),
                pickTitle: "Filter by this exact result",
            }), { filter: "result" }),
            data.ByDecisionSource
                ? panel(
                      "Decision sources",
                      "click to filter",
                      decisionRows(data.ByDecisionSource, { onPick: (k) => addFilter("decisionSource", k), pickTitle: "Filter by this decision source" }),
                      { filter: "decisionSource" },
                  )
                : null,
            panel("Read-only vs read-write", "click to filter", decisionRows(data.ByReadOnly, {
                onPick: (k) => addFilter("commandAccess", k),
                pickTitle: "Filter requests containing commands with this access",
            }), { filter: "commandAccess" }),
            panel("Tool kinds", "click to filter", decisionRows(data.ByKind, { onPick: (k) => addFilter("kind", k), pickTitle: "Filter by this kind" }), { filter: "kind" }),
            panel("Commands", "click to filter", decisionRows(data.ByCommand, { onPick: (k) => addFilter("command", k), pickTitle: "Filter by this command" }), { filter: "command" }),
            panel("Paths", "click to filter", decisionRows(data.ByPath, { onPick: (k) => addFilter("path", exactPattern(k)), pickTitle: "Filter by this path" }), { wide: true, filter: "path" }),
            panel("URLs", "click to filter", decisionRows(data.ByURL, { onPick: (k) => addFilter("url", exactPattern(k)), pickTitle: "Filter by this URL" }), { wide: true, filter: "url" }),
            panel("Working directories", "click to scope", decisionRows(data.ByCWD, { onPick: cwdPick, pickTitle: "Show only this directory", display: shortPath }), { wide: true, filter: "workspace" }),
        )]),
        h("div", { class: "grid" }, activityPanel(data.SessionActivity)),
    ];
}

function permissionWaitDetails(wait) {
    const human = wait.HumanResponses;
    return h("div", null,
        h("h3", null, "Permission response wait"),
        h("p", { class: "muted small" }, "Only recorded human_response decisions are confirmed user wait. Other and unknown-source decision times stay separate. Permission filters and request start-time bounds apply; Top does not. Later completions still resolve requests. Intervals include UI and transport overhead; overlapping waits are summed, not unique wall-clock idle time. Unresolved requests have no inferred duration."),
        h("div", { class: "cards" },
            card("Human responses", num(human.Completed), `${num(human.Timed)} timed · ${num(human.Untimed)} untimed`),
            card("Total permission wait", duration(human.TotalWaitMs)),
            card("Average permission wait", duration(human.AvgWaitMs)),
            card("Longest permission wait", duration(human.MaxWaitMs)),
            card("No recorded decision", num(wait.Unresolved), "excluded from timing")),
        h("table", null,
            h("thead", null, h("tr", null,
                ["Other decision timing", "Completed", "Timed", "Untimed", "Total", "Average", "Longest"]
                    .map((label) => h("th", { scope: "col" }, tableHeading(label))))),
            h("tbody", null, [
                ["Other recorded sources", wait.NonHumanDecisions],
                ["Unknown source", wait.UnknownSourceDecisions],
            ].map(([label, entry]) => h("tr", null,
                h("th", { class: "key", scope: "row" }, label), h("td", null, num(entry.Completed)),
                h("td", null, num(entry.Timed)), h("td", null, num(entry.Untimed)),
                h("td", null, duration(entry.TotalWaitMs)), h("td", null, duration(entry.AvgWaitMs)),
                h("td", null, duration(entry.MaxWaitMs)))))),
        human.Untimed + wait.NonHumanDecisions.Untimed + wait.UnknownSourceDecisions.Untimed > 0
            ? h("p", { class: "muted small" }, "Completed decisions with missing or invalid timestamps are counted as untimed and excluded from durations.")
            : null);
}

function askUserPanel(q, source = "session") {
    const vscode = source === "vscode";
    return panel("User questions", vscode ? "vscode_askQuestions · completed tool calls" : "ask_user · tool start to completion", [
        h("p", { class: "muted small" }, vscode
            ? "Each multi-question batch counts as one call and shares one wait interval. Partially skipped batches with an answer count as answered. Only recorded completed calls are available; in-flight questions cannot be reconstructed. Time bounds select call starts. Agent and tool filters apply; model filters and Top do not. Wait includes UI and transport overhead; overlapping intervals are summed, not unique wall-clock idle time."
            : "Time bounds select question starts; answers completed later still resolve them. Permission filters do not apply. Wait includes UI and transport overhead. Totals sum per-question intervals, not unique wall-clock idle time."),
        h("div", { class: "cards" },
            card("Sessions with questions", num(q.Sessions)),
            card(vscode ? "Question tool calls" : "Questions", num(vscode ? q.Calls : q.Questions)),
            card("Answered", num(q.Answered)),
            vscode ? card("Skipped", num(q.Skipped)) : null,
            card("Cancelled", num(q.Cancelled)),
            card("Failed", num(q.Failed)),
            vscode ? null : card("Unresolved", num(q.Unresolved)),
            card("Unknown outcome", num(q.Unknown)),
            card("Total answer wait", duration(q.TotalWaitMs), `${num(q.TimedAnswers)} timed answers`),
            card("Average answer wait", duration(q.AvgWaitMs)),
            card("Longest answer wait", duration(q.MaxWaitMs))),
        q.UntimedAnswers > 0
            ? h("p", { class: "muted small" }, `${num(q.UntimedAnswers)} answered ${vscode ? "calls have missing or invalid duration" : "questions have missing or invalid timestamps"} and are excluded from wait metrics.`)
            : null,
        q.Unknown > 0
            ? h("p", { class: "muted small" }, "Unknown outcomes have a completion but no recognized answer/cancellation result. They are excluded from answer wait metrics.")
            : null,
    ], { wide: true });
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
        h("thead", null, h("tr", null, h("th", null, "Tool"), h("th", null, "Calls"), h("th", null, ""), h("th", null, "Errors"), h("th", null, "Avg"), h("th", null, tableHeading("Total time")))),
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
const AGENT_VIEW_KEYS = {
    normal: "attendant-dashboard.agentView",
    fusion: "attendant-dashboard.fusionAgentView",
};
const FUSION_PHASE_VIEW_KEY = "attendant-dashboard.fusionPhaseView";
const FUSION_MODEL_VIEW_KEY = "attendant-dashboard.fusionModelView";
const FUSION_WORKSPACE_VIEW_KEY = "attendant-dashboard.fusionWorkspaceView";

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
const agentViews = Object.fromEntries(Object.entries(AGENT_VIEW_KEYS).map(([mode, key]) => [mode, loadView(key, normalizeAgentUsageView, mode)]));
const fusionPhaseView = loadView(FUSION_PHASE_VIEW_KEY, normalizeFusionPhaseView);
const fusionModelView = loadView(FUSION_MODEL_VIEW_KEY, normalizeFusionModelView);
const fusionWorkspaceView = loadView(FUSION_WORKSPACE_VIEW_KEY, normalizeFusionWorkspaceView);

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
    return value === null ? "–" : aiu(value);
}

function unitCostBar(value, label, max, format = unitCostText) {
    return stackedBar([[value ?? 0, "var(--c-unit-cost)", `${label}: ${value === null ? "–" : format(value)}`]], max);
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
        unitCostControls(units, view, onChange),
    );
}

function unitCostControls(units, view, onChange) {
    return h("div", { class: "controls" },
        segmented("Unit cost per", Object.fromEntries(Object.entries(units).map(([key, unit]) => [key, unit.label])),
            view.unit, (unit) => onChange({ unit })));
}

function modelHeader(column, source) {
    const view = modelViews[source];
    const label = column.id === "unit" ? MODEL_UNITS[view.unit].header : column.label;
    const next = toggleModelSort(view, column.id, source);
    return sortableHeader(column, { view, label, next, attribute: "data-model-sort", onChange: (patch) => setModelView(source, patch) });
}

function fusionPhaseHeader(column) {
    return sortableHeader(column, {
        view: fusionPhaseView, label: column.label, next: toggleFusionPhaseSort(fusionPhaseView, column.id),
        attribute: "data-fusion-phase-sort", onChange: (patch) => updateView(fusionPhaseView, FUSION_PHASE_VIEW_KEY, patch),
    });
}

function fusionUsageHeader(column, kind) {
    const model = kind === "model";
    const view = model ? fusionModelView : fusionWorkspaceView;
    const key = model ? FUSION_MODEL_VIEW_KEY : FUSION_WORKSPACE_VIEW_KEY;
    const columns = model ? FUSION_MODEL_COLUMNS : FUSION_WORKSPACE_COLUMNS;
    const attribute = model ? "data-fusion-model-sort" : "data-fusion-workspace-sort";
    const label = column.id === "unit" ? FUSION_MODEL_UNITS[view.unit].header : column.label;
    return sortableHeader(column, {
        view, label, next: toggleTableSort(view, column.id, columns),
        attribute, onChange: (patch) => updateView(view, key, patch),
    });
}

function agentUsageHeader(column, mode, columns) {
    const view = agentViews[mode];
    return sortableHeader(column, {
        view, label: column.label, next: toggleTableSort(view, column.id, columns),
        attribute: "data-agent-sort", onChange: (patch) => updateView(view, AGENT_VIEW_KEYS[mode], patch),
    });
}

function tableHeading(label) {
    const words = label.split(" ");
    if (words.length < 2) return label;
    // Two unbroken phrases allow at most one line break without truncating labels.
    const split = Math.ceil(words.length / 2);
    return h("span", { class: "table-heading" },
        h("span", { class: "table-heading-part" }, words.slice(0, split).join(" ")),
        " ",
        h("span", { class: "table-heading-part" }, words.slice(split).join(" ")));
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
            tableHeading(label),
            h("span", { class: "sort-indicator", "aria-hidden": "true" }, selected ? (view.direction === "asc" ? "▲" : "▼") : "↕"),
        ),
    );
}

function modelPanels(entries, source, filterNote = null) {
    const share = source === "session"
        ? shareBar(entries, (entry) => entry.UsageAIU ?? 0, undefined, { formatValue: fusionNumber, formatShare: usageSharePercent })
        : aiuShare(entries);
    const session = source === "session";
    const note = session
        ? "Recorded modelMetrics and request usage are included; checkpoints may lack a model breakdown. Shares cover listed models. A session may use multiple models." +
            (filterNote ? ` Filters: ${filterNote}.` : "")
        : null;
    const missingSessions = entries.some((e) => e.Sessions == null);
    return [
        share ? panel("Usage share by model", session ? "AIU · listed models" : "AIU", share, { wide: true, filter: session ? undefined : "model" }) : null,
        panel("Models", session ? "recorded model usage · API requests" : "click to filter", [
            note ? h("p", { class: "muted small" }, note) : null,
            missingSessions ? h("p", { class: "muted small" }, "Session counts are not reported for some models; their AIU / session is unavailable.") : null,
            modelControls(source),
            modelTable(entries, source),
        ], { wide: true, filter: session ? undefined : "model" }),
    ];
}

function usageTokenSegments(e, source) {
    if (source !== "session") return vscodeTokenSegments(e);
    const fields = ["CachedTokens", "CacheWriteTokens", "UncachedInputTokens", "OutputTokens"];
    return USAGE_TOKEN_COLORS.map(([label, color], index) => [e[fields[index]], color, `${label}: ${num(e[fields[index]])}`]);
}

function modelTable(entries, source) {
    const view = modelViews[source];
    return metricsTable(sortModels(entries ?? [], view, source), source, {
        view, columns: modelColumns(source), units: MODEL_UNITS, costOf: unitCost,
        header: (column) => modelHeader(column, source),
        pick: source === "vscode" ? (key) => addFilter("model", exactPattern(key)) : null,
        pickTitle: "Filter by this model",
        empty: source === "session" ? "No recorded model usage for this query." : "No entries.",
    });
}

function metricsTable(entries, source, { view, columns, units, costOf, header, pick, pickTitle, displayKey, empty = "No entries." }) {
    if (!entries.length) return h("div", { class: "muted small" }, empty);
    const max = Math.max(...entries.map((e) => (e.InputTokens ?? 0) + (e.OutputTokens ?? 0)), 1);
    const maxUnit = Math.max(...entries.map((e) => costOf(e, view.unit) ?? 0), Number.MIN_VALUE);
    const unit = units[view.unit];
    const recordedTokens = (entry, id, value) =>
        Number.isSafeInteger(value) && CLI_TOKEN_FIELDS[id].every((field) =>
            Number.isSafeInteger(entry._rawUsage?.[field]) && entry._rawUsage[field] >= 0);
    const cell = (column, entry, cost) => {
        const value = column.value(entry, view.unit);
        if (value === null) return h("td", null, "–");
        switch (column.id) {
            case "model":
            case "workspace":
                return h("td", { class: "key" }, keyCell(entry.Key, pick, pickTitle, displayKey ? displayKey(entry.Key) : entry.Key));
            case "tokens": {
                const segments = usageTokenSegments(entry, source);
                const descriptions = segments.map((segment) => segment[2]);
                return h("td", source === "session" && recordedTokens(entry, column.id, value)
                    ? { title: descriptions.join(" · "), "aria-label": descriptions.join("; ") } : null,
                stackedBar(segments, max));
            }
            case "input":
            case "cached":
            case "cacheWrite":
            case "uncached":
            case "output": {
                const text = big(value);
                const title = source === "session" && recordedTokens(entry, column.id, value) ? `Recorded: ${num(value)} tokens` : null;
                return h("td", { title, "aria-label": title ? `${text}; ${title}` : null }, text);
            }
            case "cacheHit":
                return h("td", null, pct(entry.CachedTokens, entry.InputTokens));
            case "ttft":
            case "apiDuration":
                return h("td", null, source === "session" ? fusionDuration(value) : duration(value));
            case "aiu":
                return h("td", null, source === "session" ? fusionNumber(value) : aiu(value));
            case "premium":
                return h("td", null, source === "session" ? fusionNumber(value) : decimal(value));
            case "unit":
                return h("td", null, source === "session" ? fusionNumber(cost) : unitCostText(cost));
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
                    h("td", null, unitCostBar(cost, unit.header, maxUnit, source === "session" ? fusionNumber : unitCostText)),
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
    const entries = rawEntries.map((e) => {
        const entry = workspaceEntry(e, source);
        return source === "session" ? { ...entry, _rawUsage: e } : entry;
    });
    const usage = source === "session" || hasWorkspaceUsage(entries);
    const share = usage ? shareBar(entries, (e) => e.UsageAIU ?? 0, (e) => shortPath(e.Key),
        source === "session" ? { formatValue: fusionNumber, formatShare: usageSharePercent } : {}) : null;
    const notes = [];
    if (!usage && entries.length) notes.push("Workspace usage is not reported by this CLI build; update gh copilot-attendant to compare AIU.");
    if (usage && entries.some((e) => e.Requests === undefined)) notes.push("Request counts are not reported for some workspaces; their AIU / request is unavailable.");
    if (usage && entries.some((e) => e.Sessions === undefined)) notes.push("Session counts are not reported for some workspaces; their AIU / session is unavailable.");
    return [
        share ? panel("Usage share by workspace", topNote ? `AIU · ${topNote}` : "AIU", share, { wide: true, filter: "workspace" }) : null,
        panel("Workspaces", topNote ? `${topNote} · click to scope` : "click to scope", [
            ...notes.map((note) => h("p", { class: "muted small" }, note)),
            usage ? usageControls(source, WORKSPACE_UNITS, workspaceViews[source], (patch) => setWorkspaceView(source, patch)) : null,
            workspaceTable(entries, source, usage),
        ], { wide: usage, filter: "workspace" }),
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

function usageSharePercent(value, total) {
    return value / total < 0.001 ? "<0.1%" : pct(value, total);
}

/** Renders a proportional share bar and legend for entries with a positive value. */
function shareBar(entries, value, label = (e) => e.Key, { formatValue = aiu, formatShare = pct } = {}) {
    const withValue = (entries ?? []).filter((e) => value(e) > 0);
    const total = withValue.reduce((n, e) => n + value(e), 0);
    if (!total) return null;
    const color = (i) => SHARE_COLORS[i % SHARE_COLORS.length];
    const segments = withValue.map((e, i) => [value(e), color(i), `${label(e)}: ${formatValue(value(e), e)} AIU (${formatShare(value(e), total)})`]);
    const bar = stackedBar(segments, total);
    bar.classList.add("share");
    return [bar, legend(withValue.map((e, i) => [`${label(e)} ${formatShare(value(e), total)}`, color(i)]))];
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
    const agentScoped = state.results.vscode.argv?.some((a) => a.startsWith("--agent=")) ?? state.query.filters.vscode.agent.length > 0;

    return [
        categorySection("vscode-usage", "Usage", null, [
            agentScoped
                ? h("p", { class: "muted small" }, "Only the selected subagents' own usage is included. Main-agent and differently named descendant usage is excluded. Sessions count parent chats.")
                : null,
            h(
                "div",
                { class: "cards usage-cards" },
                card("Sessions", num(data.Sessions)),
                card("Usage (AIU)", aiu(sum("UsageAIU")), modelNote ? modelNote.slice(3) : null),
                card("LLM requests", num(data.LLMRequests), data.Turns ? `${(data.LLMRequests / data.Turns).toFixed(1)} / turn` : null),
                card("Input tokens", big(input), `${pct(cached, input)} cached${modelNote}`),
                card("Output tokens", big(sum("OutputTokens")), modelNote ? modelNote.slice(3) : null),
                card("Turns", num(data.Turns), data.Sessions ? `${(data.Turns / data.Sessions).toFixed(1)} / session` : null),
                card("Tool calls", num(data.ToolCalls), toolTotal ? `${pct(toolErrors, toolTotal)} errors (listed tools)` : null),
            ),
            h(
                "div",
                { class: "grid" },
                ...modelPanels(models, "vscode"),
                ...workspacePanels(workspaces, "vscode", topNote),
                data.UserQuestions != null ? askUserPanel(data.UserQuestions, "vscode") : null,
            ),
        ]),
        categorySection("vscode-tools", "Tools", null, [h(
            "div",
            { class: "grid" },
            panel("Tools", topNote ? `${topNote} · click to filter` : "click to filter", toolTable(tools), { wide: true, filter: "tool" }),
            panel("Subagents", "click to scope usage", simpleRows(data.ByAgent, (e) => e.Total, { onPick: (k) => addFilter("agent", exactPattern(k)), pickTitle: "Show only this subagent's own usage" }), { filter: "agent" }),
        )]),
    ];
}

// ---------------------------------------------------------------- ask agent

function renderPresets(source, result) {
    const presets = meta.presets.filter((p) => p.sources.includes(source) &&
        !(source === "session" && state.query.fusionOnly && p.id === "permissions"));
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
            top: 20,
            ...(typeof state.query.fusionOnly === "boolean" ? { fusionOnly: false } : {}),
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
        const bounds = calendarPeriodBounds(form.period.value);
        if (bounds) {
            form.since.value = bounds.since;
            form.until.value = bounds.until;
        } else if (form.period.value) {
            form.since.value = "";
            form.until.value = "";
        }
    });
    form.since.addEventListener("input", () => {
        if (form.since.value.trim() || calendarPeriodBounds(form.period.value)) form.period.value = "";
    });
    form.until.addEventListener("input", () => {
        if (calendarPeriodBounds(form.period.value)) form.period.value = "";
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
