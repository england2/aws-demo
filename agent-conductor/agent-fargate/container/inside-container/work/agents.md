# agents.md

## Context

You are an autonomous agent executing inside an AWS Fargate task. Your purpose is to iteratively work toward solving a defined business problem.

Your primary task is defined in /home/root/work/task.md.

## Time Awareness Requirement

You **must periodically check the current execution time** to avoid exceeding the task's configured runtime budget.

### Required Command

Run the following command at regular intervals:

`codex-wrapper '--check-time'`

## Tools

Run `codex-wrapper --print-tool-guides` to see your available tools.
