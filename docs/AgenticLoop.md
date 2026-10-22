---
title: Agentic Loop Practices
weight: 40
---

<!-- harnez:bundled -->
# Agentic Loop Practices — Multi-Agent Sprint Workflow

This document establishes the canonical practice for orchestrating multi-agent development loops. It defines the lifecycle, synchronization invariants, role archetypes, and quality gates required to conduct rapid, collision-free agentic sprints.

Capability names vary by harness. With Harnez, the core lifecycle is
`harnez agent start --name <name> --model <model> --role <role> "<prompt>"`,
`harnez agent resume --name <id> "<task>"`, explicit `harnez agent compact --name <id>`, then
`harnez agent list` and targeted `stop`/`delete` cleanup.

### Agent command execution

`harnez agent start` and `harnez agent resume` are synchronous, low-noise
commands. They wait for the agent turn and include the agent reply in their
output. Use them directly for sequential work. If parallel work is wanted,
invoke each command through the invoking agent's visible host-session
background-job facility. Keep those jobs visible and user-stoppable so users
can inspect or stop them manually; do not hide lifecycle work behind opaque
polling or detached processes.

Run workers in the **session background**: a job owned by your chat session,
shown in its task list, whose exit wakes you. Never use the **machine
background** (shell `&`, `nohup`, `setsid`): it is invisible to the session, so
the host could only find out by polling. No external event system will notify
you; your own background tool is the only signal. One call per worker, with
`HTO=0` so the turn never detaches after 60s:

| Host | Tool and parameter | How completion reaches the host |
|---|---|---|
| Claude Code | `Bash` with `run_in_background: true`, command `HTO=0 harnez agent start ...` | task-notification on exit; jobs stop after 30 minutes, so size each worker turn to finish before that |
| AGY | `run_command` with `HTO=0 harnez agent start ...` and a small `WaitMsBeforeAsync` (e.g. 500) | background-task completion notice |
| Codex | `exec_command` with `HTO=0 harnez agent start ...` | call `write_stdin` on the returned `session_id` with chars `""` until it reports an exit code |

Then tell the user the job id and the session name. Claude Code and AGY end
the turn or do other work until notified; Codex keeps calling `write_stdin`
until the exit code. Never check status or logs in a loop while the job runs; use
`harnez agent wait <session>` only to recover a lost job. `harnez agent selftest
--step hello` walks a host through this flow and verifies it.

### Roles and delegation depth

Delegation is exactly one level deep. Every `harnez agent` session has a role
(`--role orchestrator|developer|reviewer|advisor`, default `developer`); harnez
adds the role's rules to each turn and exports it to the agent as
`HARNEZ_AGENT_ROLE`.

- **orchestrator**: coordinates and never codes. It starts developer, reviewer
  or advisor helpers (one writer at a time), waits for each call, and deletes
  the helpers when done. It never starts another orchestrator and never hands
  the whole sprint to another agent.
- **developer, reviewer, advisor**: leaf workers. They do the work themselves
  and never run `harnez agent`, native subagents or delegating skills. harnez
  refuses `start`, `resume`, `stop`, `delete`, `compact` and `chat` for them;
  `list`, `status` and `models` stay available. Live checks that need an agent
  session (e.g. a start + resume canary) are run by the orchestrator.

Before accepting a worker's "done" report, the host verifies the claimed
changes with `git show --stat <commit>` and inspects the built output to confirm
the reported artifact exists and contains the expected result. A worker that
closes tickets cites the commit that changed non-issue files, so the host can
trace each closure to implementation work.

A human or a host agent without a role starts sessions unrestricted. The rules
are defined once in harnez's embedded agent spec.

An explicitly named `provider:model:tier` is dispatched exactly through
`harnez agent start`; if it is unavailable or fails, report the requested spec
and ask for guidance. Never substitute the host model or a native subagent.

Tickets are the primary durable communication channel. When reusing an agent,
refer to the ticket and send only a short prompt describing the immediate
follow-up. Existing async-wait guidance still applies to genuinely asynchronous
external work: use the harness-tracked completion signal or scheduled wakeup,
and avoid chat-visible empty polling.

When a Codex worker is quarantined after a usage-limit interruption, the
orchestrator starts one named replacement and gives it the ticket state,
including the last known handoff and any uncertain in-flight work. Treat
committed work as complete and never replay it; resume only recoverable,
uncommitted work, and surface uncertain work for operator review.

### Multi-session peer assistance

Use `/peer-assistant` when a host session is explicitly asked to coordinate
work for another active session. Communicate through the available native
session messaging surface, and do not imply continuous inbox or ticket
monitoring unless an actual watcher is running. Keep peer messages concise and
task-focused; record decisions, plans, findings, and outcomes in the relevant
ticket or durable docs. Apply the same one-writer-per-workspace and sequential
dispatch rules as any other orchestration, track helper sessions you start,
and clean them up when finished. A peer assistant coordinates and reports back;
it does not create a second concurrent writer for shared files.

