# - the 'main-function' that calls on the agent library to start a CPU investigation agent
# - we use many functions from
#
#
#
# for some very simple pseudocode (to give you an idea of what this might look like):
# create_agent_dir()
# clone_repo("aws1", "path-to-config-config")

import time


def main():
    print("Hello from investigate-cpu!")

    while True:
        time.sleep(60)


if __name__ == "__main__":
    main()
