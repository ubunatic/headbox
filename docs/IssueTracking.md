---
title: Issue Tracking Practices
weight: 45
---

<!-- harnez:bundled -->
# Issue Tracking Practices — Priority & Metadata Standards

This document defines the canonical standards for tracking bugs, features, and technical tasks across `harnez`-managed repositories. Every project maintains an in-repository issue tracker in `issues/` with consistent priorities, metadata headers, and lifecycle rules.

---

## 1. Issue Priority Schema

Priorities define **scheduling urgency** — the order and immediacy with which work must be addressed.

| Priority | Level | Description & Escalation Trigger | Target SLA / Lifecycle |
|---|---|---|---|
| **P0** | **Critical** | Blocker, data loss, security vulnerability, broken build, or critical regression violating core invariants. Halts regular development ("stop the line"). | Immediate resolution; fix precedes all other tasks. |
| **P1** | **High** | Core functionality broken, major workflow impediment, key API regression, or high-urgency milestone deliverable. No acceptable workaround exists. | Current sprint / next immediate release cycle. |
| **P2** | **Medium** | Normal feature, standard bug fix, performance optimization, UX polish, or refactoring without active blockage. Workaround may exist. | Standard backlog; prioritized during routine planning. |
| **P3** | **Low** | Minor cosmetic glitch, typo, nice-to-have suggestion, speculative idea, or non-urgent documentation improvement. | Opportunistic; addressed when touching related code. |

---

## 2. Priority vs. Severity

Do not conflate **Priority** with **Severity**:

- **Severity** measures **technical impact and damage** (how severely the system is broken).
- **Priority** measures **scheduling urgency** (how quickly the team/agent must fix it).

*Examples:*
- A spelling error in the primary brand header on the homepage is **Low Severity** (cosmetic) but **P1 / High Priority** (reputational urgency).
- A catastrophic memory leak in an obscure, deprecated, unadvertised CLI flag that nobody uses is **Critical Severity** (crash/resource exhaustion) but **P3 / Low Priority** (low operational risk).

---

## 3. Standard Ticket Metadata Schema

Local issue files are placed under `issues/NNN-kebab-case-title.md` (e.g. `issues/042-standardized-issue-priority-schema.md`).

### Allocating & Reserving Ticket Numbers
Computing the next issue number and reserving/creating it are two separate commands, split across
`find` (read-only query) and `issues` (write-side): never use ad hoc shell commands (`ls | grep |
sort | tail`) for either.
- `harnez find issues next` — reports the next free ticket number (e.g., `195`) calculated as `max(allocated) + 1` across `issues/*.md` and `issues/archive/*.md`. Read-only: it never creates or reserves anything.
- `harnez find issues next --json` — outputs machine-readable JSON (`{"number":"195","reserved":false}`).
- `harnez issues new "Ticket Title"` (or `harnez issues new` with no title) — atomically allocates the next number and creates a placeholder ticket file (`issues/NNN-<title-slug>.md` or `issues/NNN-reserved.md` with status `Draft`) using `O_CREATE|O_EXCL` to prevent number collisions between concurrent agents. Prints `NNN<TAB>issues/<reserved-filename>.md` — write the real ticket content directly to that printed path rather than re-deriving the slug from the title by hand; a hand-derived slug can diverge from the reserved filename and leave an orphaned placeholder behind (see issue 202). Add `--json` for the same JSON shape as above with `"reserved":true` plus `file`/`path`. `new` never commits.

**Remote/cloud allocation:** `harnez issues new --remote "Ticket Title"` creates a GitHub
issue on the repository's `github` remote using authenticated `gh issue create`, then links its
body to the exact ticket path. Its number allocates a permanent `GH<n>` ID, stored as
`issues/GH<n>-<title-slug>.md` with header `# GH<n> — Ticket Title`. No host reservation or
local counter is needed. Missing/unauthenticated `gh` is an error, with no local fallback.
If updating the GitHub body or writing the ticket fails after allocation, the error identifies
the existing GitHub issue; it does not reuse that ID or allocate a local number.

