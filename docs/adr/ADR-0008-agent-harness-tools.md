# ADR-0008: Agent-Harness Tools for layar-cli (read/write/grep/fetch/advisor/judge/critique/review/loop)

**Status**: Accepted
**Date**: 2026-07-18
**Deciders**: go-adk-q maintainers (layar-cli session)

---

## Context

`layar-cli` needed a standard coding-agent tool harness — read, write,
review, fetch, grep, advisor, loop, judge, critique — comparable to what a
harness like Claude Code or opencode gives its agent, built on this
project's actual infra (ADK Go), not ported from another project's code.

Two upstream constraints shaped the design:

1. **Charm stack stays v1** (`github.com/charmbracelet/...`). A parallel ask
   referenced a v2 (`charm.land/.../v2`) dependency table — that is
   crush/contrabass's stack, not opencode's (opencode is v1, and it is this
   project's explicit visual/UX reference). Migrating is a large, unscoped,
   one-way rearchitecture; it was declined for this work.
2. **Single-agent-turn architecture.** A separate reference (contrabass)
   describes a multi-agent swarm/board/queue event model
   (`AgentStarted`/`BackoffEnqueued`, `board_issue_created/moved`,
   `team/stalled`, queue dispatch blocked on dependencies). This TUI runs
   exactly one agent turn at a time (`AGENTS.md`: "Failover chain is
   sequential-only by design") — there is no concurrent multi-agent swarm,
   todo board, or dependency queue in this codebase. Building fake
   versions of those concepts to match another project's event schema
   would be over-engineering with no real behavior behind it.

---

## Decision

Build every harness capability that has a genuine referent in ADK Go's own
primitives; explicitly skip, not fake, the ones that don't apply here.

### New FunctionTools (`tools/` package)

| Tool | File | Notes |
|---|---|---|
| `read_file` | `tools/fs.go` | Confined to cwd (rejects absolute/`..` paths), 256 KiB cap, warns on credential-pattern filenames (`.env`, `id_rsa`, etc.) |
| `write_file` | `tools/fs.go` | Same confinement + cap; logs every write via `slog` for auditability |
| `grep_search` | `tools/search.go` | Pure Go `regexp` (RE2 — no ReDoS) + `filepath.WalkDir`, no shell-out (no command-injection surface); capped at 200 results, never silently |
| `fetch_url` | `tools/fetch.go` | http/https only; SSRF guard applied at **dial time** via a custom `Transport.DialContext` (re-validates the resolved IP, not just a pre-check — closes the DNS-rebinding gap a pre-check-then-dial approach leaves open); 1 MiB cap, 10s timeout |

Read/write/grep/fetch are wired unconditionally in `cmd/tui/main.go` — they
carry no more risk than the existing local, single-user trust model already
applied to `@path` attachments (plus the confinement/SSRF guards above,
which that precedent didn't need but new code can build in cheaply).

### New agents (`agents/` package, same panic-on-construction-error
convention as `agents/llmauditor.go`)

| Agent | File | Role |
|---|---|---|
| `advisor_agent` | `agents/advisor.go` | Holistic second opinion on a plan/approach — soundness, biggest risk, one adjustment |
| `judge_agent` | `agents/judge.go` | Rubric-based APPROVED/NEEDS_WORK scoring |
| `critique_agent` | `agents/critique.go` | Adversarial refutation — tries to break a claim/code rather than score it |
| `review_agent` | `agents/review.go` | The one agent given real tools (`read_file`, `grep_search`) — reviews actual files, not a state-interpolated string |
| `critique_loop` | `agents/harness_loop.go` | The "loop" capability: bounded (`MaxIterations: 3`) reviser⇄critic `LoopAgent` |

All five are wired via `agenttool.New(x, nil)` inside the existing
`GOOGLE_API_KEY`-gated block in `cmd/tui/main.go`, same precedent and same
stated reason as `llm_auditor` (reliable multi-step tool-calling needs
Gemini).

### Fixed a real bug found while researching this: `critique_loop` actually uses `exitlooptool`

Root `main.go`'s own reference `doc_refinement_loop` (`DocDrafter` ⇄
`QualityChecker`, lines ~277-310) does **not** include `exitlooptool` in the
critic's `Tools`, despite `AGENTS.md:190-202` documenting it as mandatory
("Otherwise it runs MaxIterations unconditionally (silent logic error)").
`QualityChecker` writes `"APPROVED"` to `quality_verdict` on iteration 1, but
nothing reads it — the loop silently runs all 3 iterations regardless,
wasting 2 LLM calls on an already-approved draft every time. This is
demo-only code in the `adk-q` binary, out of scope for a `layar-cli` change,
so it was **not modified** — but `critique_loop` in `agents/harness_loop.go`
is built with the exit tool correctly wired into the critic's `Tools`, so an
early approval genuinely stops the loop. Flagging this here so it doesn't
get re-discovered as a surprise later; fixing the demo is a candidate
follow-up, not part of this change.

### Observability: additive `slog` events, no new UI plumbing

- `tool_call` events (with a `kind: ToolCall` field) are emitted by each new
  FunctionTool handler.
- `agent_turn` events (`kind: AgentStarted` / `AgentFinished`) bracket each
  turn in `cmd/tui/chat.go`'s `startAgentStream`.
- The existing 429-retry log line in `model/failover/failover.go` gained a
  `kind: BackoffEnqueued` field (log-field addition only, no behavior
  change).
- All of the above land in the existing `$TMPDIR/go-adk-q-tui.log` sink
  (interactive `chat` mode only — `run` mode has no log redirect and was
  left as-is).
- **Deliberately not built**: a `tea.Msg`/visible event pane in the
  Bubbletea UI. It would touch the largest, most change-sensitive file in
  the repo (`chat.go`, ~2100 lines) for a benefit that could not even be
  visually confirmed in this environment (no live provider key to trigger a
  real tool-calling turn to watch flow through). The `slog` sink already
  gives real, auditable observability; the visible-pane version is a
  candidate follow-up, not a gap.
- **Deliberately not built, and not applicable**: `team/stalled`,
  `team/all_idle`, `team/missing`, `board_issue_created/updated/moved`,
  queue dispatch-blocked-by-dependencies. See Context above — there is no
  concurrent multi-agent swarm or todo/issue board in this codebase for
  these events to describe.

---

## Verification

- `go build ./... && go vet ./... && go test -race ./...` green after every
  file added.
- `tools/fs_test.go`, `tools/search_test.go`, `tools/fetch_test.go` — real
  temp files/dirs, a real `httptest.Server` (proves the SSRF guard actually
  refuses a live loopback server, not a mocked check), and a real fetch of
  `https://example.com/` (IANA's reserved test/documentation domain).
- Real binary run: `layar-cli --help`/`chat --help`/`run --help` all clean;
  `ECHO_FALLBACK_ENABLED=1 layar-cli run "..."` end-to-end with no crash.
- Real construction check: with a syntactically-valid but invalid
  `GOOGLE_API_KEY` set, all 5 new gated agents (plus `llm_auditor`)
  constructed without panicking, and a live call correctly failed over to
  echo — proving the larger tool/agent list doesn't break the composition
  root.
- Real interactive pty session (`expect` + `stty rows/cols`): every one of
  the 10 slash commands (`/settings /model /providers /theme /help /clear
  /skills /filepicker /acp /acpstop`) driven in sequence with no panic and a
  clean alt-screen exit.
- **Stated limitation, not claimed as tested**: whether an LLM actually
  *chooses* to call these new tools/agents requires a real `GOOGLE_API_KEY`
  — unavailable in this environment.

---

## Addendum (same day): ACP alignment — `cmd/tui/acp_server.go`

A follow-up request asked to "ensure there is an agent protocol for
read/write and etc" against https://agentclientprotocol.com. Deep research
(fetched live: `overview.md`, `initialization.md`, `session-setup.md`,
`prompt-turn.md`, `file-system.md`, `tool-calls.md`) found the existing
`acp_server.go` was **not** real ACP — it used made-up method names
(`session/create`, `message/send`, `message/stream`, `ping`) and a made-up
`initialize` response shape (`serverInfo`/`serverCapabilities` instead of
spec's `agentInfo`/`agentCapabilities`).

**Key finding, category-error risk**: ACP's `fs/read_text_file` /
`fs/write_text_file` go **Agent → Client** — the agent asks the *editor* to
read/write the editor's own unsaved buffers. This is the opposite
relationship from this repo's `read_file`/`write_file` harness tools (ADR-0008
main text), which the LLM calls directly through ADK. The two are unrelated;
"expose read/write over ACP" was not a coherent ask once the spec direction
is understood — flagging this explicitly since it's the kind of thing that's
easy to conflate.

**Fixed** (mechanical, spec-name/shape alignment, safe and testable without
an external ACP client):
- `initialize` → correct `agentInfo`/`agentCapabilities` response shape
  (`loadSession`, `promptCapabilities{image,audio,embeddedContext}`,
  `mcpCapabilities{http,sse}`, `authMethods`), integer `protocolVersion`.
- `session/create` → renamed to spec's `session/new`, correct
  `{cwd, mcpServers[]}` params / `{sessionId}` result.
- `message/send` → renamed to spec's `session/prompt`, correct
  `{sessionId, prompt: ContentBlock[]}` params / `{stopReason}` result
  (`stopReason` is spec-correct; the agent's actual reply text has no
  spec field on this synchronous transport, so it's carried under a
  clearly-labeled non-spec `response` key rather than silently dropped).
- `message/stream` (SSE): kept as a non-spec extension, but its frames are
  now shaped like real `session/update` notifications
  (`{"jsonrpc":"2.0","method":"session/update","params":{sessionId,update:
  {sessionUpdate:"agent_message_chunk",content}}}`) instead of ad hoc
  `message.started`/`message.completed` event names.

**Not implemented — architectural blocker, not an oversight**:
`fs/read_text_file`, `fs/write_text_file`, `terminal/*`,
`session/request_permission` are all Agent→Client: they need the agent to
push an unsolicited request to the client and await its reply mid-turn,
which requires a persistent bidirectional channel (stdio or WebSocket).
This server is HTTP request/response only. Implementing these needs a
transport change first — it's a prerequisite, not "more code on the current
server." `authenticate`/`session/load`/`session/set_mode`/`logout` are
omitted as genuinely unneeded by this single-session, no-persistence TUI.

**Verified, real execution**: new `cmd/tui/acp_server_test.go` — 5 tests,
each a real `httptest.Server` HTTP round trip through the actual
`handleRPC` handler (not a mock): `initialize`'s exact response shape,
`session/new`'s `sessionId`, `session/prompt`'s real bridge round-trip
(prompt text extracted from the `ContentBlock` array, real reply carried
back), empty-prompt rejection, and `fs/read_text_file` cleanly returning
`-32601 Method not found` rather than crashing. All pass. `go build`/`vet`/
`test -race ./...` green repo-wide after the change.

An interactive pty test of `/acp` (typing the command live) hit a
`Bubbletea`-textinput/`expect` keystroke-timing quirk in this test
environment (the slash-autocomplete menu didn't resolve to a submit
reliably under scripted `expect` input) — noted as a test-harness
limitation, not a product bug: the httptest-based tests above exercise the
identical `handleRPC` code path the real `/acp` command wires up, and the
same slash-command sweep two turns earlier (before this ACP work) had
already driven `/acp`/`/acpstop` successfully once each.

## Addendum (2026-07-21): ACP stdio Agent→Client completeness

A later session built the real spec-conformant transport (`cmd/tui/acp_stdio.go`, newline-delimited JSON-RPC over stdio — see that file's header) with exactly one Agent→Client method, `fs/read_text_file`, built end-to-end as the representative case. This addendum closes the remaining three: `fs/write_text_file`, `terminal/create|output|wait_for_exit|kill|release`, and `session/request_permission`.

**Scope, Q&A'd explicitly before writing code**: "plumbing only, no new wiring" — each new method follows `requestReadTextFile`'s exact original shape (now factored through a shared `sendRequest` helper once there were 7 near-identical consumers instead of 1), capability-gated where the spec defines a `clientCapabilities` flag (`fs.readTextFile`/`fs.writeTextFile`/`terminal`), tested against an in-process mock client over a real `io.Pipe`. **Not built**: wiring any of these into an actual caller (e.g. routing `write_file`/`exec_command` through the ACP client instead of local disk/subprocess when running under ACP). That's a materially bigger integration decision — it would change the just-shipped `exec_command` confirmation flow's behavior depending on transport mode — explicitly deferred to a future pass with its own scope.

**Exact wire shapes fetched from ACP's authoritative source, not guessed or re-derived from the rendered docs** (the rendered `agentclientprotocol.com/protocol/v1/prompt-turn` page was missing the `session/request_permission` request/response fields entirely — trailing off mid-description): pulled `schema/v1/schema.json` directly from `github.com/zed-industries/agent-client-protocol` via `gh api`, the same repo the rendered docs site is generated from. Confirmed field-for-field:
- `fs/write_text_file`: `{sessionId, path, content}` all required → empty result.
- `terminal/create`: `{sessionId, command, args?, env?[EnvVariable], cwd?, outputByteLimit?}` → `{terminalId}`.
- `terminal/output`: `{sessionId, terminalId}` → `{output, truncated, exitStatus?{exitCode,signal}}`.
- `terminal/wait_for_exit`: `{sessionId, terminalId}` → `{exitCode?, signal?}`.
- `terminal/kill` / `terminal/release`: `{sessionId, terminalId}` → empty result each.
- `session/request_permission`: `{sessionId, toolCall: ToolCallUpdate, options: []PermissionOption}` → `{outcome}`, where `outcome` is a discriminated union: `{"outcome":"cancelled"}` or `{"outcome":"selected","optionId":"..."}`. **No `clientCapabilities` flag gates this method** — unlike `fs/*`/`terminal/*`, it's always available per spec, so `requestPermission` has no capability check (confirmed directly in the schema, not assumed by symmetry with the others).
- `ToolCallUpdate`'s `content`/`locations` fields are a 3-way discriminated union (`content`/`diff`/`terminal` blocks) with no real caller yet to justify fully typing every variant — left as `[]json.RawMessage` rather than speculatively modeled, consistent with this repo's own YAGNI precedent elsewhere.

**Verified, not assumed**: `go build ./... && go vet ./... && go test -race ./...` green; `gofmt -l` clean. All 6 existing `acp_stdio_test.go` tests still pass unchanged after refactoring `requestReadTextFile` onto the shared `sendRequest` helper (behavior-preserving, confirmed by the pre-existing tests, not just by inspection). 11 new tests added: `fs/write_text_file` (mock-client round trip + capability rejection), all 5 `terminal/*` methods in one mock-client round-trip test plus one combined capability-rejection test, and `session/request_permission`'s two outcome variants (`selected` and `cancelled`) — the latter deliberately initializes the mock client with **no** `clientCapabilities` at all, proving the no-gate claim above rather than just asserting it in a comment. Rebuilt + reinstalled (`make install`).

**Not verified live**: no real ACP client (Zed or otherwise) is available in this environment to drive any of these seven methods end-to-end against — same standing limitation as the original `fs/read_text_file` addition.

## Alternatives Considered

- **Port contrabass's actual event-bus/board/queue architecture** — rejected;
  it models a multi-agent swarm this single-agent-turn TUI doesn't have.
  Faking the structure without the underlying concurrency would be
  cosmetic, not functional.
- **Migrate to Charm v2 alongside this work** — rejected; contradicts the
  literal "match opencode" reference (opencode is v1) and is a large,
  separate, one-way migration or rearchitecture — out of scope here.
- **Fix `doc_refinement_loop`'s missing `exitlooptool` while touching this
  area** — rejected for this change; it's demo-only code in a different
  binary (`adk-q`, not `layar-cli`), out of the stated scope. Documented
  above instead so it's not lost.
