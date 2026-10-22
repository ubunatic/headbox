# 002 — shot command for screenshots; uman website shot uses headbox

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Feature
**Related**: 001 (M1, closed), ubunatic.com 096 (plan, milestone M4), uman `website shot`

---

## 1. Goal

/goal `headbox shot URL -o FILE.png [--width N] [--height N] [--dark]` takes a screenshot in the
sandboxed browser over CDP (no extra runtime: Go only), and uman `website shot` calls it instead
of its own browser start; or stop and report when uman's owner decision or a missing CDP
capability blocks it.

## 2. Notes

- Dark mode must come from emulated `prefers-color-scheme`, not a forced page attribute (lesson
  from harnez.org 028 M2.4).
- The README's agent section could also show `agent-browser --cdp` next to
  `chrome-devtools-mcp --browserUrl` (former M3 doc item; `serve` itself shipped in v0.1.0).
- Lightpanda as an optional `read` engine stays undecided (ubunatic.com 096 §6).

## 3. Progress

- **M1 delivered (a8302d7): `headbox shot`.** Stdlib-only CDP client in `internal/cdp`, dark via
  `Emulation.setEmulatedMedia`, read-only bind of a local page's directory. Host review: `make
  check` green; shots of ubunatic.com over a local HTTP server are correct in light (1200 px) and
  dark (390 px); no browser left behind. `file://` pages with root-absolute links (`/style.css`)
  render unstyled; that is expected, sites need an HTTP server.

## 4. M2: uman `website shot` calls headbox

Pre-work (do first, commit separately):

1. **DNS in the sandbox (bug since v0.1.0, affects exec and serve too).** `headbox shot
   https://ubunatic.com/headbox/` fails with `net::ERR_NAME_NOT_RESOLVED`. On Fedora
   `/etc/resolv.conf` links into `/run/systemd/resolve/`, which the sandbox lacks. Bind it
   read-only when present (`--ro-bind-try`), and add a browser test that resolves a name (skip
   when the host itself has no network).
2. **Proxy env.** The sandbox clears `HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY` (and lower-case);
   pass them through when set.

Then M2: `/home/uwe/projects/uman` `website shot` (`website_shot.go`) uses `headbox shot` when
`headbox` is on PATH, keeping its current browser detection as fallback; light/dark via
`--dark`, not a forced page attribute. Keep uman's tests green, run `make install` in both repos.
