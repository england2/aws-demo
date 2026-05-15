# Report Guide

This guide defines what to include when generating a final work report.

The report is produced by running the special report command. Assume this command is the last action before shutdown. Do not defer investigation, cleanup, or additional checks until after the report command runs.

## Purpose

A report is a concise operational summary of the work performed by the agent.

It is **not** a pull request message, changelog entry, marketing summary, or success announcement. Its main value is diagnostic: it should help a human understand what made the task difficult, where the agent lost time, what failed, what was uncertain, and what risks remain.

Positive outcomes may be mentioned, but the report should prioritize headaches, frictions, blockers, failed attempts, degraded assumptions, and unresolved issues.

## Core Reporting Principles

1. Be concrete.
   - Prefer specific files, commands, errors, services, APIs, tests, and symptoms.
   - Avoid vague statements such as "encountered some issues" unless immediately followed by details.

2. Report friction even if the final task succeeded.
   - A successful end state does not erase the difficulties encountered.
   - Include workarounds, retries, unexpected behavior, confusing interfaces, missing context, and fragile assumptions.

3. Distinguish facts from guesses.
   - Clearly separate confirmed outcomes from suspected causes.
   - Mark uncertainty explicitly.

4. Do not hide failure.
   - If the task failed, partially failed, or was abandoned, say so directly.
   - The report should make it easy to diagnose why.

5. Keep successful reports terse.
   - If the job succeeded cleanly and there were no meaningful frictions, keep the report short.
   - Do not inflate the report with routine implementation details.

## REQUIRED REPORT STRUCTURE

Use the exact report structure below. Keep the headings exactly as written.

The very first line of `/worker/work/agent-meta/ending-report.md` must be a concise plain-text GitHub title. This first line will be used as the pull request or failure issue title and will be removed from the rendered report body. Do not prefix it with `#`, `Title:`, or any other label. Keep it under 120 characters.

```markdown
Concise PR or issue title goes here

## Agent's Job Understanding

State your high-level understanding of the user's requested task in 2-5 sentences.

Do not list routine setup actions such as reading TASK.md, reading AGENTS.md, starting the worker, or opening the repository unless one of those actions revealed a meaningful blocker.

## Task Success Opinion

Write exactly one of these lines:

- Task succeeded.
- Task failed.

Then add 1-3 sentences explaining the reason.

## Code Alteration Description

Describe the meaningful source code, configuration, documentation, or test changes made during the task.

Focus on behavior and intent, not a diary of actions. Do not report trivial process steps such as reading files, inspecting directories, creating the report file, or writing the success marker.

If no code or repository files were changed, say so directly and explain why.

Include a very small code snippet only when it clarifies an important change. Do not include large diffs or unrelated command output.

## Code Alteration Verification

State what checks were run and what their results were. Include tests, builds, linters, formatters, manual runtime checks, or targeted inspections.

If checks were skipped, say why. Do not claim verification that was not actually run.

## External System State Changes

State whether any command may have changed external or durable system state outside the task repository.

Examples include Terraform apply, AWS resource changes, Docker container starts/stops, database migrations, GitHub issue/PR creation, package publication, remote deployments, or modifying secrets.

If none were attempted, write: "None known."

If uncertain, say what might have changed and why.

## Frictions and Headaches

List only meaningful difficulties that would help a human or future agent improve the environment or continue the work.

Include important errors, failed commands, missing context, tool limitations, permissions issues, timeouts, dependency problems, confusing requirements, or unexpected runtime behavior.

Do not list harmless retries, routine command failures that were immediately corrected, or ordinary exploration steps unless they changed the outcome or consumed meaningful time.

If the task succeeded cleanly and there were no meaningful frictions, write: "None significant."

## Remaining Risks or Follow-Ups

List unresolved problems, assumptions, incomplete work, fragile areas, or anything a human should inspect next.

If there are no known follow-ups, write: "None known."

## Additional Notes

Include only useful information that does not fit above.

Do not use this section for a second summary, a task log, or filler. If there is nothing useful to add, write: "None."

Do not list repository branch names, commit hashes, pull request URLs, or issue URLs just to record Git metadata. GitHub already shows that information around the report. Mention Git details only when they are directly relevant to a blocker or risk.

```

# What to Capture

Include relevant details from the agent's execution, especially:

- Commands that failed or produced surprising output.
- Error messages, stack traces, timeout symptoms, or AWS Fargate runtime constraints.
- Missing environment variables, credentials, permissions, files, packages, or network access.
- Conflicting or ambiguous user requirements.
- Tooling limitations, unavailable dependencies, incompatible versions, or filesystem restrictions.
- Retries, fallbacks, partial workarounds, and why they were needed.
- Tests, linters, builds, deployments, or checks that were run.
- Tests, linters, builds, deployments, or checks that could not be run.
- Any assumptions made to continue despite incomplete information.
- Any files or components most likely to need human review.

Do not include routine progress log entries such as:

- Read `/worker/work/TASK.md`.
- Read `/worker/work/AGENTS.md`.
- Ran `clone-repo.fish` successfully.
- Created `/worker/work/agent-meta/WAS_JOB_SUCCESSFUL`.
- Wrote `/worker/work/agent-meta/ending-report.md`.

Include those actions only when they produced a meaningful blocker, surprising result, or decision-relevant fact.

## If the Job Failed

When the job failed, the report should focus on diagnosis.

Include:

- The exact failure point, if known.
- The last meaningful action before failure.
- The most relevant error output.
- What had already been completed before the failure.
- What remains incomplete.
- Whether the failure appears deterministic, intermittent, environmental, permission-related, dependency-related, or caused by unclear requirements.
- The next concrete step a human or future agent should take.

Avoid presenting a failed job as mostly successful. Do not bury the failure in neutral wording.

## If the Job Succeeded

If the job succeeded and verification passed, keep the report relatively terse.

Still include meaningful frictions if any occurred.

Example:

```markdown
Add CLI argument support to number-adder

## Agent's Job Understanding

The task was to add command-line argument support to the Rust number-adder program.

## Task Success Opinion

Task succeeded.

The requested behavior was implemented, committed on a feature branch, and verified.

## Code Alteration Description

Added parsing for three accepted CLI arguments in `test-applications/number-adder/src/main.rs` and updated the usage output.

## Code Alteration Verification

Ran `cargo test` and `cargo run -- --help`; both passed.

## External System State Changes

None known.

## Frictions and Headaches

None significant.

## Remaining Risks or Follow-Ups

None known.

## Additional Notes

None.
```

## Style Requirements

- Be concise but not evasive.
- Use plain language.
- Prefer bullet points for scanability.
- Include exact names of files, commands, and failing components when useful.
- Do not write persuasive PR-style prose.
- Do not overstate confidence.
- Do not omit important problems to make the result look better.

## Final Reminder

The report command is the final action before shutdown. The report must capture the agent's best available understanding at that moment, including failures, uncertainty, incomplete verification, and any operational friction that would help a human debug or continue the work.

# Output 

Place your report in /worker/work/agent-meta/ending-report.md

Before finishing, confirm that /worker/work/agent-meta/WAS_JOB_SUCCESSFUL accurately contains `true` or `false`.