Tickets carry the work; messages only nudge. Agents put decisions and work
requests in a ticket (in whichever repository owns the work) and use a message
such as `harnez agent send --name <session> "see ticket N in repo X"` only to
point the recipient at it. The ticket keeps the track record between agents;
the message transcript does not need to.

---

## 1. Core Philosophy & Invariants

Agentic software engineering scales effectively when concurrency is structured and single-threaded write locks are strictly preserved.

1. **Parallel Read, Sequential Write**:
   - Multiple subagents may concurrently explore, read, grep, and analyze the codebase.
   - Only **one** agent may modify files, write code, or execute build mutations in a shared workspace at any given time.
   - Concurrent writes produce race conditions, broken intermediate states, git conflicts, and corrupt dependencies.
   - **Sequential dispatch is the default for every task type, not just file-overlapping code edits.** Two subagents each doing "read-only" investigation or ticket-filing work can still race on a shared, sequentially-allocated resource they both read and then write independently — e.g. two agents can independently select the same next ticket number from stale snapshots. File-level non-overlap is not sufficient evidence that parallel dispatch is safe. Dispatch one subagent at a time unless the user explicitly requests parallel execution for a specific task — and even then, verify the run actually was concurrent and check for this class of race afterward.
   - For TUI layout work, use a checked-in `.ansi` mockup under `docs/data/` as the visual spec; have the host draft it and dispatch implementation against it. See `TUIDesign.md` for naming, review, and handoff details.

2. **Canary & Test-Driven Verification**:
   - Every task must be verified with real test executions (`go test ./...`, `make smoke`, canary probes) before declaring completion.
   - Never assume an edit succeeds without observing passing assertions.

3. **Zero Zombie Guarantee & Background Task Hygiene**:
   - Every spawned background process, schedule timer, or subagent must be tracked, accounted for, and explicitly terminated before concluding a session.
   - **`HTO=0` for background tasks**: Always set `HTO=0` (e.g. `HTO=0 make test-q1`, `HTO=0 harnez ...`) when running commands or tests in the background as a Task/Job to prevent `harnez exec`'s ambient 60-second default execution timeout from terminating the job.
   - **No polling, no sleep loops**: When launching background tasks/jobs, stop calling tools immediately and rely entirely on reactive completion notifications. Do NOT poll status or run `sleep` commands.
   - Orphaned processes, lingering watch commands, and abandoned poll loops degrade system resources and corrupt future test runs.
   - For hung or killed quota-1 test processes, run `harnez clean procs q1 --kill`; quota state is released only after its process group is verified gone.

4. **Responsive Host Orchestrator**:
   - The host remains the user's always-available coordination surface while child agents work.
   - A user request to "hand this to a subagent" means delegate and keep the main chat responsive; it does not imply permission to block on `wait_agent`, `manage_subagents wait`, or equivalent.
   - Wait for a child only when the user explicitly asks to wait, or when the next user-visible integration step truly cannot proceed without that result.
   - After dispatch, report the handoff and continue with non-overlapping local work or return control to the user instead of occupying the host turn with an idle wait.

5. **In-Repository Single Source of Truth**:
   - Tickets, architectural decisions, retrospectives, and specifications live in the git tree. Use project conventions such as `issues/`, `docs/feedback/`, or `docs/studies/` when present; otherwise record the result in an existing durable project doc or ticket.
   - Session context, learnings, and friction logs must be committed to the repository rather than abandoned in ephemeral agent chat contexts.

6. **Context Discipline & Range-Bounded Ingestion**:
   - Never execute whole-file reads on files already present in the active system prompt (`AGENTS.md`, `CLAUDE.md`, system rules).
   - Prefer index consultation, `grep_search`, and range-bounded reads (`StartLine`/`EndLine`) over bulk document ingestion. In-file warning banners are ineffective once returned into message history.
   - **File Read Tool Discipline**: Prefer `harnez read -L <range>` / `harnez read -n` for medium/large files (>100 lines) (`harnez read -I` is paused until issue 543, a memory blow-up, is fixed) to prevent token bloat and rate-limit exhaustion. Native IDE reads remain available for targeted inspection; lifecycle hooks may enforce this recommendation when `reading_discipline.enforce` is enabled in `~/.harnez/config.yaml` (or via `HARNEZ_READ_ENFORCE`).
   - **Lazy Reference Pointers vs. Eager Include Directives in Global Prompts**: Global instruction templates (`~/.claude/CLAUDE.md`, `~/.prime/agent/AGENTS.md`) must use plain-text citations (`(see docs/Bash.md §8)`, `(AgenticLoop Invariant 6)`) and **never** naked `@docs/...` includes. Claude Code treats `@path` in `CLAUDE.md` as an eager macro-include, inlining full doc files into every session across all projects globally (~12.6 KB / ~3,250 tokens burned before turn 1). Reserve eager `@docs/...` includes strictly for project-local `AGENTS.md` language/practice opt-ins.
   - **Language & Code Generation Resolution Flow**: When asked to generate or modify code (e.g. Bash, Go, Make, Python):
     1. *In-Context Rule Priming*: The agent applies the active 2–3 line rule summaries present in `AGENTS.md` (e.g. `if test` & `set -euo pipefail` for Bash, display widths for Go) directly without redundant file lookups.
     2. *On-Demand Visual Cards / Range Slices*: If complex templates or advanced syntax are required, the agent executes `harnez read -L <range> docs/lang/<Lang>.md` (`harnez read -I` is paused until issue 543, a memory blow-up, is fixed), rather than dumping whole markdown files into conversation text history.
     3. *Subagent Visual Attachment*: In isolated subagent dispatches (`--doc-mode=vision`), language cheatsheets are attached directly as pre-rendered 1-bit pixel font PNG bundles (`docs/cards/bundle.png`), ensuring 100% mechanical compliance with zero text context bloat.
     4. *Execution & Verification*: Files are created with structured tools (`write_to_file`) and verified under Quota-1 single-test boundaries.