Setting `CLAUDE_CODE_REMOTE` (even to an empty value) automatically selects remote mode;
`new` prints the selected mode to stderr, keeping text/JSON stdout machine-readable.
GH IDs never renumber on merge and cannot be moved with `issues mv`. Local allocation and
`find issues next` ignore GH IDs. All status verbs, `show`, `find`, `index`, lint and duplicate
checks accept both namespaces; arguments `GH42` and `gh42` select the same remote ticket,
while `42` selects local `042`. README sorts local numbers first, then GH IDs numerically.

Atomic local reservation only guards writers sharing one working tree. Cloud agents must
use remote allocation rather than choosing local numbers from a divergent clone.

Duplicate GH IDs are errors: reconcile the duplicate files while preserving the permanent ID.
The renumbering repair below applies only to local numeric IDs.

After a merge, `harnez index` and `harnez status` reject duplicate numbers and name every
colliding file; index leaves README unchanged. Run
`harnez issues mv -d . issues/NNN-extra-ticket.md` for each extra file. The path disambiguates
the ticket; omitting the destination picks the next free number. The move rewrites the title,
exact relative filename/basename references in issues/ and docs/, and the index in one commit.
Bare numbers remain unchanged because they cannot identify which duplicate was intended.
If the ticket lacks its numbered title, add `# NNN — Title` before retrying. After all moves,
run `harnez index --check` and check for any remaining merge conflict markers.

README synchronization uses a persistent `issues/README.md.lock` for `flock`; deleting it
can split concurrent writers across different inodes. `harnez init` ignores it (and the
reservation lock) in `.git/info/exclude`.

### Updating Ticket Status (`harnez issues <verb>`)
Use `harnez issues <verb>` to change a ticket's status, resync `issues/README.md`, and commit in one call:
- `harnez issues open <n> [reason]` — Status: Open (or `Open — <reason>`)
- `harnez issues start <n> [reason]` — Status: In Progress (or `In Progress — <reason>`)
- `harnez issues block <n> <reason>` — Status: `Blocked — <reason>` (reason required)
- `harnez issues close <n> [reason]` — Status: Closed (or `Closed — <resolution>`)
- `harnez issues done <n> [reason]` — Alias for `close`: Status: Closed (or `Closed — <resolution>`)
- `harnez issues draft <n> [reason]` — Status: Draft (or `Draft — <reason>`)
- `harnez issues mv <old> [new]` — Renumber a ticket, rename file, rewrite title and exact filename references in issues/ and docs/, resync index, and commit.
- `harnez issues list [filter]` — List matching tickets (e.g. `is:open`)

Closing an already-Closed ticket (`close` or `done`) exits 0 without changing
the ticket, index, or commits, even when a different reason is supplied. The
notice preserves the existing reason. To correct it explicitly, use
`harnez issues close <n> "<corrected resolution>" --amend-reason`; this updates
the reason in one commit and errors without writing if the ticket is not Closed.

The top of each ticket MUST contain the standardized metadata block:

```markdown
# NNN — Title of the Issue

**Status**: Open | In Progress | Blocked — <reason> | Closed — <resolution> | Draft
**Priority**: P0 (Critical) | P1 (High) | P2 (Medium) | P3 (Low)
**Severity**: Critical | Major | Moderate | Minor
**Category**: Bug | Feature | Architecture | Documentation | Performance | Refactor | Agentic Ergonomics
**Related**: [Doc / Ticket / Commit references]

---

## 1. Problem & Motivation
...

## 2. Technical Specification / Findings
...

## 3. Implementation & Verification Plan
...
```

### Allowed Values

