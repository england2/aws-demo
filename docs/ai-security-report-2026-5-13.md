# AI Security Report: AWS Conductor/Worker Platform

Date: 2026-05-13

Scope: practical security review of the AWS-backed conductor/worker execution platform, with emphasis on Terraform, GitHub Actions deployment, conductor/worker runtime code, file transfer, Docker usage, scripts, ad-hoc hosts, legacy directories, tracked dotfiles, and selected ignored local artifacts. Local secret and state artifacts were inspected only for risk indicators; secret values and raw message bodies are not quoted here.

Limitations: this is a source and local-artifact review. I did not query live AWS IAM trust policies or live resource settings. The local Terraform plan could not be rendered because the AWS provider schema was unavailable locally, so plan-file contents remain residual risk.

## 1. Executive summary

The largest risks are operational rather than cryptographic: public EC2 hosts with SSH, an exposed conductor gRPC port in Docker Compose, unauthenticated worker RPCs, root containers running Codex with no sandbox/approval guardrail, mutable `latest` image deployment, broad worker secret access, local Terraform state, and unsafe artifact/file-transfer patterns that can cause overwrite or denial of service.

No committed AWS access key pattern was found in tracked source. The repo does contain tracked and ignored infrastructure state/artifact files that expose account topology, ARNs, public IP/DNS outputs, task ARNs, and raw scheduler message bodies. Those are not API credentials, but they materially help an attacker map the environment.

## 2. Biggest immediate risks

1. Publicly addressable infrastructure and debug surfaces exist by design: EC2 instances get public IPs, SSH ingress is enabled, the conductor Compose file publishes `50055:50055`, and debug SSH is enabled in conductor deployment config.
2. Worker identity is just a caller-supplied `worker_id`; gRPC uses insecure credentials and has no TLS, mTLS, shared secret, or per-worker token.
3. The worker image runs as root and starts Codex with `approval_policy="never"`, `sandbox_mode="danger-full-access"`, and `default_permissions=":danger-no-sandbox"`.
4. The Fargate task role can read OpenAI, GitHub PAT, and debug SSH key secrets. Any container compromise can likely recover those values through the task credentials.
5. File transfer buffers whole zip archives in memory and extracts externally supplied zips with `unzip -o` after deleting the destination directory.
6. Deployment uses mutable `latest` tags and SSM shell commands, making rollback and provenance weaker than digest-pinned deployments.

## 3. AWS misconfiguration findings

### Finding: Public EC2 instances with SSH enabled

Severity: High

Affected files:

- `infra/terraform/cpu-spin.tf:1`
- `infra/terraform/cpu-spin.tf:89`
- `infra/terraform/agent-operation.tf:1`
- `infra/terraform/agent-operation.tf:68`
- `infra/terraform/variables.tf:24`

Evidence:

- Both EC2 security groups allow TCP 22 from `var.ssh_allowed_cidr`.
- Both EC2 instances set `associate_public_ip_address = true`.
- `ssh_allowed_cidr` validates `/32`, which is good, but SSH still remains an internet-reachable management path.

Why it matters: SSH exposed to the internet is a common scanner target. Even `/32` rules are easy to stale, mis-set, or later loosen during debugging.

How it could realistically happen: a developer applies Terraform with their home IP, then later changes networks, opens the CIDR for convenience, or leaves keys/SSH enabled after switching to SSM.

Concrete remediation: remove EC2 key pairs and SSH ingress from both hosts. Use SSM Session Manager only. If SSH is temporarily needed, create a separate break-glass module with expiration, strict `/32`, and an explicit variable such as `enable_public_ssh = true`.

### Finding: EC2 IMDSv2 is not explicitly required

Severity: High

Affected files:

- `infra/terraform/cpu-spin.tf:89`
- `infra/terraform/agent-operation.tf:68`

Evidence:

- Neither `aws_instance` includes `metadata_options`.
- Both instances attach IAM instance profiles.

Why it matters: instance role credentials are available through metadata. IMDSv2 reduces SSRF and local process credential theft risk.