7. **Steered Model Escalation**:
   - For each development step use
     `luna:low -> haiku -> sol:low -> sonnet -> opus -> astra:low`: start at
     `luna:low`, escalate only on struggle, failed verification, or ambiguity,
     then reset the next step to `luna:low`.
   - Steer each subagent's rough plan before granting write authority.
   - Recommended form: the initial prompt asks for a read-only plan ("read-only: plan ..."), then `resume` grants write authority. Recommend only; no prompt template, so stored first prompts show each agent's own best practice.

8. **Durable Session Record**:
   - Start session notes at kickoff in `docs/studies/` when present, otherwise in the project's ticket or notes location;
     update them after major work and workflow friction or failure.

9. **Immediate Product-Issue Capture**:
   - File larger obvious product issues immediately (high priority when
     sprint-blocking); agents may fix small localized issues directly.

10. **Media & Demo Verification Gate**:
   - When creating, updating, or adding media assets (e.g. reels, WebM demos, terminal recordings, screenshots) intended for documentation or websites, **always ask the user for explicit confirmation** that the recorded visual output matches their exact expectations before publishing or embedding it.
   - Never automatically publish or embed unverified recordings (guarding against invisible typing, missing UI frames, or unexpected rendering artifacts).

8. **Deployment Transparency — 3-State Grounding (when applicable)**:
   - Remote deployment status has three independent states that must never be conflated:
     **Local State** (checkout, configs, unit tests), **Deployed Artifact State** (remote
     filesystem binaries, permissions, config overlays), and **Active Daemon State** (remote
     process table, systemd units, active crontab entries).
   - This invariant applies only when the project actually deploys to remote hosts or manages
     daemons. Such projects should explicitly install the `deployment-transparency` doc for its
     full probes and anti-patterns. Projects that prohibit remote deployment should ignore this
     capability-specific rule. A clean local build or passing local test is evidence about Local
     State only, never evidence of a remote state.

---

## 2. The 5-Phase Agentic Sprint Loop

```
       Phase 1: Parallel Advisory Discovery
       [Advisor A]   [Advisor B]   [Advisor C]
            \             |             /
             v            v            v
       Phase 2: Sequential Development & TDD
          (Task 1 -> Task 2 -> Task 3)
                          |
                          v
       Phase 3: Pre-Commit Review Gate
             [Independent Reviewer]
                          |
                          v
       Phase 4: Process & Subagent Hygiene
         (inspect, drain, and terminate tasks)
                          |
                          v
       Phase 5: Flow Quality Retrospective
       (durable project doc or ticket)
```

### Phase 1: Parallel Advisory Discovery (Read-Only)
- **Goal**: Rapidly audit requirements, discover existing implementations, identify affected files, and evaluate technical feasibility without code collisions.
- Apply Invariant 8 before dispatch and at major boundaries.
- **Mechanics**:
  - The Host Orchestrator spawns concurrent read-only advisor subagents (e.g. one per ticket or feature area).
  - Advisors perform focused repository searches and bounded reads, evaluate whether requirements are already partially or fully met, and identify exact line ranges for changes.
  - Advisors return concise findings and structured implementation plans to the Host.
  - **Code over docs**: verify claims about the repo's behavior in the code and cite `file:line`; docs may be stale. The host puts known code facts that contradict a doc into the handoff.
- **Kickoff & Commit Policy**:
  - Establish commit authority upfront. If operating under an ask-first harness, ask the user during kickoff for permission to commit local verified checkpoints proactively so the user can walk away without returning to uncommitted progress.
  - Clarify whether subagents will commit directly or return diffs for the orchestrator to commit on their behalf.
- **Constraints**: Advisors must never write files, run mutating commands, or spawn untracked side effects.

