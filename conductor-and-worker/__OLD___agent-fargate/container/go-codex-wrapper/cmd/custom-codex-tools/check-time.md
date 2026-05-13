# check-time

Use this tool to check how long the current Fargate agent task has been running.

Command:

```bash
custom-codex-tools --check-time
```

Expected behavior:

- Prints elapsed runtime in whole minutes.
- If the runtime budget is exceeded, tells the agent to stop active work and produce the final result.

Use this periodically during long work so the agent does not run past the configured task budget.
