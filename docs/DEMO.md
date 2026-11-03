# Recording the demo

One repo, three commands, under twenty seconds. The take is [spring-petclinic] — the
Spring reference app every Java developer recognizes, on Spring Boot 4.1.0 today. Nothing
is staged: the viewer can clone the same repo and get the same answers.

[spring-petclinic]: https://github.com/spring-projects/spring-petclinic

## The take

```sh
git clone --depth 1 https://github.com/spring-projects/spring-petclinic
./scripts/demo.sh --repo ./spring-petclinic
```

`scripts/demo.sh` prints each command the way a person types it, runs it, and pauses long
enough to read the output on video. Use `--fast` for a dry run. Measured on an M3 Pro:
`brief` 36 ms, `trail` 16 ms, `callers` 17 ms — the GIF has no dead time.

**Why these three beats**

1. `brief` — the orientation. Kind, versions, size, profiles, **every HTTP route**, and
   the persisted entities, in one screen. This is the "it already knows the map" moment.
2. `trail …OwnerController.processCreationForm` — one hop from a handler: its route
   (`POST /owners/new`), what it calls, and **the test that covers it**.
3. `callers …OwnerRepository.findByLastName --scope all` — declared-type resolution; the
   answer resolves to the derived method and lists every use, tests included. This is the
   beat `grep` gets wrong.

## What you should see (verified against a clean clone)

Each beat is cleared before it runs, so every answer is visible whole on a 120×36
terminal.

`brief` is 26 lines: a three-line header, five `pkg` lines, seventeen `http` lines, one
`out_db` line.

`trail … --scope all` is eight lines: `method`, `http POST /owners/new`, five `callees`
(`result.hasErrors`, `redirectAttributes.addFlashAttribute` ×2, `this.owners.save`,
`owner.getId`), and one `tests` row.

`callers … --scope all` is fourteen lines: `resolved findByLastNameStartingWith`, the
controller use, and twelve uses across `OwnerControllerTests` and `ClinicServiceTests`.

If a line differs, **record what actually appears** and open an issue — never edit the
frames to match this page.

## Recording it

Terminal: 120×36 or larger, dark theme, font 18–20, bare `$ ` prompt, no window chrome if
you can hide it. Clear the screen before the take (`demo.sh` does this).

**asciinema + agg** (crisp, small, re-recordable — recommended)

```sh
brew install asciinema agg
cd /path/to/cxj
./scripts/demo.sh --repo ./spring-petclinic --fast      # rehearse
asciinema rec --idle-time-limit 2 \
  --command "./scripts/demo.sh --repo ./spring-petclinic" demo.cast
agg --font-size 20 --speed 1 demo.cast docs/assets/demo.gif
```

**QuickTime + gifski** (no extra CLI config)

```sh
brew install gifski
# record the terminal window with QuickTime, then:
gifski --fps 12 -o docs/assets/demo.gif recording.mov
```

Aim for **under 5 MB** and a seamless loop (end on the last answer, no cursor park).

## Where it goes

- Top of [README](../README.md), under the tagline — the first thing a visitor sees
  working.
- The same GIF works as the X/LinkedIn clip with one line of copy (see the launch plan).

Keep `docs/assets/demo.gif` in the repo; GitHub renders it in the README and in social
cards.
