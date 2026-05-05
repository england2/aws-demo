#!/usr/bin/env python3
import argparse
import json
import os
import shlex
import shutil
import subprocess
import sys
import time
from dataclasses import dataclass
from typing import Any


DEFAULT_REGION = "us-west-2"
DEFAULT_CLUSTER = "ecs-cluster-agent-fargate"
DEFAULT_CONTAINER = "agent-fargate"
DEFAULT_REMOTE_SESSION = "agent-codex"
DEFAULT_LOCAL_SESSION = "local-tmux-watch"
DEFAULT_OPERATION_INSTANCE_NAME = "debian-agent-operation"
DEFAULT_OPERATION_REMOTE_SESSION = "agent-operation"
DEFAULT_OPERATION_USER = "admin"
DEFAULT_STARTED_BY = "agent-conductor"
DEFAULT_POLL_SECONDS = 5.0
WINDOW_TASK_ARN_OPTION = "@local-tmux-watch-task-arn"
WINDOW_ROLE_OPTION = "@local-tmux-watch-role"
WINDOW_ROLE_OPERATION = "agent-operation"


@dataclass(frozen=True)
class Config:
    region: str
    cluster: str
    container: str
    remote_session: str
    local_session: str
    operation_instance_name: str
    operation_remote_session: str
    operation_user: str
    started_by: str
    poll_seconds: float
    once: bool
    no_tmux_reexec: bool


@dataclass(frozen=True)
class Task:
    arn: str
    exec_enabled: bool
    has_container: bool
    exec_agent_running: bool


def env_value(name: str, default: str) -> str:
    value = os.environ.get(name)
    if value is None or value.strip() == "":
        return default
    return value


def env_region() -> str:
    return env_value(
        "LOCAL_TMUX_WATCH_REGION",
        env_value("AWS_REGION", env_value("AWS_DEFAULT_REGION", DEFAULT_REGION)),
    )


def env_poll_seconds() -> float:
    raw = env_value("LOCAL_TMUX_WATCH_POLL_SECONDS", str(DEFAULT_POLL_SECONDS))
    try:
        poll_seconds = float(raw)
    except ValueError:
        raise SystemExit(f"LOCAL_TMUX_WATCH_POLL_SECONDS must be a number, got {raw!r}")
    if poll_seconds <= 0:
        raise SystemExit("LOCAL_TMUX_WATCH_POLL_SECONDS must be greater than zero")
    return poll_seconds


def parse_args(argv: list[str]) -> Config:
    parser = argparse.ArgumentParser(
        description="Open local tmux windows attached to conductor-spawned Fargate agent tmux sessions.",
    )
    parser.add_argument("--region", default=env_region())
    parser.add_argument(
        "--cluster",
        default=env_value("LOCAL_TMUX_WATCH_CLUSTER", DEFAULT_CLUSTER),
    )
    parser.add_argument(
        "--container",
        default=env_value("LOCAL_TMUX_WATCH_CONTAINER", DEFAULT_CONTAINER),
    )
    parser.add_argument(
        "--remote-session",
        default=env_value("LOCAL_TMUX_WATCH_REMOTE_SESSION", DEFAULT_REMOTE_SESSION),
    )
    parser.add_argument(
        "--local-session",
        default=env_value("LOCAL_TMUX_WATCH_LOCAL_SESSION", DEFAULT_LOCAL_SESSION),
    )
    parser.add_argument(
        "--operation-instance-name",
        default=env_value("LOCAL_TMUX_WATCH_OPERATION_INSTANCE_NAME", DEFAULT_OPERATION_INSTANCE_NAME),
        help="EC2 Name tag for the agent-operation host.",
    )
    parser.add_argument(
        "--operation-remote-session",
        default=env_value("LOCAL_TMUX_WATCH_OPERATION_REMOTE_SESSION", DEFAULT_OPERATION_REMOTE_SESSION),
        help="Remote tmux session to attach on the agent-operation host.",
    )
    parser.add_argument(
        "--operation-user",
        default=env_value("LOCAL_TMUX_WATCH_OPERATION_USER", DEFAULT_OPERATION_USER),
        help="Remote user that owns the agent-operation tmux session.",
    )
    parser.add_argument(
        "--started-by",
        default=env_value("LOCAL_TMUX_WATCH_STARTED_BY", DEFAULT_STARTED_BY),
        help="ECS startedBy value to watch.",
    )
    parser.add_argument(
        "--poll",
        type=float,
        default=env_poll_seconds(),
        help="Polling interval in seconds.",
    )
    parser.add_argument(
        "--once",
        action="store_true",
        help="Poll once, create any missing windows, then exit.",
    )
    parser.add_argument(
        "--no-tmux-reexec",
        action="store_true",
        help="Run in the current terminal even when not already inside tmux.",
    )
    args = parser.parse_args(argv)

    if args.poll <= 0:
        parser.error("--poll must be greater than zero")

    return Config(
        region=args.region,
        cluster=args.cluster,
        container=args.container,
        remote_session=args.remote_session,
        local_session=args.local_session,
        operation_instance_name=args.operation_instance_name,
        operation_remote_session=args.operation_remote_session,
        operation_user=args.operation_user,
        started_by=args.started_by,
        poll_seconds=args.poll,
        once=args.once,
        no_tmux_reexec=args.no_tmux_reexec,
    )


