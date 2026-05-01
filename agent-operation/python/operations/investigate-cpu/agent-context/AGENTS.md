# Investigate CPU Operation

You are responding to an AWS CloudWatch CPU alarm for the `debian-cpu-spin` EC2 instance.

Your objective is to create a small, reviewable GitHub pull request that fixes the CPU-spinning behavior in the source code.

## Required Inputs

- Read `alert.txt` first.
- Extract the AWS event ID from the alert JSON field `id`.
- Use that event ID to name your branch.

## Hard Directives

- Do not modify infrastructure unless the alert clearly proves the infrastructure is the cause.
- Prefer fixing application code under `test-applications/cpu-spin/`.
- Keep the change minimal.
- Do not rewrite unrelated files.
- Do not delete project files.
- Do not merge the PR.
- Do not force-push to `main`.
- Do not commit secrets, AWS credentials, private keys, or local machine paths.
- If the intended fix is unclear, create a PR with the smallest safe diagnostic or rollback-style code change and explain the uncertainty in the PR body.

## Repository

The repository is:

```text
https://github.com/england2/aws-demo
```

Clone sparsely. Fetch only `test-applications/`.

```bash
git clone --filter=blob:none --sparse https://github.com/england2/aws-demo.git
cd aws-demo
git sparse-checkout set test-applications
```

Work only in the relevant app path:

```text
test-applications/cpu-spin/
```

## Branch Naming

Create a branch named:

```text
investigate-cpu-<alert-id>
```

If the full event ID is too long for comfort, use:

```text
investigate-cpu-<first-12-chars-of-alert-id>
```

## Expected Fix

The likely failure mode is that the CPU-spin test application was changed into an unhealthy mode that burns CPU.

Your fix should make the application return to healthy behavior:

- print a healthy startup message
- avoid CPU-burning loops
- sleep/block safely without causing a Go deadlock

## Useful `gh` Commands

Check authentication:

```bash
gh auth status
```

Sparse-clone only `test-applications/`:

```bash
git clone --filter=blob:none --sparse https://github.com/england2/aws-demo.git
cd aws-demo
git sparse-checkout set test-applications
```

Create and switch to a branch:

```bash
git switch -c investigate-cpu-<alert-id>
```

View changed files:

```bash
git status --short
```

Commit the fix:

```bash
git add test-applications/cpu-spin
git commit -m "fix: stop cpu-spin from burning CPU"
```

Push the branch:

```bash
git push -u origin investigate-cpu-<alert-id>
```

Create the PR:

```bash
gh pr create \
  --base main \
  --head investigate-cpu-<alert-id> \
  --title "fix: stop cpu-spin from burning CPU" \
  --body "Responds to CloudWatch alert <alert-id>. This PR restores cpu-spin to healthy non-CPU-burning behavior."
```

Check the PR:

```bash
gh pr view --web
gh pr status
```

## Completion Criteria

You are done when:

- the code change is committed on a non-main branch
- the branch is pushed
- a PR to `main` exists
- the PR body references the alert ID and summarizes the fix
