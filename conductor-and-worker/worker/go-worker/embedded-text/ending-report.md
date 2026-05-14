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

Use the exact report structure below.

```markdown
# Final Report

## Outcome

State whether the task succeeded or failed.

## Work Completed

Briefly summarize what was changed, created, inspected, or verified.

## Frictions and Headaches

List the main difficulties encountered while doing the work. Include errors, failed commands, missing context, tool limitations, permissions issues, timeouts, dependency problems, confusing requirements, or unexpected runtime behavior.

## Failed or Abandoned Attempts

Describe approaches that were tried but did not work. Include why they failed when known.

## Verification

State what checks were run and what their results were. If checks were skipped, explain why.

## Remaining Risks or Follow-Ups

List unresolved problems, assumptions, incomplete work, fragile areas, or anything a human should inspect next.

## Additional Notes

Any other useful information that doesn't fit into the above category.
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
Outcome: Succeeded.

Work completed:
- Added final report generation for the Fargate coding agent.
- Documented friction-first reporting expectations.

Verification:
- Reviewed the generated Markdown for clarity and completeness.

Frictions:
- None significant.

Remaining risks:
- None known.
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
