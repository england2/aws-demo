# Worker completion and GitHub publication flow

The worker runs one Codex thread against the unpacked work files in `/worker/work`.
Codex receives an initial prompt that defines the filesystem contract, git branch
contract, and early-exit protocol. The ending prompt still writes the canonical
report to `/worker/work/agent-meta/ending-report.md`.

## Runtime paths

The worker and Codex communicate through a small set of fixed paths:

```text
/worker/work
/worker/work/repo
/worker/work/repo/<repo-specific-subdir>
/worker/work/agent-meta
/worker/work/agent-meta/WAS_JOB_SUCCESSFUL
/worker/work/agent-meta/ending-report.md
/worker/work/agent-meta/pr-message.md
```

`/worker/work/repo` should contain exactly one Git repository in an immediate
child directory when the job succeeds. `/worker/work/agent-meta` stores the
success marker, final report, generated GitHub body markdown, and any future
agent metadata files.

## Success marker

`WAS_JOB_SUCCESSFUL` is the worker-side switch. Codex writes `true` only when the
repo work is complete, committed on a feature branch, and ready for PR creation.
Codex writes `false` when the task failed, was blocked, was not attempted, or
should not create a PR.

`false` is a valid agent outcome, not an infrastructure error. The worker skips
successful artifact validation and PR creation in that case.

## Validation loop

The worker uses one Codex thread and multiple `Run` calls directly from `main`:

1. Run the initial task prompt.
2. Run the ending report prompt.
3. Read `WAS_JOB_SUCCESSFUL`.
4. If it is `false`, write the GitHub report body and create a failed-agent
   GitHub issue when a repo is available.
5. If it is `true`, call the artifact validation function from `main`.
6. If validation fails, run a correction prompt in the same Codex thread.
7. Retry validation up to four total attempts.
8. After the fourth failed attempt, report the validation failure to the
   conductor through the existing `WorkerSendsCodexError` gRPC method.

Successful validation requires:

- exactly one Git repository under `/worker/work/repo`;
- the repo is on a named feature branch, not `main` or `master`;
- the branch has a committed diff relative to `origin/main` or local `main`;
- the repo working tree is clean, so the PR does not omit uncommitted files;
- `/worker/work/agent-meta/ending-report.md` exists and is non-empty.

The git check treats “has git changes” as a branch delta from `main`, not a dirty
working tree. A branch should be clean after commit and still have committed
changes relative to `main`.

## GitHub publication

The worker writes `/worker/work/agent-meta/pr-message.md` with:

- the full final report;
- a details block containing the full persisted Codex transcript JSON from
  `ThreadRead(... IncludeTurns: true)`.

When the success marker is `true`, the worker shells out from the repo directory:

```bash
git push -u origin <feature-branch>
gh pr create --base main --head <feature-branch> --title "<derived title>" --body-file /worker/work/agent-meta/pr-message.md
```

When the success marker is `false`, failed-agent reports use GitHub Issues
instead of no-change PRs. A failed attempt is a task/status artifact, while PRs
imply a real diff and merge intent.

```bash
gh issue create \
  --title "[agent-failed] <derived title>" \
  --body-file /worker/work/agent-meta/pr-message.md
```

If no repo is available for a failed run, the worker skips GitHub issue creation
and still uploads `/worker/work` back to the conductor.

## Implementation shape

`main` stays as orchestration:

1. establish Codex;
2. establish gRPC;
3. request work files;
4. ensure runtime dirs exist;
5. run Codex and validate artifacts;
6. write the GitHub body markdown;
7. create a PR or failed-agent issue;
8. upload work files;
9. send shutdown.

Filesystem, git, and GitHub shelling live outside `main` in
`worker-files-github.go`. Codex transcript reading and validation prompt helpers
live in `worker-runtime.go`; the actual Codex `Run` calls stay visible in
`main`.

The directory creation call intentionally stays simple and explicit:

```go
// /worker/work/repo/: contains the single task repository under one variable child directory.
// /worker/work/agent-meta/: contains worker-agent protocol files, final reports, and GitHub body markdown.
ensureDirsExist([]string{
	workerRuntimePaths.RepoRootDir,
	workerRuntimePaths.AgentMetaDir,
})
```

## Verification

Worker tests cover:

- success marker parsing;
- missing and invalid success marker failures;
- successful Git artifact validation;
- main-branch and missing-report validation failures;
- GitHub markdown body creation with transcript details;
- fake `git` and `gh` command execution for PR creation.

The worker module should pass:

```bash
cd conductor-and-worker/worker/go-worker
/home/t/.go/bin/go test ./...
/home/t/.go/bin/go build ./...
```