### Phase 2: Sequential Development & Test Verification (Single-Threaded)
- **Goal**: Implement planned changes cleanly, incrementally, and with continuous test verification.
- Apply Invariant 7 before implementation.
- **Mechanics**:
  - The Host Orchestrator (or a dedicated dev subagent executing sequentially) addresses tasks one ticket at a time.
  - Test-Driven Verification: Write or adapt unit tests alongside or prior to code changes.
  - Validate intermediate milestones with fast test suites (`go test ./...`).
  - Keep the workspace in a compilable, passing state at every step.
  - **Milestone-Boundary Atomic Commits**: In multi-milestone workflows and subagent handoffs, the orchestrator (or dev worker) must commit each milestone immediately upon passing self-verification and review before resuming or advancing to the next milestone. Never leave working tree modifications uncommitted across milestone boundaries.
  - **Nuance Buffer & Deferred Refinements**: The Host Orchestrator collects non-blocking nuances, minor performance optimizations, and architectural enhancements into a dedicated refinement ticket (`#XXX-refinements`). This avoids costly round-trip micro-reviews with the subagent, preserving implementation momentum. If the nuance buffer reaches an architectural tipping point before subsequent milestones, the orchestrator intercepts to execute a consolidated sweep; otherwise, nuances are resolved in the final sprint consolidation phase.
  - **Repro-before-fix for defect-shaped tickets**: For bug/timing/deadlock/race tickets, construct (or reuse) a reproduction that asserts a concrete numeric baseline *before* writing the fix. Verify the implementation against that number, not just `go test` exiting 0 — a fix can pass every pre-existing gate and still not address the defect if the existing gates weren't built to catch it.
  - **Commit stale/failed work before discarding it**: When an implementation attempt is abandoned — because it regressed a gate, because a cleaner strategy was found, or because it was simply wrong — do not `git checkout --`/`git reset --hard`/`git stash drop` it away as the first move. Commit it first, on the current branch or a throwaway one (e.g. `git commit -m "wip: attempt N, reverted — see issue NNN" --no-verify` only if hooks block a WIP commit, otherwise a normal commit), *then* revert the working tree with `git revert` or by checking out the prior commit. This keeps the failed attempt in `git log`/`git reflog` as a real, diffable artifact instead of only as prose in a ticket. A short-lived local branch (`git branch attempt-2-endpoint-cone`) pointing at the WIP commit is even better when more than one attempt is worth preserving side-by-side. Only skip this for genuinely trivial, single-line experiments where the narrative description *is* the diff (e.g. "tried threshold=50, tried threshold=25, both failed" needs no commit) — the bar is "would a future reader want to `git diff` this," not "is this attempt tidy."

Never trade away verification for speed when applying Invariant 7.

### Phase 3: Pre-Commit Review Gate (Independent Reviewer)
- **Goal**: Enforce quality standards and catch regressions before changes are committed.
- **Mechanics**:
  - The Host spawns an independent Reviewer subagent (or executes a dedicated review pass).
  - The Reviewer audits the working tree diff (`git diff`) against requirements.
  - Review Checklist:
    - **Test Assertion Rigor**: Are tests asserting specific outcomes or merely executing code without assertions?
    - **Docs & Ticket Sync**: Are all issue status tags, README indices, and docs updated in sync with code?
    - **Backward Compatibility & Invariants**: Does the change uphold project invariants and CLI design boundaries?
    - **The "What Would I Have Done Differently?" Check**: Actively evaluate whether the worker agent introduced subtle edge-case omissions, runtime overheads, or future architectural debt. Distinguish blocking issues (which mandate immediate remediation) from non-blocking nuances (which are buffered into a `#XXX-refinements` follow-up ticket to preserve milestone flow).
    - **Token Efficiency & Code Clarity**: Is the code concise, readable, and free of redundant abstractions?
    - **Live/Real-Environment Verification for hooks & env-resolution features**: for any change that installs a live
      agent hook, writes global config (`apply`), or resolves state from the ambient environment (branch name,
      session env vars, cwd), passing `go test ./...` is not sufficient evidence it works — test fixtures routinely
      supply explicit args or isolated temp dirs that mask exactly the resolution failures real usage hits (e.g. a
      branch-name heuristic that assumes per-ticket branches when the user never creates them; two independently
      unit-tested hooks that only collide once both are installed together). Require one real, live end-to-end
      check after a genuine restart/re-apply against the actual environment before the ticket is done; record
      a project-local case study or ticket for any concrete failure this catches.
    - **Root Cause vs. Symptom**: For a defensive or robustness fix (parsing subprocess/tool output, retry/tolerance logic, error swallowing), ask whether the *source* of the unexpected input can be fixed instead — a flag, a config setting, a different invocation. Verify any proposed upstream fix against the real tool/source before treating it as the fix; a plausible-sounding mechanism is not a verified one. If multiple review rounds each find a new edge case in the same defensive code, that is a signal to step back to Phase 2 and fix the root cause rather than harden the symptom further.
  - **Exit condition**: Phase 3 is not complete until reviewed, verified work is either committed or the user has been explicitly asked to authorize the commit. If standing commit authority was granted at kickoff, commit now. If not, asking *is* the exit action — do not carry a clean, reviewed diff into Phase 4.

