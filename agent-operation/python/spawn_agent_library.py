import shutil
from datetime import UTC, datetime
from pathlib import Path


PYTHON_ROOT = Path(__file__).resolve().parent
OPERATIONS_ROOT = PYTHON_ROOT / "operations"
AGENT_WORK_ROOT = Path("/tmp/agent-operation-runs")


def copy_agent_context(operation_name: str, agent_dir: Path) -> Path:
    source_context_dir = OPERATIONS_ROOT / operation_name / "agent-context"
    target_context_dir = agent_dir / "agent-context"

    if not source_context_dir.exists():
        raise FileNotFoundError(source_context_dir)

    shutil.copytree(source_context_dir, target_context_dir)
    return target_context_dir


def new_agent_dir(operation_name: str) -> Path:
    timestamp = datetime.now(UTC).strftime("%Y%m%d-%H%M%S-%f")
    agent_dir = AGENT_WORK_ROOT / f"{timestamp}-{operation_name}"
    agent_dir.mkdir(parents=True, exist_ok=False)
    copy_agent_context(operation_name, agent_dir)
    return agent_dir
