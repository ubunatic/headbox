# 001 — M1 parity with the ubunatic scripts

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Feature
**Related**: ubunatic.com 095 (research), 096 (plan, `docs/studies/2026-10-22-browser-launcher-plan.md`), ubunatic.com `scripts/browser/{launch,install,spec}.sh`, `spec/browser.yaml`, canary `scripts/browser/canaries/k10-supervise/`

---

## 1. Goal

/goal `headbox install`, `headbox exec -- CMD` and `headbox version` do what ubunatic.com's
`install.sh` + `launch.sh` do today, with the browser pin embedded here, tests for the spec, the
sha256 check and a `kill -9` of headbox (no process or file left), `make install` working, and
`go install ubunatic.com/headbox@latest` resolving; or stop and report when a step needs root,
an owner decision, or a large download without `/mnt/kdev` mounted.

## 2. Decisions (owner, 2026-10-22)

- Name and home: `codeberg.org/ubunatic/headbox` (primary), GitHub mirror, vanity
  `ubunatic.com/headbox`.
- The browser pin (version, URL, sha256, tmpfs sizes) moves here, embedded via `spec/`
  (`docs/Spec.md`); ubunatic.com pins only the headbox version.
- uman `website shot` switches to headbox in M4.
- Lightpanda: not decided; pro/con and a demo in ubunatic.com 096.

## 3. Scope of M1

- `spec/browser.yaml` + schema, copied from ubunatic.com (`156.0.8078.47`, own sha256; Chrome for
  Testing publishes none). Settings file optional (`$XDG_CONFIG_HOME/headbox/config.yaml`) only
  if a value really needs overriding.
- `install`: download into `$XDG_DATA_HOME/headbox/chrome-headless-shell-<version>/` via a temp
  dir, check sha256, unzip, rename into place; `--zip FILE` skips the download.
- `exec`: bwrap as the direct child with `Pdeathsig=SIGKILL`; flags exactly as `launch.sh`
  (`--unshare-all --share-net --die-with-parent --new-session --clearenv`, byte-sized tmpfs for
  `/tmp`, `/dev/shm`, `$HOME`, read-only `/usr`, `/lib64`, `/etc`, browser dir, `--ro-bind-try`
  of the host fontconfig cache). Free port via `--remote-debugging-port=0` + `DevToolsActivePort`
  (canary first), ready = `/json/version` answers; then run CMD with `CDP_URL` set and stop the
  browser when CMD exits, passing on CMD's exit code.
- Known pitfalls (095): `--size` takes bytes only; Chrome's own sandbox works inside bwrap (no
  `--no-sandbox`); without the fontconfig bind each start costs about 300 ms.

## 4. Later milestones (own tickets when M1 is done)

- M2: ubunatic.com `check.sh` uses `headbox exec`; remove the scripts there and update callers.
- M3: `serve` with idle timeout; doc on attaching agent tools (`chrome-devtools-mcp --browserUrl`,
  `agent-browser --cdp`).
- M4: `shot`; uman `website shot` calls headbox.
- macOS (process group + watchdog) is out of scope until asked.
