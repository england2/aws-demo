# shared utilities related to spinning up an agent operation.
# we will use them in the main task file of an operation, e.g "investigate-cpu.py"
# these are all imperative/deterministic functions that will help us prepare the agent to work, and ultimate spawn one to carry out a task
#
# we definetely want these functions:
# - clone repo (full repository name)
# -   i think this by default will just clone to /tmp/date +"%Y%m%d-%H%M%S-%N")
# -   in fact, let's just have a function here that ALWWAYS creates a /tmp/date directory
# -     it can be something like new_agent_dir..
# -     even though agents have bubblewrap, actually cloning onto the filesystem will allow for deterministic git pushes that are called from an authenticated program.
#
# - spawn agent()
#   - I might actually run investigate-cpu.py in a forked process that attaches to the running tmux session in a new pane! This way, I can ssh into the server and watch the agent work, which is ideal. however, this might be a bit strange! however, it's good because then our python program is not blocked, and we can respond to multiple alerts at once
