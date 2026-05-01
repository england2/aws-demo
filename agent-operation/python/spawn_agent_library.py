from datetime import UTC, datetime
from pathlib import Path


AGENT_WORK_ROOT = Path("/tmp/agent-operation-runs")


def new_agent_dir(operation_name: str) -> Path:
    timestamp = datetime.now(UTC).strftime("%Y%m%d-%H%M%S-%f")
    agent_dir = AGENT_WORK_ROOT / f"{timestamp}-{operation_name}"
    agent_dir.mkdir(parents=True, exist_ok=False)
    return agent_dir
