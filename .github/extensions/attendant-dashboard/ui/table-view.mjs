export function normalizeTableView(saved, { units, columns, defaultUnit, defaultSort }) {
    const view = saved !== null && typeof saved === "object" ? saved : {};
    return {
        unit: Object.hasOwn(units, view.unit) ? view.unit : defaultUnit,
        sort: columns.some((column) => column.id === view.sort) ? view.sort : defaultSort,
        direction: view.direction === "asc" ? "asc" : "desc",
    };
}

export function toggleTableSort(view, columnId, columns) {
    const column = columns.find((entry) => entry.id === columnId);
    return {
        sort: column.id,
        direction: view.sort === column.id ? (view.direction === "asc" ? "desc" : "asc") : column.direction ?? "desc",
    };
}

export function sortTableEntries(entries, view, columns) {
    const column = columns.find((entry) => entry.id === view.sort);
    const direction = view.direction === "asc" ? 1 : -1;
    return [...entries].sort((a, b) => {
        const left = column.value(a, view.unit);
        const right = column.value(b, view.unit);
        // Unknown metrics stay last in either direction, rather than looking like zero.
        if (left === null && right !== null) return 1;
        if (right === null && left !== null) return -1;
        const comparison = left === null ? 0 : typeof left === "string" ? left.localeCompare(right) : left - right;
        return comparison * direction || String(a.Key).localeCompare(String(b.Key));
    });
}
