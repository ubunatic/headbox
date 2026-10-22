**Once per session, before your first task, read all Harnez rules in one call: `harnez read .harnez/rules/Tools.md .harnez/rules/Issues.md .harnez/rules/Quota.md .harnez/rules/Subagents.md .harnez/rules/Output.md .harnez/rules/Local.md`. Skip this if you already read them in this session. This follows Index.md order; Local.md overrides the other rules.**

Adhere to the following conventions.

<!-- harnez:begin Project Summary -->
## Project Summary

`headbox` is a Go CLI that installs a pinned chrome-headless-shell, runs it inside bwrap
(tmpfs profile, read-only binds, no leftovers after any kill) and hands its CDP URL to
tests and agents. Design, scope and milestones: `issues/001-m1-parity-with-the-ubunatic-scripts.md`
and the origin research in ubunatic.com (issues 095, 096).
<!-- harnez:end Project Summary -->

## Development Scripts

Run from project root. `make help` lists the targets: `make check` (vet, tests, `reuse lint`)
before every commit, `make install` after changes, `make release` for a version bump in
`version.yaml`. The browser tests need `headbox install` once; `make test-short` skips them.
The module path is the vanity URL `ubunatic.com/headbox` (page in the ubunatic.com repo, generated
from `website/page.yaml`).

## Background Tasks & Process Hygiene

- Subagent handoff must not block the main chat. When the user asks to hand work to a subagent,
  spawn/delegate the task and remain responsive as the host orchestrator; do not immediately wait
  on the child agent unless the user explicitly asks you to wait or the next user-visible
  integration step truly cannot proceed without the result.
- Do not spawn subagents with git worktree isolation unless the user explicitly requests it.
  Sequential/consecutive ticket work should run directly on the currently checked-out branch —
  worktrees have their own failure modes (e.g. branching from a stale base, or being unable to
  see a prior step's still-uncommitted changes) and add reconciliation overhead that isn't needed
  for normal one-after-another dev work.
<!-- harnez:begin Language Conventions -->
Adhere to the following conventions.

Docs in `./docs/` are managed by harnez. <!-- harnez:bundled -->

- Agentic Loop Practices @docs/AgenticLoop.md,
  5-phase loop (Advisory -> Dev -> Review -> Hygiene -> Retro), zero zombie guarantee
- Bash/Shell @docs/Bash.md,
  Read before multi-line shell: Make recipes, embedded scripts
  No ";", break before then/else/docs
  No "if [[]]", No "if []", Use "if test"
  smart indent!
  Use git -C/make -C, not cd
- Canary-first development @docs/Canary.md,
  probe external mechanisms before building features on them
- Git @docs/Git.md,
  conventional commits, work on the default branch, don't push unless asked
- Go/Golang @docs/Go.md,
  Modern Go, avoid deps but use Cobra, add tests; use runes and display width for terminal layout
- Release Pipeline @docs/GoRelease.md,
  harnez release, version.yaml spec, GoReleaser v2, non-interactive minisign (-W), Forgejo has_releases, language-agnostic (Go/Python/Zig/Rust/scripted)
- Issue Tracking Practices @docs/IssueTracking.md,
  P0-P3 priorities, metadata headers (Status, Priority, Severity, Category), tracker sync
- Make/Makefile @docs/Make.md,
  ⚙️ phony sentinel, self-doc help, build dependency pattern
- Markdown @docs/Markdown.md,
  PascalCase for evergreens, kebab-case for ephemeral docs; ASCII art in chat, Mermaid only in docs/
- Search Practices @docs/Search.md,
  harnez find code/docs, finder configuration, partial results, and rg fallback
- Spec system @docs/Spec.md,
  YAML spec files as single source of truth; Go code must not duplicate spec values
- Test Tiers @docs/TestTiers.md,
  make lint / test-pkg / test-short / test / container-test; no sleeps, raceFactor budgets, PTY tests in the full tier
<!-- harnez:end Language Conventions -->
<!-- harnez:begin Repo Setup -->
## Repo Setup
- Solo/hobby repo — single default branch, no PR workflow.
- codeberg.org is primary; github.com (if present) is a synced mirror only.
<!-- harnez:end Repo Setup -->