How it could realistically happen: a web process, shell script, container, or compromised user on the host reaches IMDS and steals the attached role credentials.

Concrete remediation:

```hcl
metadata_options {
  http_tokens                 = "required"
  http_endpoint               = "enabled"
  http_put_response_hop_limit = 1
}
```

### Finding: Agent-operation role can spawn privileged worker tasks and pass worker roles

Severity: High

Affected files:

- `infra/terraform/agent-fargate.tf:171`
- `infra/terraform/agent-fargate.tf:183`
- `infra/terraform/agent-fargate.tf:196`

Evidence:

- The agent-operation role can call `ecs:RunTask` for the worker task definition.
- It can call `ecs:StopTask`, `ecs:ListTasks`, and `ecs:DescribeTasks` on `*`.
- It can `iam:PassRole` the worker task and execution roles.

Why it matters: compromise of the agent-operation EC2 host becomes the ability to launch workers that inherit the worker task role and its secrets access.

How it could realistically happen: a public SSH/SSM host compromise, malicious deployment artifact, or conductor code execution starts worker tasks and reads task-role secrets.

Concrete remediation: add `iam:PassedToService = ecs-tasks.amazonaws.com`, scope ECS monitor permissions where AWS supports it, tag-limit runnable task definitions, and separate a minimal scheduler role from an operator/debug role.

### Finding: Worker task role can read OpenAI, GitHub PAT, and debug SSH key secrets

Severity: High

Affected files:

- `infra/terraform/agent-fargate.tf:65`
- `infra/terraform/agent-fargate.tf:74`
- `conductor-and-worker/worker/go-worker/cmd/get-secrets/get-secrets.go:33`
- `conductor-and-worker/worker/docker-resources/inside/entrypoint.sh:8`
- `conductor-and-worker/worker/docker-resources/inside/entrypoint.sh:19`

Evidence:

- The task role allows `secretsmanager:GetSecretValue` for OpenAI key, GitHub PAT, and debug public SSH key secret names.
- The shipped `get-secrets` binary prints the resolved secret to stdout.
- The entrypoint exports OpenAI and GitHub token values into the process environment.

Why it matters: a worker is intentionally running arbitrary agent work. If the agent, a dependency, or a shell command is compromised, the task role and environment are enough to retrieve or exfiltrate high-value tokens.

How it could realistically happen: a prompt or downloaded dependency asks Codex to print environment variables, run `get-secrets`, or push code using the GitHub token.

Concrete remediation: split secrets by job and issue short-lived credentials. Avoid long-lived GitHub PATs; use GitHub App installation tokens or OIDC-mediated credentials scoped to the target repo. Do not grant debug SSH secrets to normal worker tasks.

### Finding: ECR tags are mutable and deploy paths use `latest`

Severity: Medium

Affected files:

- `infra/terraform/variables.tf:52`
- `infra/terraform/ecr.tf:1`
- `infra/terraform/ecr.tf:39`
- `infra/terraform/ecr.tf:77`
- `.github/workflows/conductor.yml:58`
- `.github/workflows/worker.yml:51`
- `.github/workflows/cpu-spin.yml:50`
- `infra/ad-hoc/debian-agent-operation/inside/deploy-conductor.sh:4`

Evidence:

- `ecr_image_tag_mutability` defaults to `MUTABLE`.
- CI pushes both SHA tags and `latest`.
- Fargate task definition and deploy scripts reference `latest` in several paths.

Why it matters: mutable tags weaken provenance. A later push can change what `latest` means without a Terraform diff or clear deployment record.

How it could realistically happen: a bad build or compromised CI credential overwrites `latest`; a host pulling `latest` runs unexpected code.

Concrete remediation: set ECR repositories to `IMMUTABLE`, deploy by image digest or immutable SHA tag, and record the digest in SSM/deploy logs.

### Finding: GitHub Actions role trust policy is unverified in repo

Severity: Medium

Affected files:

- `.github/workflows/conductor.yml:32`
- `.github/workflows/worker.yml:31`
- `.github/workflows/cpu-spin.yml:29`
- `infra/terraform/artifacts.tf:52`

Evidence:

