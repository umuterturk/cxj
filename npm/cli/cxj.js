#!/usr/bin/env node
// Shim: run the platform binary fetched by scripts/install.js.
// All arguments and stdio pass straight through — cxj is a Go binary.
const { spawnSync } = require("node:child_process");
const path = require("node:path");
const fs = require("node:fs");

const binary = path.join(__dirname, "..", "native", "cxj");
if (!fs.existsSync(binary)) {
  console.error(
    "cxj: platform binary missing — postinstall did not complete.\n" +
      "Re-run: npm rebuild @umuterturk/cxj"
  );
  process.exit(1);
}
const r = spawnSync(binary, process.argv.slice(2), { stdio: "inherit" });
if (r.error) {
  console.error("cxj: failed to run " + binary + ": " + r.error.message);
  process.exit(1);
}
process.exit(r.status === null ? 1 : r.status);
