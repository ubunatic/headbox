# headbox: a headless browser that cleans up after itself

**Date**: 2026-10-22
**Scope**: Why headbox exists, what the research behind it measured, and how v0.1.0 is built.

## The problem

The ubunatic.com site checks ran a Flatpak Chromium. On 2026-10-22 its sandbox `/tmp`, which
lives in RAM under `/run/user/1000`, held 2.4 GB of old browser profiles and filled the runtime
dir; an unrelated tool then failed with "no space left on device". Killing the wrapper did not
stop the browser: twelve headless Chromiums started by coding agents were still running, some
for a day. Every agent that started a browser and was then killed left one behind.

## The research (ubunatic.com issue 095)

Three researchers surveyed the field, eleven focused studies followed, and ten small canaries
measured each candidate on the same machine: start time, RAM, disk, screenshots in light and
dark mode, and what is left after a `kill -9` of the launcher and of the browser.

| Candidate | Ready | RAM | After `kill -9` of the launcher |
| --- | --- | --- | --- |
| chrome-headless-shell in bwrap | 64–75 ms | ~515 MB | clean |
| rod + leakless | 137 ms | 457 MB | profile in `/tmp/rod/` leaks |
| chromedp | 161 ms | 445 MB | `/tmp/chromedp-runner*` leaks |
| podman image | 7.2 s cold | 374 MB | container runs on until `--timeout` |
| agent-browser | 685 ms | 455 MB | browser, profile and sockets leak |
| Obscura 0.2.5 | 17 ms | 60 MB | orphaned; dark mode wrong |
| Firefox 158 (BiDi) | 1.6 s | 1.0 GB | orphaned, profile stays |
| Lightpanda 1.0.1 | 26 ms | 28 MB | orphaned; text-only rendering |
| chrome-devtools-mcp | 870 ms | 607 MB | profile leaks |

The lesson: any cleanup that runs in userspace is skipped by `kill -9`. Only the kernel cleans
up reliably. bwrap gives the browser its own PID namespace (when bwrap dies, every process in
it dies) and keeps every writable path in a tmpfs (when the namespace goes, the memory is
freed). `--die-with-parent` and a `Pdeathsig` on bwrap tie it to its launcher.

Two findings changed the design along the way. Chrome's own sandbox works nested inside
bwrap, so the common `--no-sandbox` advice is wrong here. And without the host's fontconfig
cache, each start rebuilt it inside the empty tmpfs home, about 300 ms; a read-only bind of
`~/.cache/fontconfig` made the sandboxed start as fast as an unsandboxed one.

## From scripts to headbox

The site first switched to two shell scripts (`launch.sh`, `install.sh`) and a spec file.
headbox turns them into one Go binary usable from any repo:

- `install` downloads the zip named in the embedded `spec/browser.yaml`, checks it against a
  sha256 pinned there (Chrome for Testing publishes none), unpacks next to the target and
  renames into place, so an install is either complete or absent.
- `exec` starts bwrap as a direct child with `Pdeathsig: SIGKILL` and its own process group,
  lets Chrome pick a free port (`--remote-debugging-port=0`) and reads the port from Chrome's
  `DevTools listening on ws://…` line, then runs the given command with `CDP_URL` set. The
  command gets `Pdeathsig: SIGTERM`, so a killed headbox takes it along.
- `serve` keeps the browser for agents and counts established connections to the CDP port in
  `/proc/net/tcp` to stop it after an idle period.

The tests start real browsers: one kills headbox with `SIGKILL` mid-run and checks that no
process carrying the run's `HEADBOX_ID` (set inside the sandbox, readable from
`/proc/<pid>/environ`) survives; another holds a TCP connection open to prove `serve` waits
for it.

## What is next

Switch ubunatic.com's `make check-browser` to `headbox exec`, a `shot` command for
screenshots that `uman website shot` can use, and possibly Lightpanda as an optional engine
for reading pages, with telemetry always off.