- Workflows assume `github-actions-ecr-push`.
- Terraform attaches an S3 artifact write policy to a pre-existing role name.
- The OIDC provider and role trust policy are not defined in this repository.

Why it matters: the safety of CI AWS access depends heavily on the trust policy conditions for repo, branch, workflow, and audience.

How it could realistically happen: a broad trust policy allows another branch, fork-like context, or repository workflow to assume the deploy role.

Concrete remediation: manage the GitHub OIDC role in Terraform. Restrict `sub` to this repo and protected branch/environment. Keep `permissions` minimal and consider environment approval for deploy jobs.

## 4. Container/runtime findings

### Finding: Worker runs Codex as root with danger-full-access and no approvals

Severity: Critical

Affected files:

- `conductor-and-worker/worker/Dockerfile:23`
- `conductor-and-worker/worker/Dockerfile:25`
- `conductor-and-worker/worker/go-worker/worker.go:116`
- `conductor-and-worker/worker/go-worker/worker.go:121`
- `conductor-and-worker/worker/go-worker/worker.go:135`
- `conductor-and-worker/worker/go-worker/skip-conductor-debugging.go:24`

Evidence:

- The worker runtime stage has no `USER`, so it runs as root.
- PATH and Go paths point into `/root`.
- Codex is configured with auto-approval, no approval policy, danger-full-access sandbox, and danger-no-sandbox permissions.

Why it matters: the intended workload is an autonomous agent. If the prompt, fetched code, package install, or model behavior goes wrong, it has root-level control inside the container and access to task credentials/secrets.

How it could realistically happen: a malicious task file or repository instructs the agent to read `/root/.codex`, call `get-secrets`, or curl exfiltrate data.

Concrete remediation: run the worker as a non-root user, make the root filesystem read-only where possible, remove package managers and SSH from the runtime image, use a constrained workspace, and treat danger-full-access as acceptable only inside a strongly isolated disposable task with no broad secrets.

### Finding: Worker image includes SSH server and broad tooling

Severity: High

Affected files:

- `conductor-and-worker/worker/Dockerfile:33`
- `conductor-and-worker/worker/Dockerfile:41`
- `conductor-and-worker/worker/Dockerfile:49`
- `security-concerns.md:5`
- `infra/terraform/agent-fargate.tf:36`

Evidence:

- Worker image installs `openssh-server`, GitHub CLI, npm, Python, curl, tar, zip, unzip, and other admin tooling.
- Terraform opens port 22 to `var.ssh_allowed_cidr` for Fargate tasks.
- Existing notes describe debug SSH controlled by `DEBUG_SSH_ENABLED`.

Why it matters: the image contains many post-compromise tools, and debug SSH increases the remote access surface.

How it could realistically happen: debug SSH remains enabled on a task; a leaked debug key or allowed CIDR gives shell access to a secrets-bearing worker container.

Concrete remediation: remove SSH from normal worker images. Build a separate debug image/profile with no production secrets and short-lived ingress. Keep admin tooling out of the default runtime image.

### Finding: Conductor gRPC is published on the host and has no transport security

Severity: High

Affected files:

- `conductor-and-worker/conductor/Dockerfile:36`
- `infra/ad-hoc/debian-agent-operation/inside/docker-compose.conductor.yml:6`
- `conductor-and-worker/conductor/go-conductor/main.go:317`
- `conductor-and-worker/worker/go-worker/worker.go:152`

Evidence:

- Conductor container starts with `-addr=0.0.0.0:50055`.
- Docker Compose publishes `50055:50055`.
- Worker uses `grpc.WithTransportCredentials(insecure.NewCredentials())`.

Why it matters: any reachable client can talk to the conductor API. The only application-level gate is knowing or guessing a worker ID.

How it could realistically happen: the EC2 host firewall or cloud security group later opens 50055, or another local container/process connects to the published port and uploads files or spoofs worker state.

Concrete remediation: bind conductor to `127.0.0.1` unless remote workers truly require it, avoid publishing the port publicly, use mTLS or per-worker bearer tokens, and restrict host firewall/security group ingress for 50055.

