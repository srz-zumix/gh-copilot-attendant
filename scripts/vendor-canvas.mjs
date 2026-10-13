import { copyFile, mkdir } from "node:fs/promises";

const root = new URL("../", import.meta.url);
const target = new URL(".github/extensions/attendant-dashboard/vendor/", root);
await mkdir(target, { recursive: true });
await copyFile(new URL("node_modules/re2js/build/index.js", root), new URL("re2js.mjs", target));
await copyFile(new URL("node_modules/re2js/LICENSE", root), new URL("LICENSE-re2js", target));