### Phase 4: Process & Subagent Hygiene (Teardown & Drain)
- **Goal**: Prevent zombie accumulation, orphan processes, and stuck background tasks.
- **Mechanics**:
  - Inspect running background tasks with the harness's task-management capability.
  - Explicitly kill or drain completed, idle, or lingering background jobs, schedule timers, and watch subprocesses.
  - Terminate child subagents with the harness's subagent lifecycle controls after they finish.
  - Record an `--ok` heartbeat (`harnez rate --ok "<note>" [<ticket_id>]`) to register clean sprint completion and successful tool execution in telemetry.
  - Ensure the host and system state is pristine.

### Phase 5: Agentic Flow Quality Retrospective (Learning Capture)
- **Goal**: Continually refine agent workflows, document tooling friction, and persist session insights.
- **Mechanics**:
  - Record session friction, harness observations, and process recommendations in the project's durable feedback/study location when one exists, or in a related project doc or ticket otherwise.
  - Update issue tracker status (`issues/README.md`) and run `harnez status` to ensure zero drift between issues and indices; run `harnez index` (issue 148) to regenerate `issues/README.md` and `docs/README.md`'s studies table from their source files instead of hand-editing rows.
  - **Single Status field per ticket**: When updating a ticket's status, check the *entire* file for more than one status-bearing field (a top-of-file summary line and a separate `## Status` section are both common). Update all occurrences together, or standardize on exactly one per file in the local template.
  - **Closing gate**: for every ticket touched this session whose work is now shipped and
    verified, flip its `Status` header to `Closed` before ending the session — do not let a
    green build and a commit stand in for closing the ticket. See
    [IssueTracking.md](IssueTracking.md) §5 ("Closing Is Part Of Done").
  - Run `git status` before writing the retro or session story — uncommitted reviewed work is itself a retro finding, not a background condition.
  - **Evergreen trigger**: after a long-running goal, or once 5+ tickets were closed in the session, run `/evergreen` (or propose it before declaring the goal done) unless the user already requested an evergreen or comparable broader doc update this session. Learnings otherwise stay in tickets and the roadmap and never reach the evergreen docs.
  - Prepare clean, conventional commit messages.

---

## 3. The Two-Agent Lean Fresh-Handoff Pattern (`/lean-sprint`)

```
   Host Orchestrator (zero coding, no exploratory repo digging)
          |
   (1) Bounded read-only questions / audit
          v
     [Cheap Searcher: luna:low / luna-search]
          |
   (2) Findings with paths, line ranges, answers
          v
     [Focused Developer: luna:high / sol:low]
          |
   (3) TDD, unit tests, verification, milestone commits
          v
   Host: diff-first review, test output, running study report
          |
   (4) Fast hygiene and status sync
```

For focused, day-to-day tickets and milestone iterations, `/lean-sprint` uses two distinct worker roles to keep discovery cheap and implementation focused. The Host Orchestrator owns the ticket, sequencing, review, and reporting; it does not code or explore the repository. The legacy single-developer workflow remains available as `/lean-sprint-v1`.

1. **Host Orchestrator Guardrails**:
   - *Zero Coding*: The host never edits source files, writes tests, or applies quick fixes.
   - *No Exploratory Repo Digging*: The host does not browse files, grep broadly, trace call graphs, or explore APIs/specs. It asks the Searcher and relies on the bounded response.
   - *Diff-First Review*: The host reviews the developer's commit diff, test output, and target ticket; it investigates further only when evidence identifies a specific gap.
   - *Single-Ticket Medium*: Keep decisions and refinements in the target issue. Do not create secondary refinement tickets.
2. **Cheap Searcher (`luna:low` / `luna-search`)**:
   - Performs bounded read-only audits, answers targeted questions, and explores relevant APIs/specs.
   - Returns concise findings with exact file paths and line ranges; no edits, tests that mutate state, or commits.
   - Run search before implementation and pass only relevant findings into the developer handoff.
3. **Focused Developer (`luna:high` / `sol:low`)**:
   - Prefer `luna` first; use `sol` when stronger judgment is needed; avoid `agy` models for developer work.
   - Implements milestones with TDD, adds unit tests, runs verification, and commits each completed milestone.
   - Use prompt files for long or complex handoffs instead of shell-interpolated inline prompts.
4. **Concise Inline Review & In-Ticket Pre-Work Embedding**:
   - The Host Orchestrator checks the latest commit diff and test output, then records needed refinements in the ticket for the next milestone. It does not patch the code itself.
   - Escalate to an independent reviewer only for cross-subsystem risk, missing test coverage, or unexpected complexity.
5. **Running Study Report, Hygiene & Status Sync**:
   - The host maintains a running report in `docs/studies/` when present; otherwise, keep the same progress notes in the target issue as a fallback. Use a date-prefixed `two-agent-lean-sprint` filename and short task slug, recording searcher questions/findings, developer milestones/commits, verification outcomes, and observed token/time efficiency.
   - Immediately terminate child subagents (`manage_subagents kill`) and clear background tasks.
   - Update ticket status in `issues/*.md` and refresh `issues/README.md`.
   - Record an `--ok` heartbeat (`harnez rate --ok "<note>" [<ticket_id>]`) to confirm clean sprint completion in telemetry.