def require_command(name: str) -> None:
    if shutil.which(name) is None:
        raise SystemExit(f"missing required command on PATH: {name}")


def run_json(args: list[str]) -> Any:
    try:
        result = subprocess.run(
            args,
            check=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
    except FileNotFoundError:
        raise SystemExit(f"missing required command on PATH: {args[0]}")
    except subprocess.CalledProcessError as err:
        stderr = err.stderr.strip()
        message = f"{shlex.join(args)} failed with exit code {err.returncode}"
        if stderr:
            message = f"{message}: {stderr}"
        raise RuntimeError(message)

    try:
        return json.loads(result.stdout)
    except json.JSONDecodeError as err:
        raise RuntimeError(f"{shlex.join(args)} returned invalid JSON: {err}") from err


def run_text(args: list[str], check: bool = True) -> subprocess.CompletedProcess[str]:
    try:
        return subprocess.run(
            args,
            check=check,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
    except FileNotFoundError:
        raise SystemExit(f"missing required command on PATH: {args[0]}")


def in_tmux() -> bool:
    return bool(os.environ.get("TMUX"))


def current_tmux_session() -> str | None:
    if not in_tmux():
        return None

    result = run_text(["tmux", "display-message", "-p", "#{session_name}"], check=False)
    if result.returncode != 0:
        return None
    return result.stdout.strip() or None


def current_tmux_window_id() -> str | None:
    if not in_tmux():
        return None

    result = run_text(["tmux", "display-message", "-p", "#{window_id}"], check=False)
    if result.returncode != 0:
        return None
    return result.stdout.strip() or None


def tmux_has_session(session_name: str) -> bool:
    return run_text(["tmux", "has-session", "-t", session_name], check=False).returncode == 0


def reexec_under_tmux(config: Config) -> None:
    if in_tmux() or config.no_tmux_reexec:
        return
    require_command("tmux")

    command = shlex.join([sys.executable, *sys.argv, "--no-tmux-reexec"])
    os.execvp(
        "tmux",
        ["tmux", "new-session", "-A", "-s", config.local_session, command],
    )


def ensure_local_tmux_session(config: Config) -> None:
    require_command("tmux")
    if tmux_has_session(config.local_session):
        ensure_current_watcher_window(config)
        return

    run_text(
        [
            "tmux",
            "new-session",
            "-d",
            "-s",
            config.local_session,
            "-n",
            "watch",
        ]
    )
    ensure_current_watcher_window(config)


def ensure_current_watcher_window(config: Config) -> None:
    if current_tmux_session() != config.local_session:
        return

    run_text(["tmux", "rename-window", "watch"], check=False)


def existing_role_windows(config: Config) -> dict[str, str]:
    result = run_text(
        [
            "tmux",
            "list-windows",
            "-t",
            config.local_session,
            "-F",
            f"#{{window_id}}\t#{{{WINDOW_ROLE_OPTION}}}",
        ]
    )
    windows: dict[str, str] = {}
    for line in result.stdout.splitlines():
        if not line.strip():
            continue
        window_id, _, role = line.partition("\t")
        if role:
            windows[role] = window_id
    return windows


def operation_attach_command(config: Config) -> str:
    tmux_attach = shlex.join(["tmux", "attach-session", "-t", config.operation_remote_session])
    sudo_attach = shlex.join(["sudo", "-iu", config.operation_user, "tmux", "attach-session", "-t", config.operation_remote_session])
    inner = f"{sudo_attach} || {tmux_attach} || exec /bin/bash"
    return f"/bin/bash -lc {shlex.quote(inner)}"


def operation_window_command(config: Config) -> str:
    describe_instances = shlex.join(
        [
            "aws",
            "ec2",
            "describe-instances",
            "--region",
            config.region,
            "--filters",
            f"Name=tag:Name,Values={config.operation_instance_name}",
            "Name=instance-state-name,Values=running",
            "--query",
            "Reservations[].Instances[].InstanceId",
            "--output",
            "text",
        ]
    )
    start_session = (
        "aws ssm start-session"
        f" --region {shlex.quote(config.region)}"
        ' --target "${target}"'
        " --document-name AWS-StartInteractiveCommand"
        f" --parameters {shlex.quote(json.dumps({'command': [operation_attach_command(config)]}))}"
    )
    script = "\n".join(
        [
            f"printf '%s\\n' {shlex.quote('Locating ' + config.operation_instance_name)}",
            f"targets=$({describe_instances})",
            'target=$(printf "%s\\n" "${targets}" | awk \'NF { print $1; exit }\')',
            'if [ -z "${target}" ]; then',
            f"  printf '%s\\n' {shlex.quote('No running EC2 instance found for Name=' + config.operation_instance_name)} >&2",
            '  exec "${SHELL:-/bin/bash}" -l',
            "fi",
            'printf "Connecting to %s\\n" "${target}"',
            start_session,
            "status=$?",
            "printf '\\n%s\\n' \"agent-operation SSM session ended with exit code ${status}.\"",
            'exec "${SHELL:-/bin/bash}" -l',
        ]
    )
    return shlex.join(["/bin/bash", "-lc", script])


def ensure_operation_window(config: Config) -> None:
    if WINDOW_ROLE_OPERATION in existing_role_windows(config):
        return

    target = f"{config.local_session}:"
    new_window_args = [
        "tmux",
        "new-window",
        "-d",
        "-P",
        "-F",
        "#{window_id}",
        "-t",
        target,
        "-n",
        "agent-operation",
        operation_window_command(config),
    ]
    if current_tmux_session() == config.local_session:
        target = current_tmux_window_id() or target
        new_window_args[new_window_args.index("-t") + 1] = target
        new_window_args.insert(2, "-b")

    window_id = run_text(new_window_args).stdout.strip()
    run_text(["tmux", "set-option", "-w", "-t", window_id, WINDOW_ROLE_OPTION, WINDOW_ROLE_OPERATION])


def task_id(task_arn: str) -> str:
    return task_arn.rsplit("/", 1)[-1]


def window_name(task_arn: str) -> str:
    return "agent-" + task_id(task_arn)[:12]


def existing_task_windows(config: Config) -> dict[str, str]:
    result = run_text(
        [
            "tmux",
            "list-windows",
            "-t",
            config.local_session,
            "-F",
            f"#{{window_id}}\t#{{{WINDOW_TASK_ARN_OPTION}}}",
        ]
    )
    windows: dict[str, str] = {}
    for line in result.stdout.splitlines():
        if not line.strip():
            continue
        window_id, _, task_arn = line.partition("\t")
        if task_arn:
            windows[task_arn] = window_id
    return windows


def list_running_task_arns(config: Config) -> list[str]:
    payload = run_json(
        [
            "aws",
            "ecs",
            "list-tasks",
            "--region",
            config.region,
            "--cluster",
            config.cluster,
            "--desired-status",
            "RUNNING",
            "--started-by",
            config.started_by,
            "--output",
            "json",
        ]
    )
    return payload.get("taskArns", [])


def chunks(values: list[str], size: int) -> list[list[str]]:
    return [values[index : index + size] for index in range(0, len(values), size)]


def describe_tasks(config: Config, task_arns: list[str]) -> list[Task]:
    tasks: list[Task] = []
    for batch in chunks(task_arns, 100):
        payload = run_json(
            [
                "aws",
                "ecs",
                "describe-tasks",
                "--region",
                config.region,
                "--cluster",
                config.cluster,
                "--tasks",
                *batch,
                "--output",
                "json",
            ]
        )
        for failure in payload.get("failures", []):
            arn = failure.get("arn", "")
            reason = failure.get("reason", "")
            detail = failure.get("detail", "")
            print(f"warn: describe-tasks failure arn={arn} reason={reason} detail={detail}", file=sys.stderr)
        for task in payload.get("tasks", []):
            arn = task.get("taskArn", "")
            if not arn:
                continue
            containers = task.get("containers", [])
            matching_containers = [container for container in containers if container.get("name") == config.container]
            tasks.append(
                Task(
                    arn=arn,
                    exec_enabled=bool(task.get("enableExecuteCommand")),
                    has_container=bool(matching_containers),
                    exec_agent_running=any(exec_agent_running(container) for container in matching_containers),
                )
            )
    return tasks


def exec_agent_running(container: dict[str, Any]) -> bool:
    for managed_agent in container.get("managedAgents", []):
        if managed_agent.get("name") == "ExecuteCommandAgent" and managed_agent.get("lastStatus") == "RUNNING":
            return True
    return False


def remote_attach_command(config: Config) -> str:
    inner = f"tmux attach-session -t {shlex.quote(config.remote_session)} || exec /bin/bash"
    return f"/bin/bash -lc {shlex.quote(inner)}"


def local_window_command(config: Config, task_arn: str) -> str:
    aws_command = shlex.join(
        [
            "aws",
            "ecs",
            "execute-command",
            "--interactive",
            "--region",
            config.region,
            "--cluster",
            config.cluster,
            "--task",
            task_arn,
            "--container",
            config.container,
            "--command",
            remote_attach_command(config),
        ]
    )
    script = "\n".join(
        [
            f"printf '%s\\n' {shlex.quote('Connecting to ' + task_arn)}",
            aws_command,
            "status=$?",
            "printf '\\n%s\\n' \"Remote tmux connection ended with exit code ${status}.\"",
            "printf '%s\\n' 'This local window is intentionally left open.'",
            'exec "${SHELL:-/bin/bash}" -l',
        ]
    )
    return shlex.join(["/bin/bash", "-lc", script])


def create_task_window(config: Config, task: Task) -> None:
    window_id = run_text(
        [
            "tmux",
            "new-window",
            "-d",
            "-P",
            "-F",
            "#{window_id}",
            "-t",
            f"{config.local_session}:",
            "-n",
            window_name(task.arn),
            local_window_command(config, task.arn),
        ]
    ).stdout.strip()

    run_text(["tmux", "set-option", "-w", "-t", window_id, WINDOW_TASK_ARN_OPTION, task.arn])
    print(f"opened {window_name(task.arn)} for {task.arn}", flush=True)


def poll_once(config: Config, warned_no_exec: set[str], warned_missing_container: set[str]) -> None:
    task_arns = list_running_task_arns(config)
    if not task_arns:
        return

    existing_windows = existing_task_windows(config)
    for task in describe_tasks(config, task_arns):
        if not task.has_container:
            if task.arn not in warned_missing_container:
                print(
                    f"warn: task {task.arn} does not contain container {config.container!r}; skipping",
                    file=sys.stderr,
                )
                warned_missing_container.add(task.arn)
            continue
        if not task.exec_enabled:
            if task.arn not in warned_no_exec:
                print(f"warn: task {task.arn} does not have ECS Exec enabled; skipping", file=sys.stderr)
                warned_no_exec.add(task.arn)
            continue
        if not task.exec_agent_running:
            print(f"waiting: task {task.arn} ECS Exec agent is not running yet", flush=True)
            continue
        if task.arn in existing_windows:
            continue
        create_task_window(config, task)


def watch(config: Config) -> None:
    require_command("aws")
    ensure_local_tmux_session(config)
    ensure_operation_window(config)

    warned_no_exec: set[str] = set()
    warned_missing_container: set[str] = set()
    print(
        "watching "
        f"cluster={config.cluster} region={config.region} started_by={config.started_by} "
        f"local_session={config.local_session} remote_session={config.remote_session}",
        flush=True,
    )

    while True:
        try:
            poll_once(config, warned_no_exec, warned_missing_container)
        except RuntimeError as err:
            print(f"warn: {err}", file=sys.stderr, flush=True)

        if config.once:
            return
        time.sleep(config.poll_seconds)


def main() -> None:
    config = parse_args(sys.argv[1:])
    reexec_under_tmux(config)
    watch(config)


if __name__ == "__main__":
    main()
