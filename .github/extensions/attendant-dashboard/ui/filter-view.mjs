/** Returns a minimal patch clearing one panel's condition, or null when inactive. */
export function panelClearPatch(query, source, filter) {
    if (filter === "workspace") {
        return query.scope === "all" ? null : { scope: "all", path: "" };
    }
    const values = query.filters[source]?.[filter];
    if (!Array.isArray(values)) throw new Error(`Unknown ${source} panel filter: ${filter}`);
    return values.length ? { filters: { [source]: { [filter]: [] } } } : null;
}
