#!/usr/bin/env node
// postinstall: download the cxj release zip for this platform, unpack it
// next to the shim. No state, no cache — same law the binary lives by.
//
// Asset names are a contract with internal/update/update.go AssetName:
//   darwin/arm64  -> cxj-macos-arm64.zip    darwin/x64   -> cxj-macos-x86_64.zip
//   linux/x64     -> cxj-linux-x86_64.zip   linux/arm64  -> cxj-linux-arm64.zip
//
// Version: pinned to package.json version ( vX.Y.Z ), overridable with
// CXJ_VERSION. Unzips with the system `unzip`; no npm dependencies.
const { spawnSync } = require("node:child_process");
const https = require("node:https");
const fs = require("node:fs");
const path = require("node:path");

const pkg = require("../package.json");
const version = process.env.CXJ_VERSION || pkg.version;

const asset = {
  "darwin-arm64": "cxj-macos-arm64.zip",
  "darwin-x64": "cxj-macos-x86_64.zip",
  "linux-x64": "cxj-linux-x86_64.zip",
  "linux-arm64": "cxj-linux-arm64.zip",
}[process.platform + "-" + process.arch];

if (!asset) {
  console.error(
    `cxj: no release asset for ${process.platform}/${process.arch}.\n` +
      "Build from source: go install github.com/umuterturk/cxj/cmd/cxj@latest"
  );
  process.exit(1);
}

const url = `https://github.com/umuterturk/cxj/releases/download/v${version}/${asset}`;
const dest = path.join(__dirname, "..", "native");
const zip = path.join(dest, asset);
fs.mkdirSync(dest, { recursive: true });

function get(u, file, redirects, done) {
  if (redirects > 5) return done(new Error("too many redirects: " + u));
  https.get(u, { headers: { "User-Agent": "@umutert/cxj installer" } }, (res) => {
    if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
      res.resume();
      return get(res.headers.location, file, redirects + 1, done);
    }
    if (res.statusCode !== 200) {
      res.resume();
      return done(new Error(`HTTP ${res.statusCode} for ${u}`));
    }
    const out = fs.createWriteStream(file);
    res.pipe(out);
    out.on("finish", () => out.close(done));
    out.on("error", done);
  }).on("error", done);
}

get(url, zip, 0, (err) => {
  if (err) {
    console.error(
      "cxj: could not download " + url + "\n" + err.message + "\n" +
        "Manual install: unzip the asset for your platform from\n" +
        "https://github.com/umuterturk/cxj/releases/tag/v" + version
    );
    process.exit(1);
  }
  const u = spawnSync("unzip", ["-o", "-q", zip, "cxj", "-d", dest], {
    stdio: "inherit",
  });
  fs.rmSync(zip, { force: true });
  if (u.status !== 0) {
    console.error("cxj: unzip failed (is `unzip` installed?)");
    process.exit(1);
  }
  fs.chmodSync(path.join(dest, "cxj"), 0o755);
  console.log(`cxj v${version} installed for ${process.platform}/${process.arch}.`);
});
