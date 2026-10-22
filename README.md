# headbox

A headless browser in a box: one Go binary that installs a pinned
[chrome-headless-shell](https://developer.chrome.com/blog/chrome-headless-shell),
runs it inside [bubblewrap](https://github.com/containers/bubblewrap) and leaves nothing
behind, even after `kill -9`. Tests and coding agents talk to it over the
Chrome DevTools Protocol (CDP).

```sh
go install ubunatic.com/headbox@latest
headbox install                  # once: download + sha256-check the pinned browser (126 MB zip)
headbox exec -- node check.mjs   # browser up, CDP_URL set, browser gone when the command ends
```

## Why

Headless browsers left behind by tests and agents pile up: profiles in RAM-backed `/tmp`,
orphaned browser trees after a killed agent, caches that grow to gigabytes. headbox runs the
browser in its own PID namespace with every writable path in a size-capped tmpfs, as a direct
child that dies with headbox. When headbox ends, for whatever reason, the browser and all its
files are gone.

## Commands

| Command | What it does |
| --- | --- |
| `headbox install [--zip FILE]` | Download the pinned chrome-headless-shell, check its sha256, unpack it to `~/.local/share/headbox/` |
| `headbox exec [flags] -- CMD` | Start the browser on a free port, run CMD with `CDP_URL`, `HEADBOX_WS_URL`, `HEADBOX_CDP_PORT` set, stop the browser when CMD exits; exits with CMD's code |
| `headbox serve [--idle 15m]` | Keep a browser for agents; prints the endpoint as shell variables, stops after `--idle` without a CDP connection (0: never), on Ctrl-C or SIGTERM |
| `headbox shot URL -o FILE.png` | Start the browser, load URL (http(s), `file://` or a local path) after its load event, write a PNG on the host, stop the browser; `--width`/`--height` (default 1280x800), `--dark` emulates `prefers-color-scheme: dark`, `--timeout` (default 30s) |
| `headbox path` | Print the browser's install dir |
| `headbox version` | Print the headbox and browser versions |

Flags for `exec`, `serve` and `shot`: `-p, --port` (default 0, a free port), `-v, --verbose`
(Chrome's stderr), `--chrome-arg FLAG` (repeatable).

For a local page, `shot` binds the file's directory read-only into the sandbox, so relative
CSS and images load. Dark mode is the emulated media feature (CDP
`Emulation.setEmulatedMedia`), as if the user preferred it; the page is not modified.

## Examples

```sh
# Any CDP client: puppeteer, playwright connectOverCDP, chromedp, a hand-written script
headbox exec -- sh -c 'curl -s "$CDP_URL/json/version"'

# A browser for coding agents, on a fixed port, gone after 30 idle minutes
headbox serve --port 9333 --idle 30m &
chrome-devtools-mcp --browserUrl http://127.0.0.1:9333

# Screenshots of a local checkout, light and dark
headbox shot public/index.html -o light.png
headbox shot public/index.html -o dark.png --dark --width 390 --height 844
```

## The sandbox

| Path | Inside the sandbox |
| --- | --- |
| `/tmp`, `/dev/shm` | tmpfs, 256 MiB each |
| `$HOME` (`/home/sandbox`) | tmpfs, 512 MiB: profile and caches |
| `/usr`, `/lib64`, `/etc` | read-only from the host |
| `/run/systemd/resolve` | read-only if present, for DNS where `/etc/resolv.conf` links there |
| `/opt/browser` | the browser dir, read-only |
| `~/.cache/fontconfig` | the host's font cache, read-only (saves about 300 ms per start) |
| the page's directory (`shot` with a local file) | read-only, at its host path |

bwrap flags: `--unshare-all --share-net --die-with-parent --new-session --clearenv`. Only the
network is shared, so pages load as usual and the CDP port is reachable on `127.0.0.1`.
The environment is cleared except for `HTTP(S)_PROXY`, `NO_PROXY` and `ALL_PROXY` (either
case), which pass through when set.
Chrome's own sandbox stays on; it works inside bwrap. Sizes, version, URL and sha256 live in
[`spec/browser.yaml`](spec/browser.yaml), embedded in the binary.

Measured on Fedora 44 (x86_64): `headbox exec -- true` takes about 0.2 s; the browser tree
uses about 500 MB RAM with one page open.

## Requirements

Linux with unprivileged user namespaces and `bwrap` (`dnf install bubblewrap`,
`apt install bubblewrap`). Go 1.26 or newer to build. macOS is not supported yet.

Release binaries for x86_64 and aarch64 are on
[Codeberg](https://codeberg.org/ubunatic/headbox/releases). `SHA256SUMS` is signed with
minisign:

```sh
minisign -Vm SHA256SUMS -P RWTRssohnfcNDh5mjZtyhk/Wks5P+WGwnQB+ermH1/TJq5fCqCyrVi0c
sha256sum -c --ignore-missing SHA256SUMS
```

## Background

headbox came out of a comparison of nine headless browser setups for tests and agents
(chrome-headless-shell in bwrap, rod, chromedp, podman, agent-browser, Obscura, Firefox,
Lightpanda, chrome-devtools-mcp); none of the ready-made tools cleaned up after `kill -9` on
its own. See the
[engineering story](docs/studies/2026-10-22-headbox-story.md).

Licence: AGPL-3.0-or-later.