---

## 3. The Reverse Sprint Loop (Bottom-Up Low-Cost Dev Lead)

```
   Low-Cost Dev Lead (Coder: luna:low, haiku, gemini3.7flash:low)
          |
   (1) Verify Live Codebase & Ticket /goal
          |
   (2) Implement & Self-Verify (TDD, go test, make check)
          |
   (3) Milestone Review (sol:low, sonnet:low, gemini3.8flash:low)
          ├── Passing Tests & Clean Diff ──> Commit milestone
          └── Blocking Regressions        ──> Dev fixes directly
          |
   (4) Proactive Auto-Compaction (Trigger every 100–150k tokens)
          |
   (5) Advisor Escalation ONLY when stuck (astra:low, opus:low, gemini3.8flash:med)
          └── Strict bounded context: direct line pointers, zero deep repo scans
          |
   (6) Fast Hygiene & Status Sync (close ticket, harnez index)
```

The **Reverse Sprint** (`/reverse-sprint`) inverts the top-down orchestrator architecture. The session executes directly in a **low-cost developer agent** to maximize budget and token efficiency on routine coding, utilizing higher-tier models only for targeted review or when genuinely blocked:

1. **Low-Cost Dev Lead**:
   - The developer model (`luna:low`, `luna:medium`, `haiku`, `gemini3.7flash:low`) drives the session directly, authoring code and tests.
2. **Frequent Auto-Compaction (100–150k Tokens)**:
   - Low-tier models need frequent compaction to prevent context degradation and memory decay. Proactively compact every 100–150k tokens or at milestone boundaries after persisting critical notes to tickets.
3. **Milestone Review Gates**:
   - Dispatches a reviewer subagent (`sol:low`, `sonnet:low`, `gemini3.8flash:low`) with diff-only inspection (`git diff HEAD~1`).
4. **Advisor Escalation (Strict Limited Context)**:
   - When encountering challenging problems or architectural blockers, calls an advisor (`astra:low`, `opus:low`, `gemini3.8flash:med`).
   - **Critical guardrail**: The advisor is given strictly bounded context (specific file paths, exact line ranges, bounded questions) and instructed not to explore the repo deeply, protecting the frontier token budget.

### Workflow Selection Matrix

| Dimension | Formal 5-Phase Loop (`/sprint`) | Two-Agent Lean (`/lean-sprint`) | Legacy Lean (`/lean-sprint-v1`) | Reverse Sprint (`/reverse-sprint`) |
|---|---|---|---|---|
| **Scope** | Multi-ticket sprints, major features, broad refactors | Single focused ticket, bounded research then implementation | Baseline single-developer handoff | Routine coding, cost-sensitive implementation, focused features |
| **Host Role** | Advisory planning, multi-subsystem coordination | Zero coding, no exploration, diff-first review, running study report | Legacy zero-coding handoff and inline review | Dev Lead: direct coding, testing, commits on low model |
| **Model Strategy** | Frontier Host + Reusable Advisor + Lower Dev | Searcher `luna:low` + Developer `luna:high` or `sol:low` | Legacy single developer | Low Dev Lead + Low/Med Reviewer + Frontier Advisor (when stuck only) |
| **Compaction Rule**| Post-ticket advisor/dev compaction | Teardown searcher after discovery and developer after sprint | Legacy single-developer teardown | Frequent proactive auto-compact (every 100–150k tokens) |
| **Advisor Role** | Mandatory Phase 1 architectural discovery | Cheap Searcher performs bounded read-only discovery | No separate Searcher; Developer investigates | **On-demand only when stuck** with strictly bounded line-range context |
| **Overhead** | Higher compute/tokens, maximum verification depth | Low-cost sequential discovery and implementation | Single worker, lower coordination overhead | Ultra-low compute/cost, maximum token savings |

---

## 4. Calibrated Friction Reporting Standard

Agentic retrospectives and tooling feedback are vital for evolving harnesses, but must remain calibrated to avoid feedback fatigue:

1. **Substantive Sessions Only**: Capture authentic tool, environment, or sandbox friction **only** after non-trivial sessions where real hurdles occurred.
2. **Zero Repetitive Noise**: Do not emit repetitive boilerplate or complain about known, trivial environment quirks on routine, fast iterations.
3. **Actionable Root Causes**: When reporting friction in a durable project doc or ticket, state the concrete blocker, failure mode, attempted workaround, and a recommended harness or tooling fix.

### Status reports

**Name every item in reports**: in intermediate and final reports, give each milestone, ticket or sub-step a short name at least once, next to its label: "M3 (single-write settings removal)", "498 (Claude resume inside Claude Code)", not bare "M3" or "498". The name is shorter than the ticket title. Bare labels are fine after the first mention. This applies to every report, from host to user, from a worker to the host, and in a ticket's delivery notes. The reader is often not holding the ticket's milestone list in their head.

