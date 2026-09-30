// Loopback HTTP server backing one canvas instance.
//
// The host iframe has no privileged bridge, so the UI talks to the extension
// over plain HTTP on 127.0.0.1. Because any local process or web page can
// reach a loopback port, every API request must carry a per-instance token
// (delivered only inside the same-origin HTML page), a matching Host header,
// and, when present, a matching Origin header.

import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { randomBytes, timingSafeEqual } from "node:crypto";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { QueryError, FILTERS, SOURCES } from "./query.mjs";
import { RUNNERS } from "./runner.mjs";
import { PROMPT_PRESETS } from "./summary.mjs";

const UI_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), "..", "ui");
const TOKEN_HEADER = "x-dashboard-token";
const MAX_BODY = 64 * 1024;
const MAX_ASK = 8 * 1024;

const STATIC = {
    "/": ["index.html", "text/html; charset=utf-8"],
    "/index.html": ["index.html", "text/html; charset=utf-8"],
    "/app.js": ["app.js", "text/javascript; charset=utf-8"],
    "/styles.css": ["styles.css", "text/css; charset=utf-8"],
};

const CONTENT_SECURITY_POLICY = [
    "default-src 'none'",
    "script-src 'self'",
    "style-src 'self' 'unsafe-inline'",
    "img-src 'self' data:",
    "connect-src 'self'",
    "base-uri 'none'",
    "form-action 'none'",
].join("; ");

class HttpError extends Error {
    constructor(status, message) {
        super(message);
        this.status = status;
    }
}

function sendJson(res, status, body) {
    res.writeHead(status, { "Content-Type": "application/json; charset=utf-8", "Cache-Control": "no-store" });
    res.end(JSON.stringify(body));
}

async function readJson(req) {
    const type = String(req.headers["content-type"] ?? "");
    if (!type.startsWith("application/json")) throw new HttpError(415, "expected application/json");
    const chunks = [];
    let size = 0;
    for await (const chunk of req) {
        size += chunk.length;
        if (size > MAX_BODY) throw new HttpError(413, "request body too large");
        chunks.push(chunk);
    }
    const text = Buffer.concat(chunks).toString("utf-8").trim();
    if (!text) return {};
    try {
        const value = JSON.parse(text);
        if (value === null || typeof value !== "object" || Array.isArray(value)) throw new Error("not an object");
        return value;
    } catch {
        throw new HttpError(400, "invalid JSON body");
    }
}

function tokenMatches(expected, actual) {
    if (typeof actual !== "string") return false;
    const a = Buffer.from(expected);
    const b = Buffer.from(actual);
    return a.length === b.length && timingSafeEqual(a, b);
}

/**
 * Starts the per-instance server.
 *
 * @param {object} options
 * @param {import("./dashboard.mjs").Dashboard} options.dashboard
 * @param {(args: {source: string, text: string}) => Promise<void>} options.ask
 * @param {string} [options.host]
 * @returns {Promise<{url: string, token: string, close: () => Promise<void>}>}
 */