- **Status**:
  - `Open`: Unresolved, ready to be worked on. May carry an optional `— <note>` suffix.
  - `In Progress`: Actively being worked on in current session. May carry an optional `— <note>`
    suffix (e.g. `In Progress — implementation complete; tracker closure awaits ...`, as issue 201
    does in practice).
  - `Blocked — <reason>`: Waiting on upstream dependency or external resolution. Reason required.
  - `Closed — <resolution>`: Completed and verified with tests (e.g. `Closed — resolved`, `Closed — invalid`).

    **Why no commit hash**: a commit's hash is content-addressed and cannot be known by the
    commit that writes it, so a ticket cannot self-reference its own closing commit without a
    follow-up fixup commit. Record the resolution in words and use
    `git log --oneline -- issues/NNN-*.md` for traceability. A hash is optional only when it
    deliberately points to an earlier commit.
  - `Draft`: Tentative proposal or placeholder. May carry an optional `— <note>` suffix.
- **Priority**: `P0 (Critical)`, `P1 (High)`, `P2 (Medium)`, `P3 (Low)`
- **Severity**: `Critical`, `Major`, `Moderate`, `Minor`
- **Category**: `Bug`, `Feature`, `Architecture`, `Documentation`, `Performance`, `Refactor`, `Agentic Ergonomics`, `Infrastructure`

### 3.3 Lean Specifications, /goal Definition, and As-Needed Milestones

- **Explicit Goal Definition**: Every ticket must define an explicit `/goal` (or clear Goal statement and acceptance criteria) specifying what the agent should work toward and achieve (the desired end-state and definition of done).
- **Keep Issues Short & Brief by Default**: When the goal is clear, keep tickets lean without over-specifying low-level implementation details or hypothetical steps.
- **Milestones Only When Truly Needed**: Only decompose Section 3 into discrete, numbered milestones (`### Milestone 1: ...`, `### Milestone 2: ...`) when the task genuinely requires staged execution, distinct review gates, or multi-phase handoffs (e.g., complex multi-subsystem migrations). When used, each milestone defines deliverables, automated verification targets, and acceptance criteria.
- **Issue Handling & Live Codebase Verification**: Because issues may sit in the backlog for days or weeks as the project evolves, any agent picking up a ticket must always check the live project/code status and recent commit history before starting work, re-verifying assumptions against current code rather than blindly following stale references. The ticket's `/goal` serves as the durable north star.

---

## 4. Issues Index & Archive Conventions

### 4.1 Index Table (`issues/README.md`)

`issues/README.md` contains active tickets only: `Open`, `In Progress`, `Blocked`, and `Draft`.
It links to generated `issues/BACKLOG.md` and `issues/CLOSED.md`. `harnez index` regenerates all
three files; `harnez issues lint` checks their completeness and generated content. Use
`harnez find -d <repo> issues -a status:active` for the same active set. The configured active
ticket cap lives in `spec/issues.yaml`; `harnez issues lint` and `harnez issues status` warn when
the active count exceeds it.

Deferred work goes to the **Backlog** lifecycle (`Status: Backlog — <reason>`) with
`harnez issues backlog <n…> [reason]`; `harnez issues promote <n…>` returns it to Open. `is:open`
excludes Backlog; `status:backlog` and `status:unresolved` reach it. Closed tickets cannot be
backlogged. Before filing, `harnez issues new` runs `harnez find issues similar "<title>"` over all
tickets (including Backlog, Closed and `issues/archive/`) and refuses on a strong match (score at or
above 0.65; threshold in `spec/issues.yaml`), printing `issues open|promote <n>` hints; reopen or
promote the match instead, or pass `--force` for a genuinely new ticket.

Generated files start with a `<!-- managed by harnez index: … -->` line and wrap generated tables
in `harnez:begin/end` markers; edit the source ticket or study, never the rows. `harnez init`
installs a `harnez-issues-index` git merge driver (`.gitattributes` + local git config) that merges
these files by rebuilding their generated blocks from both branches; conflicts in hand-written parts
of a partly generated file stay normal git conflicts.
For generated rows, a deletion on one branch wins when the other branch left the row unchanged from
the common ancestor. A rename is a deletion of the old key and an addition of the new key; competing
edits to the same row remain conflicts.