### Finding: Local worker spawn uses host networking and local plaintext OpenAI key file

Severity: Medium

Affected files:

- `scripts_condtesting/spawn-docker-worker-testing:7`
- `scripts_condtesting/spawn-docker-worker-testing:23`
- `scripts_condtesting/spawn-docker-worker-testing-manual:12`

Evidence:

- The test spawn script reads `~/downloads/private/openai_api_key.txt` into `OPENAI_API_KEY`.
- Both worker test scripts run Docker with `--network host`.

Why it matters: host networking erases container network isolation, and passing long-lived API keys via env makes accidental exposure through process inspection/logging more likely.

How it could realistically happen: a local worker or dependency scans host-local services or prints its environment.

Concrete remediation: use Docker bridge networking and explicit port mappings. Load secrets from a local secret manager or short-lived env file with strict permissions, and avoid passing production keys to local test containers.

## 5. Secret handling findings

### Finding: Local Terraform state and ignored runtime files expose infrastructure metadata

Severity: Medium

Affected files:

- `infra/terraform/terraform.tfstate`
- `infra/terraform/terraform.tfstate.backup`
- `infra/terraform/terraform.tfvars`
- `scripts_aws1/ignore.current-test-fargate.txt`
- `.gitignore:9`

Evidence:

- Local state, backup state, tfvars, and plan artifacts exist and are ignored.
- State contains public IP/DNS outputs, ARNs, IAM policy JSON, queue URLs, ECR repository names, and role names.
- The ignored Fargate state file contains task and task-definition ARNs.

Why it matters: these are not access keys, but they make targeting much easier and can leak public endpoint history.

How it could realistically happen: a developer includes ignored files in a support bundle, paste, backup, or broader archive.

Concrete remediation: move Terraform state to an encrypted remote backend with locking. Keep local plan/state files out of shared archives. Consider `terraform output` hygiene and mark sensitive outputs where appropriate.

### Finding: Raw SQS message bodies are persisted in SQLite and local debug DBs

Severity: Medium

Affected files:

- `conductor-and-worker/conductor/go-conductor/db-sqlc/database.sql:1`
- `conductor-and-worker/conductor/go-conductor/db-sqlc/database.sql:10`
- `conductor-and-worker/conductor/go-conductor/db-sqlc/queries/sqs_alarm_mesages_queries.sql:17`
- `conductor-and-worker/conductor/go-conductor/DB_TESTDATA/data/*`

Evidence:

- Both scheduler tables store `raw_message_body TEXT NOT NULL`.
- Local ignored SQLite DBs exist under `DB_TESTDATA/data`.
- Redacted probes showed multiple DBs contain alarm rows with raw body lengths around 1.6-1.7 KB.

Why it matters: today CloudWatch alarm messages are low sensitivity, but future ticket/Jira/task messages may contain customer data, internal URLs, credentials, or incident details.

How it could realistically happen: an SQS event includes a ticket body or alert metadata with secrets, and the conductor stores it indefinitely in local or host-mounted SQLite.

Concrete remediation: store only normalized fields needed for scheduling, or encrypt/redact raw bodies. Add retention cleanup for scheduler rows and keep debug DBs out of backups.

### Finding: Secrets are moved through stdout/env

Severity: Medium

Affected files:

- `conductor-and-worker/worker/go-worker/cmd/get-secrets/get-secrets.go:33`
- `conductor-and-worker/worker/go-worker/cmd/get-secrets/get-secrets.go:49`
- `conductor-and-worker/worker/entrypoint.sh:6`
- `conductor-and-worker/worker/docker-resources/inside/entrypoint.sh:13`

Evidence:

- `get-secrets` prints resolved values to stdout.
- Entrypoints pipe `OPENAI_API_KEY` into `codex login`.
- The debug resources entrypoint exports `GITHUB_TOKEN`.

Why it matters: stdout and environment variables often end up in logs, shell history, crash dumps, or process listings.

How it could realistically happen: a failed command, `set -x`, debugging session, or agent command dumps env/stdout.

