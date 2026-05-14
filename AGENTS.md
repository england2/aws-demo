## Working Style

**Complete the plan, then check in**: When a plan is approved, execute all
steps to completion. Don't stop after each step for review. When you think
you're done, recursively apply all relevant principles from this file — check
each one, act on any that apply, then check again until no more principles
are relevant. Only then report completion and wait for feedback.

**Plans require discussion before implementation**: After devising a plan
(whether in plan mode or not), run the review loop (see "Mandatory review
checkpoints") before presenting it. Do NOT proceed to implementation until
the plan has been seen and explicitly approved.

**Mandatory review checkpoints**: At each of these points, run the full
review loop — spawn a fresh-context reviewer subagent, address findings,
spawn another fresh reviewer, repeat until a reviewer finds no issues. When
you disagree with a reviewer's finding, escalate — do not resolve disputes
unilaterally. Do not proceed past a checkpoint without a clean review.
1. **After devising a plan**, before presenting it for discussion. For plan
   reviews, adapt the reviewer prompt: instead of reading changed files and
   running tests, the reviewer should read the plan document, read existing
   code the plan references, verify assumptions about the codebase, and check
   for structural gaps (missing steps, naming conflicts, incorrect
   dependencies).
2. **After completing implementation and self-review**, before opening a PR.

The only exception: if you believe a change is truly trivial (a typo fix, a
one-line config change), ask for permission to skip the review. Do not decide
on your own that something is trivial enough to skip. When in doubt, run the
review.

**Discuss important decisions before acting**: When encountering an important
decision point — architectural choices, tradeoffs between approaches, anything
that could meaningfully change the direction of work — stop and discuss first.
Don't pick a path silently.

**Apply principles before escalating decisions**: Before presenting a design
decision as an open question, check whether existing principles already resolve
it. If a principle clearly answers the question, apply it and state which
principle you used. Only escalate decisions that the principles don't cover or
where multiple principles conflict.

**Challenge me when the evidence says I'm wrong**: I use Claude reviews because
I can be wrong. If a reviewer flags something that contradicts what I said, or
if Claude has concrete evidence that my instruction is incorrect, raise it —
don't silently comply. The whole point of the review process is catching
mistakes from everyone, me included. Present the evidence and discuss it. This
isn't pushback; it's the collaboration working as intended.

**Cross-project lessons go here**: When we discover insights, patterns, or
techniques that are useful across projects (not just the current one), write
them to this file, not to project-specific memory files where they'll get lost.

**Ask about project conventions**: Always ask whether we want to preserve the
existing coding patterns, unless the answer is already recorded (e.g., in a
project CLAUDE.md). The answer may be: preserve them because we like them,
preserve them for consistency even if we don't prefer them, or intentionally
deviate from them. Don't assume — the choice depends on context.

**Go slow to go fast**: Before starting implementation work, identify and state
which principles from these instructions are most relevant to the current task.
This surfaces the right guidelines before they're needed rather than
rediscovering them after a mistake.

**Research findings belong in the plan**: If research or exploration surfaces
issues beyond the original task (inaccurate comments, dead code, related bugs),
include them as explicit plan steps — don't just mention them in the analysis
and move on. Anything worth noting is worth acting on or explicitly deferring.
For findings outside the current branch's scope, file a GitHub issue to track
them.

**Self-review is part of "done"**: The recursive principle check described in
"Complete the plan, then check in" IS the self-review. It's not a separate
step — it's what "done" means. Never report completion without having done it.

**During reviewer loops**: At any point during the review loop — when fixing
findings, when unsure about a reviewer's suggestion, when making tradeoff
decisions — stop and ask. The automated review removes me as a gatekeeper, not
as a collaborator. After a clean review at the pre-PR checkpoint, go straight
to opening the PR — don't stop to report "ready for PR" and wait for
permission. Open the PR, then report completion with the PR URL.

**Questions aren't corrections**: When I ask about code, don't assume I'm
flagging a problem. I often ask to confirm my understanding or verify intent.
Respond with a clear, direct confirmation rather than a defensive explanation.
I'll say explicitly if something is wrong.

**Principle review**: A structured code review that combines general correctness
checking with systematic evaluation against the principles in this file.

Automated mode — the writer session spawns a reviewer subagent:
1. The reviewer must start with fresh context — do not use an agent type that
   inherits the writer's conversation history.
2. The prompt must include:
   - Instructions to read CLAUDE.md for principles and project context.
   - The sources to review (branch name, base branch, design doc location).
   - Instructions to read all changed files in full (not just the diff), plus
     supporting files needed for verification.
   - Instructions to build and run tests.
   - Deliver the two-part review (general correctness + principle-by-principle).
   - Context from prior reviews, if any: settled decisions, opened issues, or
     fixes already in progress.
3. Factual accuracy requirement: The prompt must instruct the reviewer to verify
   every factual claim by reading the actual code. Do not summarize from memory
   or inference.
4. Dispute resolution: Fix agreed-upon findings independently; only escalate
   items where you disagree with the reviewer. Present the dispute to me. I
   rule. Pass the ruling to the next reviewer as prior-review context.

## Code Design Principles

1. **Prefer explicit over implicit**: When the language or framework allows
   something to work "by magic" (implicit conversions, convention-based wiring,
   unnamed dependencies), prefer the version that states what's happening
   directly. The cost of a few extra characters or lines is almost always less
   than the cost of someone later needing to reconstruct the hidden knowledge.
   Several principles below are specific applications of this idea.

2. **Make illegal states unrepresentable**: Centralize validation at the
   construction boundary so the rest of the code can trust its inputs. In
   strongly typed languages (F#, Rust, Haskell), lean on the compiler with
   private constructors and factory methods that return error-or-value. In
   dynamic languages (Python, Ruby, JS), enforce this by convention with
   validated types that are trustworthy once constructed. The strictness is a
   gradient depending on your language's type system, but the principle —
   validate once at creation, trust everywhere after — is universal.

3. **Errors are data, not exceptions**: Each layer should define its own error
   vocabulary as a concrete type (enum, union, sealed class). Higher-level
   errors wrap lower-level ones to preserve full context. Every error type
   should know how to describe itself as text. This gives exhaustive handling,
   no information loss during propagation, and clear error provenance.

4. **Separate data shape from data validity**: For complex types, define a "raw"
   structure for the data shape and a validated wrapper that guarantees
   correctness. Construction goes: raw shape -> validation -> validated wrapper.
   The rest of the system works with the validated form.

5. **Define separate types for each data boundary** (applications): In
   applications with multiple boundaries, user input, database records, and API
   responses should be distinct types even when they represent the same concept.
   A database record has an auto-generated ID; user input doesn't. Making these
   distinct prevents mixing concerns.

6. **Prefer qualified/namespaced references**: Even when the language lets you
   import names unqualified, prefer namespaced references (e.g., Module.foo over
   foo). The cost of a few extra characters is outweighed by the clarity of
   knowing where something comes from and avoiding name collisions as the code
   grows.

7. **Ask about sensitive data**: When handling data, ask if any of it is
   sensitive and if yes, how it should be handled. The answer may be redaction,
   encryption, masking, or something else depending on context.

8. **Separate domain logic from orchestration from presentation**
   (applications): In applications with distinct layers, domain types should
   have zero infrastructure dependencies. Orchestration combines domain logic
   with infrastructure (databases, caches). Presentation adapts orchestration
   for a specific protocol (HTTP, GraphQL, CLI).

9. **Design for changeability, not for predicted changes**: Make designs modular
    and replaceable so future needs can be accommodated, but don't add
    abstractions, extension points, or features for changes that haven't
    happened yet. The goal is a design that's easy to modify, not one that
    anticipates specific modifications.

10. **Document coupling at the point of breakage**: When code A depends on the
    internal behavior of code B (read sequence, execution order, size
    assumptions), put the comment on B — that's where a future maintainer would
    make a breaking change. Commenting at A ("we depend on B") doesn't help
    because the person changing B won't be reading A.

11. **Distinct semantics deserve distinct representations**: When two values have
    different meanings or different handling semantics, represent them as
    separate types even when one could technically serve for both. Overloading a
    single type to carry multiple meanings forces callers to use out-of-band
    knowledge to distinguish them.

12. **It is easier to give than take away**: When deciding whether to include
    something in an API (a callback, a parameter, a feature), lean toward
    omitting it. You can always add it later if needed, but removing it is a
    breaking change. Start minimal; expand based on demonstrated need.

**Present evidence before executing corrections**: When told to undo or change
something, and you have concrete evidence for why it was done that way (not just
opinion), share the evidence before acting. The user may not have the same
context you do. This isn't pushback — it's making sure decisions are informed.
Execute the change after sharing, unless the user reconsiders.

## Code Change Discipline

**Read before modifying**: Do not propose changes to code you haven't read.
Understand existing code before suggesting modifications.

**Prefer editing over creating**: Edit existing files rather than creating new
ones. This prevents file bloat and builds on existing work.

**Avoid over-engineering**: Only make changes that are directly requested or
clearly necessary.
- Don't add error handling, fallbacks, or validation for scenarios that can't
  happen.
- Don't add docstrings, comments, or type annotations to code you didn't change.
- Don't create helpers, utilities, or abstractions for one-time operations.
- Don't design for hypothetical future requirements. Three similar lines of code
  is better than a premature abstraction.

**Evaluate copied patterns, don't cargo-cult them**: When reusing a pattern from
existing code, evaluate each choice (var vs let, error handling style, data
structure, etc.) in the context of the new usage. The original may have had
reasons that don't apply, or it may have been a mistake in the original. Copy
the intent, not the incidental choices.
- Pre-action check: Before reusing a pattern from existing code, ask: "Does the
  new usage actually need each piece of this?" Strip it down to what's required
  first, then add back only what's justified. The default should be the simplest
  version, not a copy of the most similar existing code.
- Distinguish conventions from technical patterns: Conventions (legal headers,
  naming schemes, file organization) exist for consistency or organizational
  reasons — follow them. Technical approach patterns (error handling style, test
  methodology, data structures) should be evaluated on merit for the new
  context. When in doubt about which category something falls into, the presence
  of the pattern across all files in the repo suggests convention.

**No backwards-compatibility hacks**: Don't rename unused variables to _var,
re-export removed types, or add "// removed" comments. If something is unused,
delete it.

**Fix what your change makes stale**: When a change invalidates something
elsewhere — a comment, a docstring, a test description, documentation, a
configuration reference — fix it in the same PR. Stale artifacts left behind are
bugs in the making, and "I didn't modify that line" isn't an excuse when your
change is what made it wrong.

## Testing

### Test Style Preference

**Favor property-based tests (or generative variants) over example-based unit
tests.** Property-based tests define invariants that hold for all inputs and use
generated data to find counterexamples. Generative tests use the same generator
machinery but in a more example-based style — e.g., a "valid inputs" generator
exercising code to confirm all are accepted, and an "invalid inputs" generator
confirming all are rejected.

### Property-Based & Generative Testing Patterns

1. **The Valid/Invalid/Mixed generator triad**: For any validated type or input
   boundary, create three coordinated generators: one that only produces valid
   inputs, one that only produces invalid inputs, and a mixed generator that
   wraps both. This yields three properties: "good data always succeeds," "bad
   data always fails," and "mixed data succeeds if and only if it's the valid
   variant." The mixed property is the strongest — it asserts the exact boundary
   between acceptance and rejection.

2. **Invalid generators should cover every failure mode**: Invalid input
   generators should use the equivalent of oneof across distinct failure modes
   (too short, too long, invalid characters, reserved words, etc.) rather than
   just generating random bad data. This exercises all rejection branches, not
   just the easiest-to-hit path.

3. **Derive generators from validation rules**: Build generators mechanically
   from the same constants and rules the validators use (min/max length, allowed
   character sets, regexes, etc.). Valid generators produce inputs matching the
   rules; invalid generators negate them. This eliminates drift between what the
   validator checks and what the generator produces.

4. **Compositional generator hierarchy**: Compose complex generators from
   simpler validated ones. A generator for a composite type should be built from
   generators for its constituent parts. Each level reuses the generators from
   below, so complex valid inputs are always internally consistent. This is the
   property-based testing equivalent of builder patterns.

5. **Test from multiple angles**: Look for ways to verify the same behavior from
   more than one direction. This could mean comparing two independent
   implementations, checking a result against a derived invariant, or
   roundtripping through encode/decode. Testing from multiple angles catches
   bugs in both the implementation and the test logic itself.

6. **Balance edge-case coverage against iteration speed**: When generating test
   data, bias toward smaller/simpler inputs for fast feedback while still
   exercising expensive edge cases (max-size inputs, boundary conditions) at a
   lower frequency. The goal is a healthy mix — most runs iterate quickly, but
   unlikely/extreme scenarios still get covered regularly rather than never.

7. **Supplement property tests with examples for unreachable paths**: When code
   dispatches across multiple paths based on value size (format families,
   encoding tiers, protocol variants), constrained property generators will only
   cover some paths. A generator producing strings 0–100 chars exercises fixstr
   and str_8 but never str_16. The property test is still valuable for the
   boundary it tests (accept/reject at the limit), but it silently leaves entire
   code paths uncovered. Add targeted example-based tests for the dispatch paths
   generators can't efficiently reach.

### Counterfactual Testing ("Make It Fail, Make It Pass")

**Always do this** after writing new tests unless you truly can't find a way to
break the assertion and are very confident the test is correct. A test you've
never seen fail is a test you don't trust. Temporarily break each assertion to
confirm it actually fires:
1. Make a targeted change that should cause exactly one assertion to fail
2. Run tests, confirm the expected failure message
3. Revert the change

**Key takeaway**: When a counterfactual passes (assertion doesn't fire), that's
the most valuable outcome — it means the assertion is weak/wrong. Treat
counterfactual testing as a bug-finding technique, not just a confidence ritual.

**Workflow rule**: After writing a new test and seeing it pass, do NOT report
success yet. First, do a counterfactual check. Only report the test results
after the counterfactual confirms the assertions are meaningful.

### Debugging Discipline

**"How do you know that you know that?"**: When debugging, a hypothesis about
the cause is not knowledge — it's a guess. Never act on an unverified
hypothesis. Before investing effort in workarounds or fixes, validate
empirically that your suspected cause is actually the cause. Sometimes that's a
minimal test that isolates one variable; sometimes it's examining the actual
data instead of assuming what it contains; sometimes it's reading the code more
carefully. The method varies, but the discipline doesn't: verify first, then
act.

**Probe external data shapes empirically**: When consuming external data sources
(APIs, files, databases), verify the actual data shape with a real probe —
don't trust documentation or reasoning alone. A single API call, database query,
or file inspection is worth more than any amount of documentation reading or
inference.

**CI is the source of truth for build status**: A local build failure does not
mean the build is broken. Local toolchain versions, stale dependency caches, and
environment differences can all cause local failures that don't reproduce in CI.
Never declare a build "broken on main" based on local results — check CI first.


# Code Style

## Naming Conventions

### Verbose, Descriptive Naming

Agents must prefer highly verbose and self-descriptive names for variables, functions, types, and constants. Generated code should optimize for readability and unambiguous intent rather than brevity.

Avoid abbreviations, shortened forms, and generic identifiers unless they are already established within the surrounding codebase.

Preferred:

```python
validated_configuration_file_contents
serialized_http_response_payload
calculate_total_retry_backoff_duration
```

Avoid:

```python
cfg
resp
calcBackoff
```

---

### Functions That Modify Program State Must Use Explicit Verbs

Functions that mutate state, perform side effects, persist data, send data, or trigger external behavior must begin with a concrete action verb that accurately describes the operation.

Examples of suitable verbs include:

* `create`
* `update`
* `delete`
* `persist`
* `load`
* `store`
* `send`
* `enqueue`
* `dispatch`
* `synchronize`
* `register`
* `invalidate`
* `append`
* `remove`
* `commit`
* `write`

- Existing verb name schemes in the codebase are also to be respected and applied.

Preferred:

```python
update_cached_user_profile
persist_session_state_to_disk
send_metrics_payload_to_ingestion_service
enqueue_pending_email_delivery
```

Avoid vague or non-actionable names:

```python
handle_user
process_data
run_task
do_update
manage_cache
```

Functions that are pure computations or read-only queries should instead use descriptive non-mutating forms:

```python
calculate_retry_delay_duration
build_serialized_response_payload
is_configuration_file_valid
get_active_session_identifiers
```

---

### Variables Representing External Data Must Include Source or Destination Context

Variables used to receive, transport, or send data should explicitly encode the data source, transport boundary, or destination system in the identifier.

Preferred:

```python
postgresql_user_record
redis_cached_session_payload
incoming_kafka_message
outbound_websocket_frame
s3_uploaded_archive_url
http_request_headers
http_response_body
```

Avoid ambiguous transport-neutral names:

```python
data
payload
message
response
result
obj
```

When applicable, include directional semantics:

```python
incoming_api_request_payload
outgoing_email_message_body
persisted_audit_log_entry
```

---

### Existing Identifiers Must Be Referenced Exactly

Agents must preserve and reuse existing naming conventions and identifiers already present in the codebase.

Do not rename variables, functions, parameters, classes, or fields unless explicitly instructed to do so.

If the surrounding code uses:

```python
userAuthToken
```

then generated code must continue using:

```python
userAuthToken
```

and must not introduce:

```python
user_auth_token
authToken
token
```

Consistency with the local code context takes precedence over global style preferences.

# Project Specific notes
- Local Go binary: `/home/t/.go/bin/go`
- Prefer the local Go toolchain for `go test`, `go build`, and related checks instead of Docker.
- Format command: `scripts/format`
- Proto generation: `scripts/gen-proto`
- This is a learning project whose structure should model a production program; Therefore, we have to get design decisions correct.
- There is no need to restore deleted binaries or care about their tracking -- don't worry about it and move on.
- `plans-docs-etc/codex-sdk-go-guide.md` contains notes on how to use the go codex sdk.
- Don't spend time on formatting thought-loops unless it’s directly needed for compilation; when it’s not, just call it out as a next step for me.
- Always ensure your reasoning is spent on solving problems, not wrangling with tools.
- Don't hand-write SQL-accessing code in go! Refer to `docs/external/sqlc-docs.md` to see a guide on using sqlc.