The active index `issues/README.md` and generated `BACKLOG.md` and `CLOSED.md` maintain a
synchronized inventory of all tickets. Run `harnez index` (issue 148) to regenerate all three
tables from `issues/*.md` + `issues/archive/*.md` metadata instead of hand-editing rows — it is idempotent (a
second run against unchanged tickets makes no further change) and has a `--check`
flag that exits 1 on drift without writing, for CI/pre-commit use — and prints a
unified diff of exactly what would change, so running it directly in an agent
session surfaces specific drift the agent can act on immediately, without a
separate diff step. For `issues/README.md`, only the table inside its
`harnez:begin/end` markers is managed (a file without markers is migrated once from the exact
`| # | File | Title | Status |` header): prose before or after that table is preserved verbatim. A customized
table header (for example, one with an added Priority or Target column) is
refused without writing the file, because harnez cannot regenerate values for
project-specific columns; reconcile that schema manually before adopting the
generated table. `BACKLOG.md` and `CLOSED.md` are generated as complete files.
`harnez index`
also regenerates the project's studies index when that convention exists; otherwise
the issues index is updated independently. Manual edits to
rows within either managed table will be overwritten by the next `harnez index`
run — prefer fixing the source ticket/study file instead.

```markdown
# Issues

See [Backlog](BACKLOG.md) and [Closed](CLOSED.md).

| # | File | Title | Status |
|---|------|-------|--------|
| 005 | [005-permissions-grow-only.md](005-permissions-grow-only.md) | Permissions are grow-only — revoked entries never removed | Open |
| 007 | [007-no-tests.md](007-no-tests.md) | Thin test coverage — gaps remaining | Open — partially addressed |
```

`harnez issues lint` verifies every ticket appears in the index for its lifecycle category,
all table links point to valid files (including `issues/archive/`), statuses match, and ticket
numbers are unique. The broader `harnez status` tracker summary also checks index consistency.

### 4.2 Archiving Closed Issues

When an issue is closed and verified, it may be moved to `issues/archive/NNN-kebab-case.md` to keep the active `issues/` directory clean and focused. `harnez index` keeps it in the generated `issues/CLOSED.md` index.

---

## 5. Lifecycle Invariants

1. **Test Verification Before Closure**: Never mark a ticket `Closed` without executing the test suite (`go test ./...`, `make check`) and confirming assertion rigor.
2. **Atomic Index Synchronization**: Whenever ticket status changes in the file, immediately update `issues/README.md`.
3. **Immediate Tracker Commit**: After creating or updating issue-tracker files, commit the ticket file and synchronized index immediately in their own small commit. Do not batch tracker metadata with unrelated code or defer it to a later feature-work checkpoint.
4. **Traceability**: Link relevant study notes, retrospectives, ADRs, and commits in the `**Related**:` header, using the project's existing durable documentation location.
5. **Closing Is Part Of Done**: Progress-noting a ticket to `In Progress` is disciplined for
   free — closing it is not, because nothing forces the last step. The `smarthome` project
   shipped 112 commits in three days with excellent open/progress hygiene, yet four tickets
   still read `In Progress` for work that was demonstrably shipped and live-verified. A session that ends on a
   green build and a commit is not done until every ticket it touched has its `Status` flipped
   and `harnez index` has been run. Treat "did I close what I finished?" as an explicit
   end-of-session check, not an assumption that closing happens naturally alongside the code.
6. **Goal-Centric & As-Needed Milestones**: Every ticket must define a `/goal` for the agent. Only decompose into numbered milestones (`M1`, `M2`...) when multi-step staged execution is truly needed; otherwise keep issues lean and brief. Agents picking up an issue must check live code status before beginning work.

<!-- harnez:stop -->
