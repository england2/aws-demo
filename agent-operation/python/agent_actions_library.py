# - shared action utilities that are accessible to agents
# e.g "clone repo", "push to repo"(repo name), "create pr"(repo name),
#

import subprocess
from pathlib import Path


def run_command(
    command: list[str], cwd: Path | None = None
) -> subprocess.CompletedProcess:
    return subprocess.run(
        command,
        cwd=cwd,
        check=True,
        text=True,
    )
