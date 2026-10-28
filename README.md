# cxj

One-shot Java/Spring code intelligence CLI. Indexes `--root` (default: cwd) in RAM and exits. No daemon, no embeddings, no on-disk index.

`ask` is optional. If Needle 3 files are missing, `cxj ask` prompts to download them (~35 MB) into `needle/` (or next to the binary). Other commands do not need Needle.

## Install

macOS arm64 release binary (from [Releases](https://github.com/umuterturk/cxj/releases)):

```sh
# unzip cxj-macos-arm64.zip, then:
./cxj install -g   # /usr/local/bin/cxj + global agent skill
```

## Build

```sh
go build -o bin/cxj ./cmd/cxj
./scripts/bundle.sh
```

```sh
./bin/cxj install      # project skill (.agents + .claude symlink)
./bin/cxj install -g   # global skill + /usr/local/bin/cxj
./bin/cxj brief --root /path/to/workspace
./bin/cxj ask 'who calls OwnerController' --root /path/to/workspace
```

## Commands

```
cxj brief | find | show | outline | map | trail
cxj callers | callees | references | keys
cxj endpoints | beans | config | context | status
cxj ask          # optional; prompts to download Needle 3 if missing
cxj COMMAND --example   # 2-3 workspace examples
cxj install | install -g | version | update
```

Navigational commands (`show`/`outline`/`trail`/`callers`/`callees`/`references`/`endpoints`/`beans`/`config`/`keys`) index **main sources only** by default — skipping `src/test` cuts CPU ~3-5x on large repos. Pass `--scope all` (or `--scope test`) to change this; on a miss the CLI hints you to retry with `--scope all`.
