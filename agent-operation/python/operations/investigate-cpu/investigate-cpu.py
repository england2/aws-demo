import time

from spawn_agent_library import new_agent_dir


def main():
    print("Hello from investigate-cpu!")
    agent_dir = new_agent_dir("investigate-cpu")
    print(f"created agent dir: {agent_dir}")

    while True:
        time.sleep(60)


if __name__ == "__main__":
    main()