export async function startServer({ dashboard, ask, host = "127.0.0.1" }) {
    const token = randomBytes(24).toString("hex");
    const streams = new Set();
    let origin = "";

    const checkRequest = (req, { api }) => {
        if (req.headers.host !== origin.slice("http://".length)) throw new HttpError(421, "unexpected host");
        if (req.headers.origin && req.headers.origin !== origin) throw new HttpError(403, "cross-origin request");
        if (api && !tokenMatches(token, req.headers[TOKEN_HEADER])) throw new HttpError(401, "missing or invalid token");
    };

    const meta = () => ({
        sources: SOURCES,
        filters: FILTERS,
        runners: RUNNERS,
        presets: Object.entries(PROMPT_PRESETS).map(([id, p]) => ({ id, label: p.label, sources: p.sources })),
    });

    const handlers = {
        "GET /api/state": async () => ({ ...dashboard.snapshot(), meta: meta() }),
        "POST /api/query": async (body) => {
            if (body.runner !== undefined) {
                if (!RUNNERS.includes(body.runner)) throw new HttpError(400, `runner must be one of ${RUNNERS.join(", ")}`);
                dashboard.setRunnerMode(body.runner);
            }
            dashboard.applyQuery(body.query ?? {});
            // Respond as soon as the query is accepted; results arrive over /events.
            if (body.refresh !== false) dashboard.ensureFresh({ force: body.force === true }).catch(() => {});
            return dashboard.snapshot();
        },
        "POST /api/ask": async (body) => {
            const source = body.source ?? dashboard.query.source;
            if (!SOURCES.includes(source)) throw new HttpError(400, "unknown source");
            let text = typeof body.text === "string" ? body.text.trim() : "";
            if (body.preset) {
                const preset = PROMPT_PRESETS[body.preset];
                if (!preset) throw new HttpError(400, "unknown preset");
                text = text ? `${preset.text}\n\n${text}` : preset.text;
            }
            if (!text) throw new HttpError(400, "text or preset is required");
            if (text.length > MAX_ASK) throw new HttpError(413, "message too long");
            if (!dashboard.results[source].data) throw new HttpError(409, "no data loaded for this source yet");
            await ask({ source, text });
            return { sent: true };
        },
    };

    const openStream = (req, res) => {
        res.writeHead(200, {
            "Content-Type": "text/event-stream; charset=utf-8",
            "Cache-Control": "no-store",
            Connection: "keep-alive",
        });
        const write = (snapshot) => res.write(`event: state\ndata: ${JSON.stringify(snapshot)}\n\n`);
        write(dashboard.snapshot());
        const unsubscribe = dashboard.subscribe(write);
        const keepAlive = setInterval(() => res.write(": ping\n\n"), 25_000);
        const entry = { res, stop: () => (clearInterval(keepAlive), unsubscribe()) };
        streams.add(entry);
        req.on("close", () => {
            entry.stop();
            streams.delete(entry);
        });
    };

    const server = createServer(async (req, res) => {
        const url = new URL(req.url ?? "/", "http://placeholder");
        try {
            const file = STATIC[url.pathname];
            if (req.method === "GET" && file) {
                checkRequest(req, { api: false });
                let body = await readFile(path.join(UI_DIR, file[0]), "utf-8");
                if (file[0] === "index.html") body = body.replace("__DASHBOARD_TOKEN__", token);
                res.writeHead(200, {
                    "Content-Type": file[1],
                    "Cache-Control": "no-store",
                    "Content-Security-Policy": CONTENT_SECURITY_POLICY,
                    "X-Content-Type-Options": "nosniff",
                    "Referrer-Policy": "no-referrer",
                });
                res.end(body);
                return;
            }
            if (req.method === "GET" && url.pathname === "/favicon.ico") {
                res.writeHead(204);
                res.end();
                return;
            }
            if (req.method === "GET" && url.pathname === "/events") {
                checkRequest(req, { api: true });
                openStream(req, res);
                return;
            }
            const handler = handlers[`${req.method} ${url.pathname}`];
            if (!handler) throw new HttpError(404, "not found");
            checkRequest(req, { api: true });
            const body = req.method === "POST" ? await readJson(req) : {};
            sendJson(res, 200, await handler(body));
        } catch (error) {
            const status = error instanceof HttpError ? error.status : error instanceof QueryError ? 400 : 500;
            if (!res.headersSent) sendJson(res, status, { error: error.message });
            else res.end();
        }
    });

    await new Promise((resolve, reject) => {
        server.once("error", reject);
        server.listen(0, host, resolve);
    });
    const { port } = server.address();
    origin = `http://${host}:${port}`;

    return {
        url: `${origin}/`,
        token,
        close: () =>
            new Promise((resolve) => {
                for (const entry of streams) {
                    entry.stop();
                    entry.res.end();
                }
                streams.clear();
                server.close(() => resolve());
                server.closeAllConnections?.();
            }),
    };
}