Concrete remediation: write secrets to least-privileged files with `0600` where unavoidable, avoid exporting tokens longer than needed, and prefer provider-native credential helpers or short-lived token exchange.

## 6. Unsafe scripting findings

### Finding: S3 deployment artifact extraction trusts tar contents

Severity: High

Affected files:

- `infra/ad-hoc/debian-agent-operation/inside/deploy-agent-operation.sh:33`
- `infra/ad-hoc/debian-agent-operation/inside/deploy-agent-operation.sh:34`
- `infra/ad-hoc/debian-agent-operation/inside/deploy-agent-operation.sh:41`
- `infra/ad-hoc/debian-agent-operation/inside/deploy-agent-operation.sh:43`

Evidence:

- The script downloads an S3 artifact URI supplied as an argument.
- It extracts `payload.tar.gz` directly with `tar -xzf` into a temp directory.
- It installs a binary and optionally a systemd service from extracted contents.

Why it matters: a malicious or corrupted archive can contain path traversal, symlinks, or a malicious service file. Because the script uses sudo for install/systemd steps, artifact compromise becomes host compromise.

How it could realistically happen: CI role, S3 object, or artifact URI is compromised and the host deploys an attacker-controlled tarball.

Concrete remediation: verify artifact digest/signature before extraction. Extract with safe tar options, reject absolute paths and `..`, inspect the file list before extraction, and never install service files from artifacts without validation.

### Finding: Conductor image build executes remote installer via curl pipe shell

Severity: Medium

Affected files:

- `conductor-and-worker/conductor/Dockerfile:20`
- `conductor-and-worker/conductor/Dockerfile:22`

Evidence:

- The Dockerfile runs `curl -sSf https://atlasgo.sh | sh` during image build.

Why it matters: this gives the remote installer full control of the build layer. A compromised installer, DNS/TLS interception, or upstream change alters the image without source changes.

How it could realistically happen: upstream installer changes behavior or is compromised; CI builds and deploys a malicious conductor image.

Concrete remediation: pin Atlas to a specific version, download a checksummed release artifact, verify signature/digest, and install from a pinned URL.

### Finding: Deployment scripts grant `admin` Docker group membership

Severity: Medium

Affected files:

- `infra/ad-hoc/debian-agent-operation/inside/install-docker.sh:19`
- `infra/ad-hoc/debian-cpu-spin/inside/install-docker.sh:19`

Evidence:

- Both Docker install scripts run `sudo usermod -aG docker admin`.

Why it matters: Docker group membership is effectively root on the host.

How it could realistically happen: compromise of the `admin` user or SSH key becomes root via Docker bind mounts or privileged containers.

Concrete remediation: treat `admin` as privileged, use SSM with audited commands, or run deployment through a narrowly scoped root-owned systemd unit rather than broad Docker group access.

### Finding: Hardcoded absolute executable paths create untracked execution dependencies

Severity: Medium

Affected files:

- `conductor-and-worker/conductor/go-conductor/janky-spawn-worker.go:10`
- `conductor-and-worker/conductor/go-conductor/janky-spawn-worker.go:16`
- `conductor-and-worker/conductor/go-conductor/serverside-worker.go:212`
- `scripts_dbtesting/run-on-generated-db:21`

Evidence:

- The conductor starts `/home/t/spawn-docker-worker-testing`, outside the repository tree.
- Other scripts reference absolute local paths.

Why it matters: production behavior can depend on mutable local files that are not reviewed, versioned, or deployed consistently.

How it could realistically happen: a stale or malicious local script at `/home/t/spawn-docker-worker-testing` is executed by the conductor.

Concrete remediation: keep executable dependencies in the repo or container image, verify checksums, and fail closed if an expected executable is missing or not owned/permissioned correctly.

## 7. File-transfer findings

### Finding: Uploaded worker zip can overwrite files inside extraction destination and is fully trusted after weak identity check

Severity: High

Affected files:

- `conductor-and-worker/conductor/go-conductor/file-transfer.go:57`
- `conductor-and-worker/conductor/go-conductor/file-transfer.go:64`
- `conductor-and-worker/conductor/go-conductor/file-transfer.go:99`
- `conductor-and-worker/sharedlib/zip.go:38`
- `conductor-and-worker/sharedlib/zip.go:56`

