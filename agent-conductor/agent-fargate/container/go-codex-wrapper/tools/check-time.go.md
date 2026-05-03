# check-time

Use this tool to check how long the current Fargate agent task has been running.

Command:

```bash
codex-wrapper --check-time
```

Expected behavior:

- Prints elapsed runtime in whole minutes.
- If the runtime budget is exceeded, prints the final report guide by invoking
  the ending behavior.

Use this periodically during long work so the agent does not run past the
configured task budget.