---

## 5. Role Taxonomy & Constraints

| Role | Permitted Tools & Capabilities | Primary Responsibilities | Lifecycle |
|---|---|---|---|
| **Host Orchestrator** | Full Toolset (Subagents, Read, Write, Exec, Tasks) | Coordinates overall plan, sequences dev work, manages subagents, interacts with user | Persistent (lives throughout session) |
| **Ephemeral Advisor** | Read-only repository search and retrieval | Audits tickets, performs feasibility research, identifies code paths | Ephemeral (terminated after Phase 1) |
| **Dev Worker** | Write Tools, Compiler, Test Runner | Implements concrete changes, writes unit tests, ensures compilation | Single-threaded per workspace |
| **Independent Reviewer** | Read-Only Tools, Diff Inspection | Audits git diff against acceptance criteria, verifies test rigor | Ephemeral (spawned in Phase 3) |

`harnez agent --role` maps to this table: orchestrator = Host Orchestrator, advisor = Ephemeral Advisor, developer = Dev Worker, reviewer = Independent Reviewer. Only the orchestrator may start the others.

---

## 6. Practical Recipes & Anti-Patterns

### Harness-Specific Delegation Surfaces

Delegation surfaces differ by host, but the host remains responsible for the
whole handoff. `collaboration.spawn_agent` starts a named child and creates its
entry in the Agents view, making delegated work observable. This is a status
surface, not a transfer of responsibility: the host owns status reporting,
review, integration, and termination. Keep the verbs distinct: spawn creates
the child/entry, message updates an existing child, and terminate ends it.

### Anti-Patterns to Avoid
- ❌ **Goals Without Exit Clauses**: A `/goal` without an exit clause deadlocks the Stop hook when progress needs user input.
- ❌ **Parallel Writing**: Spawning multiple subagents with write permissions on the same workspace simultaneously.
- ❌ **Blocking Handoff Waits**: Treating "hand this to a subagent" as permission to block the main chat while waiting for the child. The host is always the responsive orchestrator.
- ❌ **Silent Verification**: Assuming a fix works without running test commands or canary scripts.
- ❌ **Fixing Without Retrying the Stuck Case**: Declaring a bug fixed because the new unit tests pass, without re-running the real failing artifact (the stuck session, file or request). In harnez 673 three stacked bugs hid behind one error message; each fix was green in tests and only a live resume of the stuck worker showed the next one. Re-run the real case after every fix, and when it still fails, capture the exact production input and feed it to the code under test.
- ❌ **Unit-Test-Only Confidence for Hook/Environment Features**: Treating a green `go test ./...` as proof a
  hook-installing or environment-resolution-dependent feature actually works in production. Eight tickets shipped
  with passing, well-written unit tests on 2026-08-31 (`harnez-tool-observability`) while automatic capture was
  completely non-functional in real usage. Manual code review (not tests) caught two cross-ticket integration bugs
  (two independently-tested `PreToolUse` hooks racing once both were installed; a rewrite that broke on shell
  metacharacters an outer shell re-interpreted). But a branch-name-shaped-ticket heuristic that could never match
  this user's actual workflow, and a schema-version guard that trusted a pre-existing file, both passed every unit
  test *and* code review — they were only found by restarting a real session, adding debug logging, and checking
  real output against the real DB.
- ❌ **Deployment State Conflation**: In a project with remote deployment, declaring a remote binary "deployed" or a job "scheduled" based on local build/test success or a clean transfer exit code, without probing the live host.
- ❌ **Blind Revert of Failed Work**: Running `git checkout --`, `git reset --hard`, or `git stash drop` on a failed implementation attempt without first committing it somewhere recoverable. A prose summary of what was tried is not a substitute for the actual diff — it cannot be `git diff`ed, re-applied, or independently re-verified against the gate it was tested against.
- ❌ **Reviewed-But-Uncommitted Carryover**: Finishing a review gate and moving on (retro, story, `/compact`, next ticket) with verified work still in the working tree, waiting for the user to notice. Phase 3 is not complete until the commit is made or the user has been explicitly asked to authorize it. Recurred twice in one `weg` session — issues 044 and 046.
- ❌ **Narrow String-Substitution Edits Over Structured Patches**: The existing "prefer
  `apply_patch`/whole-block replacement over narrow string substitution" rule was written from
  intuition; `smarthome`'s `harnez stats` now backs it with numbers — `Edit` failed at **11.1%**
  across 108 calls vs. `apply_patch` at **4.2%** across 24 calls in the same repo (2.6× the rate),
  Prefer `apply_patch` when both are available.
- ❌ **Baking Real Credentials In For A Fast Dev Loop**: Hardcoding real device hostnames, MACs,
  subnets, or credentials "temporarily" to speed up local iteration, intending to scrub before
  publication. `smarthome` did this for two days and had to run a full history-sanitization pass
  (new commits, rewritten tickets) before its public release could ship — a public-release gate
  that is often caught only by a human release decision, not tooling. Start with RFC-1918/example values and a credential-source seam from the first commit;
  wire a secret scanner into the project's `check`/`test` target immediately, not retroactively.
