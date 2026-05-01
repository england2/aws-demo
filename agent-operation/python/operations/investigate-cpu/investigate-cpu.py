import sys
import time
from pathlib import Path

from spawn_agent_library import new_agent_dir


def main():
    print("Hello from investigate-cpu!")
    alert_file = Path(sys.argv[1]) if len(sys.argv) > 1 else None
    agent_dir = new_agent_dir("investigate-cpu", alert_file=alert_file)
    print(f"created agent dir: {agent_dir}")

    while True:
        time.sleep(60)


if __name__ == "__main__":
    main()
