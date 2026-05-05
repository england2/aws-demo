# SHH switches for scripts/local-tmux-watch/ 

The python program `scripts/local-tmux-watch/` allows a user to wait for new agent-fargate tasks and watch their codex output in real time through a tmux session in an SSH-connected agent-fargate task.

Whether the SSH server starts on the agent-fargate tasks is controlled by `DEBUG_SSH_ENABLED` on the agent-conductor process. When enabled, conductor-spawned Fargate tasks receive `DEBUG_SSH_ENABLED=true` and `DEBUG_SSH_PUBLIC_KEY_SECRET_NAME=debug_public_ssh_key`.

The Fargate image starts `sshd` from `agent-conductor/agent-fargate/container/inside-container/bootstrap/entrypoint.sh`. The authorized key is read from Secrets Manager secret `debug_public_ssh_key`.

Network exposure is controlled in Terraform by `agent-fargate-sg` port 22 ingress, restricted to `var.ssh_allowed_cidr`.

Disable this by unsetting `DEBUG_SSH_ENABLED` before starting agent-conductor and removing/closing port 22 ingress when no longer needed.
