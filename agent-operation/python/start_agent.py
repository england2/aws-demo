import os
import json
import shlex
import subprocess
import sys
from pathlib import Path


PYTHON_ROOT = Path(__file__).resolve().parent
OPERATIONS_ROOT = PYTHON_ROOT / "operations"
DEFAULT_TMUX_SESSION = os.environ.get("AGENT_OPERATION_TMUX_SESSION", "agent-operation")
ALERT_STAGING_ROOT = Path(os.environ.get("AGENT_OPERATION_ALERT_STAGING_ROOT", "/tmp/agent-operation-alerts"))


def discover_operation_paths() -> dict[str, Path]:
    operation_paths = {}

    if not OPERATIONS_ROOT.exists():
        return operation_paths

    for operation_dir in OPERATIONS_ROOT.iterdir():
        if not operation_dir.is_dir():
            continue

        operation_name = operation_dir.name
        operation_path = operation_dir / f"{operation_name}.py"

        if operation_path.exists():
            operation_paths[operation_name] = operation_path

    return operation_paths


def operation_window_name(operation_name: str) -> str:
    return f"agent-{operation_name}"[:20]


def stage_alert(operation_name: str, alert: dict) -> Path:
    ALERT_STAGING_ROOT.mkdir(parents=True, exist_ok=True)
    event_id = alert.get("id", "unknown-event")
    alert_file = ALERT_STAGING_ROOT / f"{event_id}-{operation_name}.json"
    alert_file.write_text(json.dumps(alert, indent=2, sort_keys=True))
    return alert_file


def command_for_operation(operation_path: Path, alert_file: Path | None = None) -> str:
    python_executable = Path(sys.executable)
    command_parts = [
        "cd",
        shlex.quote(str(PYTHON_ROOT)),
        "&&",
        "PYTHONPATH=" + shlex.quote(str(PYTHON_ROOT)),
        shlex.quote(str(python_executable)),
        shlex.quote(str(operation_path)),
    ]

    if alert_file is not None:
        command_parts.append(shlex.quote(str(alert_file)))

    return " ".join(command_parts)


def start_agent_operation(
    operation_name: str,
    alert: dict | None = None,
    tmux_session: str = DEFAULT_TMUX_SESSION,
) -> None:
    operation_path = discover_operation_paths().get(operation_name)

    if operation_path is None:
        raise ValueError(f"unknown operation_name={operation_name}")

    if not operation_path.exists():
        raise FileNotFoundError(operation_path)

    alert_file = stage_alert(operation_name, alert) if alert is not None else None

    subprocess.run(
        [
            "tmux",
            "new-window",
            "-t",
            tmux_session,
            "-n",
            operation_window_name(operation_name),
            command_for_operation(operation_path, alert_file),
        ],
        check=True,
    )


def main() -> None:
    if len(sys.argv) != 2:
        print("usage: python start-agent.py <operation-name>", file=sys.stderr)
        raise SystemExit(2)

    start_agent_operation(sys.argv[1])


if __name__ == "__main__":
    main()