Evidence:

- The conductor receives worker-uploaded zip bytes and extracts them.
- Extraction removes the destination directory first.
- Extraction uses `unzip -o`.
- The RPC gate depends on caller-supplied worker ID and handshake state.

Why it matters: a spoofed or compromised worker can overwrite result files, use symlink/path tricks if `unzip` behavior changes or misses a case, or destroy previous result directories.

How it could realistically happen: a local process connects to the exposed conductor port with a known active worker ID and uploads a crafted archive.

Concrete remediation: authenticate workers, parse zip entries in Go, reject absolute paths/`..`/symlinks, enforce max file count and total size, and extract into a new unique directory before atomically publishing results.

### Finding: File transfer buffers entire archives in memory with no size limit

Severity: Medium

Affected files:

- `conductor-and-worker/sharedlib/file_transfer_chunks.go:73`
- `conductor-and-worker/sharedlib/file_transfer_chunks.go:76`
- `conductor-and-worker/sharedlib/file_transfer_chunks.go:95`
- `conductor-and-worker/proto/sharedproto.proto:14`

Evidence:

- `ReceiveFileTransferChunks` writes every chunk into an in-memory `bytes.Buffer`.
- The proto has no file size, total chunk count, checksum, or content length field.

Why it matters: a client can send a very large stream and exhaust conductor or worker memory.

How it could realistically happen: a bad worker process, bug, or local attacker streams chunks forever or sends a huge final archive.

Concrete remediation: enforce maximum bytes, maximum chunks, per-RPC deadlines, and checksums. Stream to a bounded temp file instead of memory.

## 8. Infrastructure exposure findings

### Finding: Conductor API exposure combines badly with weak worker authentication

Severity: High

Affected files:

- `infra/ad-hoc/debian-agent-operation/inside/docker-compose.conductor.yml:6`
- `conductor-and-worker/conductor/go-conductor/main.go:327`
- `conductor-and-worker/conductor/go-conductor/serverside-worker.go:100`
- `conductor-and-worker/conductor/go-conductor/serverside-worker.go:197`

Evidence:

- Compose publishes the conductor port on the host.
- The gRPC server registers services without interceptors/authentication.
- Worker registration only checks whether the supplied worker ID exists in memory.

Why it matters: if port 50055 is reachable, the conductor is an internal admin API without real authentication.

How it could realistically happen: a host firewall change, Docker networking assumption, or future security-group change exposes 50055 to a scanner or another compromised host.

Concrete remediation: keep gRPC private, enforce mTLS or signed worker tokens, and add server-side authorization per worker/task instance.

### Finding: Debug SSH is enabled in deployed conductor configuration

Severity: Medium

Affected files:

- `infra/ad-hoc/debian-agent-operation/inside/docker-compose.conductor.yml:13`
- `infra/ad-hoc/debian-agent-operation/inside/agent-operation.service:14`
- `security-concerns.md:5`

Evidence:

- Compose sets `DEBUG_SSH_ENABLED: "1"`.
- The systemd service sets `DEBUG_SSH_ENABLED=1`.
- The security note says this causes conductor-spawned Fargate tasks to receive debug SSH settings.

Why it matters: debug access often remains enabled longer than intended and expands the remote shell surface.

How it could realistically happen: a normal deploy starts conductor with debug SSH on; every spawned worker becomes SSH-capable if the corresponding runtime path honors the setting.

Concrete remediation: default debug SSH off. Make it an explicit break-glass deployment variable with short TTL and no access to production secrets.

## 9. Stability/cost-risk findings

### Finding: SQS events can spawn workers without a hard concurrency or cost limit

Severity: High

Affected files:

- `infra/terraform/cloudwatch.tf:22`
- `conductor-and-worker/conductor/go-conductor/poll.go:140`
- `conductor-and-worker/conductor/go-conductor/main.go:340`
- `conductor-and-worker/conductor/go-conductor/main.go:393`
- `conductor-and-worker/conductor/go-conductor/serverside-worker.go:181`

