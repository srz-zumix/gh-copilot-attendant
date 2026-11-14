/** Returns a minimal patch clearing one panel's condition, or null when inactive. */
export function panelClearPatch(query, source, filter) {
    if (filter === "workspace") {
        return query.scope === "all" ? null : { scope: "all", path: "" };
    }
    const values = query.filters[source]?.[filter];
    if (!Array.isArray(values)) throw new Error(`Unknown ${source} panel filter: ${filter}`);
    return values.length ? { filters: { [source]: { [filter]: [] } } } : null;
}

/** Returns an explanation when activity data does not match the selected filters. */
export function activityFilterError(activity, filters) {
    for (const [field, filter, label] of [
        ["AgentModes", "agentMode", "Agent mode"],
        ["SandboxStates", "sandbox", "Sandbox"],
    ]) {
        const selected = [...new Set(filters?.[filter] ?? [])].sort();
        const recorded = activity[field];
        if (selected.length && !Array.isArray(recorded)) {
            return `This result does not confirm that Session activity used the selected ${label} filter. Refresh with an up-to-date CLI or choose the source runner.`;
        }
        if (Array.isArray(recorded)) {
            const applied = [...new Set(recorded)].sort();
            if (JSON.stringify(applied) !== JSON.stringify(selected)) {
                const appliedLabel = applied.length ? applied.map((value) => JSON.stringify(value)).join(", ") : `no ${label} filter`;
                const selectedLabel = selected.length ? selected.map((value) => JSON.stringify(value)).join(", ") : `no ${label} filter`;
                return `This Session activity result was collected with ${appliedLabel}; the current ${label} selection is ${selectedLabel}. Refresh to show matching data.`;
            }
        }
    }
    return null;
}
