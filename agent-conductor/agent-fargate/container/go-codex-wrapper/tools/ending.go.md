# ending

Use this tool only when the agent is ready to stop work and write its final
report.

Command:

```bash
codex-wrapper --ending
```

Expected behavior:

- Prints the final report guide.
- The report guide tells the agent what to write to
  `/tmp/agent-meta/ending-report.md`.

Do not call this at the start of a task. It is intentionally named `ending` to
discourage early use.