Evidence:

- SQS retention is 14 days.
- The poller continuously receives messages.
- Each accepted message path spawns a worker.
- The in-memory registry counts active workers but does not enforce a maximum.

Why it matters: a burst of alarms or poison-message loop can create many worker tasks or local containers, causing cost and resource exhaustion.

How it could realistically happen: CloudWatch alarm flaps repeatedly, a test script pushes messages, or unsupported messages repeatedly reappear after visibility timeout.

Concrete remediation: add a max active worker limit, backpressure, DLQ, idempotency keys, per-account rate limits, and budget alarms tied to ECS/Fargate spend.

### Finding: Unsupported SQS messages are retried indefinitely until retention expires

Severity: Medium

Affected files:

- `conductor-and-worker/conductor/go-conductor/use-sheduler.go:56`
- `conductor-and-worker/conductor/go-conductor/use-sheduler.go:65`
- `conductor-and-worker/conductor/go-conductor/main.go:379`
- `conductor-and-worker/conductor/go-conductor/main.go:381`
- `infra/terraform/cloudwatch.tf:24`

Evidence:

- Unsupported messages return an error.
- The main loop deletes SQS messages only after scheduler handling succeeds.
- SQS retention is 1,209,600 seconds.

Why it matters: one malformed message can generate repeated processing and logs for up to 14 days.

How it could realistically happen: a non-CloudWatch message enters the queue or a parser bug classifies a valid message as unknown.

Concrete remediation: attach a DLQ with low max receive count. Delete or quarantine unsupported messages after logging sanitized metadata.

### Finding: Scheduler queries scan all retained alarm rows

Severity: Medium

Affected files:

- `conductor-and-worker/conductor/go-conductor/db-sqlc/queries/sqs_alarm_mesages_queries.sql:46`
- `conductor-and-worker/conductor/go-conductor/go-db-scheduler/scheduler.go:314`
- `conductor-and-worker/conductor/go-conductor/go-db-scheduler/scheduler.go:324`

Evidence:

- `ListSQSAlarmMessages` returns all alarm rows without a `WHERE` or `LIMIT`.
- Scheduler sorts and processes the full set in memory.

Why it matters: scheduler performance degrades as the local DB grows, and raw message retention grows with it.

How it could realistically happen: long-running conductor receives months of alarm events and every new event causes a full-table scheduling pass.

Concrete remediation: add indexes, query only undecided/recent rows, archive or delete old rows, and enforce retention.

## 10. Recommended fixes

1. Remove public SSH and use SSM-only host access.
2. Require IMDSv2 on all EC2 instances.
3. Make ECR tags immutable and deploy by digest or SHA tag only.
4. Move Terraform state to an encrypted remote backend with locking.
5. Add real worker authentication: mTLS or per-worker signed tokens.
6. Keep conductor gRPC private and unpublish `50055` unless protected by firewall and authentication.
7. Split worker task roles and secrets by job. Remove GitHub PAT/debug SSH secret from ordinary worker tasks.
8. Run worker/conductor containers as non-root and remove SSH/package-manager tooling from production images.
9. Replace shelling out to `zip`/`unzip`/`tar` for untrusted archives with safe library-based extraction and explicit limits.
10. Add queue DLQ, worker concurrency limits, idempotency, and cost alarms.
11. Pin installer/dependency supply chain: Atlas download, GitHub Actions by SHA where appropriate, Docker base image digests for production, and deployment artifacts by digest/signature.

## 11. Highest-priority remediation steps

1. Disable debug/public access: remove SSH ingress, set `DEBUG_SSH_ENABLED=0`, and unpublish conductor `50055`.
2. Add worker authentication before trusting any gRPC request.
3. Reduce worker blast radius: non-root user, no Codex danger-full-access unless inside a much stronger sandbox, and no broad secrets in the worker role.
4. Require IMDSv2 and protect host IAM roles.
5. Add worker concurrency limits, DLQ, and budget alarms before running this against uncontrolled event sources.
6. Replace mutable `latest` deploys with digest-pinned releases.
