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
