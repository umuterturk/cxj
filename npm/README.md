# @umuterturk/cxj

npm wrapper for [cxj](https://github.com/umuterturk/cxj) — stateless
Java/Spring code intelligence for coding agents. `postinstall` downloads the
official GitHub Release binary for your platform (macOS/Linux, arm64/x86_64);
there is no build step and no npm dependency tree.

```sh
npm install -g @umuterturk/cxj
cxj brief                # in a Java/Spring repo
cxj install -g           # write the agent skill to ~/.agents/skills/cxj
```

Or without installing, one-off:

```sh
npx @umuterturk/cxj brief
```

## How it maps to releases

| npm install runs on | downloaded asset            |
| ------------------- | --------------------------- |
| darwin / arm64      | `cxj-macos-arm64.zip`       |
| darwin / x64        | `cxj-macos-x86_64.zip`      |
| linux / x64         | `cxj-linux-x86_64.zip`      |
| linux / arm64       | `cxj-linux-arm64.zip`       |

The wrapper version tracks the cxj release (`0.3.2` installs
`v0.3.2`); set `CXJ_VERSION` to pin a different one. `cxj update` still works
after install and upgrades the same binary in place.

Requires `unzip` on PATH (present by default on macOS and most Linux distros).
Windows is not supported — the underlying tool targets macOS and Linux.