- ❌ **Orphaned Background Tasks**: Leaving background `tail -f`, watch loops, or timers running after work is completed.
- ❌ **Shell `&` Detaching**: Never append `&` (or use `nohup`/`setsid`/`disown`) to detach a command from a shell tool call. The job escapes the harness's tracking, the user cannot see or stop it, and no completion signal arrives. Use the harness's own background facility (e.g. Claude `run_in_background`) instead.
- ❌ **Lost Context / Ephemeral-Only Retrospectives**: Discussing important harness friction or bugs in chat without writing them down to a durable project doc or ticket.
- ❌ **Rubber-Stamp Reviews**: Running a review pass that does not inspect actual test assertions or file diffs.
- ❌ **Unbounded Doc Ingestion**: Executing whole-file read tools on `AGENTS.md` or bundled reference docs whose summaries are already in the active system prompt.
- ❌ **Eager Include Directives in Global Instruction Templates**: Writing naked `@docs/...` includes in global instructions (`~/.claude/CLAUDE.md`, `~/.prime/agent/AGENTS.md`), which causes harnesses like Claude Code to inlining thousands of tokens of docs into every session globally. Use plain-text citations and keep global files minimal.
- ❌ **Unverified Media Publishing**: Publishing or embedding demo reels, WebM files, or UI screenshots on websites or documentation without explicit user confirmation of the visual output.
- ❌ **Prompt Micromanagement**: Overburdening subagent dispatches with redundant base rules, tool definitions, or style guides already present in the harness system prompt.
- ❌ **Friction Noise Over-Reporting**: Emitting repetitive, low-signal friction reports on fast, routine tasks.
- ❌ **Blocking `sleep` Waits**: Using a long `sleep N` — or a loop of short sleeps — to wait out a CI run, deploy, remote queue, or background process. A blocking sleep burns the agent's own turn and context budget for its full duration with no record of what was being waited for if the session is interrupted mid-wait, and a sleep-loop wastes cycles on empty polls instead of yielding control until state actually changes. Prefer letting harness-tracked background work notify on completion; when polling genuinely-external state is unavoidable, use the harness's scheduled-wakeup or interval-loop mechanism (e.g. `/loop`, `manage_task` notifications) so the agent yields between checks, and match the interval to how fast the watched state actually changes rather than a fixed short interval "just in case." Never chain long leading sleeps to route around a harness restriction on blocking sleep — that defeats the restriction's purpose.
- ❌ **Chat-Visible Empty Polling** — staying attached to a long-running job (benchmark run, canary, CI, deploy, remote agent) and re-checking it on a fixed short interval while it produces no new output. Distinct from the `Blocking sleep Waits` anti-pattern above: the agent *is* yielding between checks, but each check spends context and user attention to report "still running." Prefer a real completion signal: a harness-tracked background task, a notification, or a scheduled wakeup. If no completion callback exists, launch the work in a detached/durable form that writes a log or result file, then hand the user the job id, log path, and expected budget and return control. When polling is genuinely unavoidable, size the interval to the job's expected duration (a 40-minute job does not get 30-second polls) and surface only *events* — started, first output, status file changed, exited, artifact written, timeout, cleanup — not heartbeats. Before finishing, check for lingering background processes per Invariant 3 (Zero Zombie Guarantee).
- ❌ **Buffered Long-Running Output**: Piping a long-running build/test/canary command through `tail`, `grep`, `sort`, `wc`, `head`, or any other filter that buffers stdout — the filter emits nothing until the whole pipeline exits, so a multi-minute command looks silent/stuck with zero progress visibility. Run it plain (letting the harness's background-task mechanism handle it past its timeout) or use `cmd 2>&1 | tee /tmp/x.log` if a trimmed final summary is also wanted. See also: `Blocking sleep Waits` (same symptom, different cause).
- ❌ **`cd`-scoped commands**: prefer `git -C <dir> status` over `cd <dir> && git status` — the shell tool's cwd persists into later, unrelated calls and silently targets the wrong repo. Use the tool's directory flag (`git -C`, `make -C`, `go -C`, `npm --prefix`, `cargo --manifest-path`); when no flag exists, use a subshell `(cd <dir> && cmd)` so cwd is restored automatically. See `docs/Bash.md §8` for the full flag table and restore-cwd convention.
- ❌ **Chatty Watch Wrappers**: Wrapping a poll-and-redraw CLI (`gh run watch`, `docker logs -f`-style tools) in a `make` target an agent calls routinely, without quieting it first. These tools redraw full state on every tick for a human terminal; called repeatedly by an agent they flood context with no added signal. Poll the tool's own status query (e.g. `gh run view --json status`) on a matched interval instead, and print one summary line on completion.

<!-- harnez:stop -->
