function model(Key, Requests, UsageAIU, InputTokens, OutputTokens, CachedTokens, TotalTTFTMs, TTFTSamples) {
    return { Key, Requests, UsageAIU, InputTokens, OutputTokens, CachedTokens, TotalTTFTMs, TTFTSamples };
}

function tool(Key, Total, TotalMs, Error = 0) {
    return { Key, Total, TotalMs, Error, OK: Total - Error };
}

function log(ID, Main, Agents, ByModel, ByTool) {
    return {
        ID, Main, Agents, ByModel, ByTool, Turns: 1,
        LLMRequests: ByModel.reduce((n, row) => n + row.Requests, 0),
        ToolCalls: ByTool.reduce((n, row) => n + row.Total, 0),
    };
}

export function vscodeDataset() {
    return {
        Since: "2026-01-01T00:00:00Z", Until: "2026-02-01T00:00:00Z", Scope: {},
        LogDetails: {
            Version: 1,
            Sessions: [
                {
                    ID: "parent-a", Workspace: "/repo/a",
                    Logs: [
                        log("main", true, [], [model("main", 1, 1, 10, 1, 0, 100, 1)], [tool("write", 1, 50)]),
                        log("shared", false, ["Explore", "Other"], [model("a", 2, 2, 100, 5, 50, 10, 1)], [tool("read", 2, 20, 1)]),
                        log("more", false, ["Explore"], [model("a", 1, 1, 30, 2, 10, 30, 1)], [tool("read", 1, 10)]),
                        log("exec", false, ["executionSubagent"], [model("b", 1, 1, 40, 10, 0, 20, 1)], [tool("write", 1, 30)]),
                    ],
                    Invocations: [
                        { ID: "shared", Agent: "Other" }, { ID: "shared", Agent: "Explore" },
                        { ID: "more", Agent: "Explore" }, { ID: "exec", Agent: "executionSubagent" },
                        { ID: "", Agent: "Explore" }, { ID: "", Agent: "Explore" },
                    ],
                },
                {
                    ID: "parent-b", Workspace: "/repo/b",
                    Logs: [
                        log("main", true, [], [model("main", 1, 1, 10, 1, 0, 100, 1)], [tool("write", 1, 50)]),
                        log("child", false, ["Explore"], [model("a", 1, 0.5, 20, 3, 5, 50, 1)], [tool("read", 1, 20)]),
                    ],
                    Invocations: [{ ID: "child", Agent: "Explore" }],
                },
            ],
        },
    };
}
