export function hookDataset() {
    return {
        Version: 1,
        State: "available",
        Data: {
            Invocations: 7, PermissionRequests: 2, PreToolUses: 5,
            Decisions: { Allow: 5, Deny: 2, None: 0 },
            Outputs: { NotAttempted: 0, WriteSucceeded: 6, WriteFailed: 1 },
            Processing: { Success: 6, InputError: 0, OutputError: 1 },
            Unidentified: { Missing: 1, Unmatched: 2, Ambiguous: 1, Unscopable: 0, Unavailable: 0 },
            Attribution: "available", MalformedRecords: 3, UnsupportedVersions: 1,
            ByTool: [
                { Key: "view", Total: 2 }, { Key: "bash", Total: 1 },
                { Key: "", Total: 1 }, { Key: "Other", Total: 1 }, { Key: "write", Total: 1 },
            ],
            ByDecisionTool: {
                Allow: [{ Key: "view", Total: 2 }, { Key: "Other", Total: 1 }, { Key: "", Total: 1 }],
                Deny: [{ Key: "write", Total: 1 }],
            },
            ByTarget: [
                { Tool: "view", Decision: "allow", Target: "/work/src/main.go", Truncated: false, Total: 1 },
                { Tool: "bash", Decision: "allow", Target: "git status <b>--short</b>", Truncated: true, Total: 1 },
                { Tool: "write", Decision: "deny", Target: "", Truncated: false, Total: 1 },
            ],
        },
    };
}
