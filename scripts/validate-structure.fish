#!/usr/bin/env fish
#
# this is a developement helper file to ensure that program structure is consistent

set script_dir (cd (dirname (status --current-filename)); pwd)
set repo_root (dirname $script_dir)
set operations_dir "$repo_root/agent-operation/python/operations"

function print_color
    set color $argv[1]
    set message $argv[2]

    set_color $color
    echo $message
    set_color normal
end

# rule 1:
# every subdir in agent-operation/python/operations must have:
# - a .py file with the same name as the sub dirt
# - this subdir must have a directory named agent-context
# - agent-context must have an AGENTS.md file
# =================================================================================

if not test -d "$operations_dir"
    print_color red "agent-operation/python/operations has invalid strucutre."
    exit 1
end

for operation_dir in "$operations_dir"/*
    if not test -d "$operation_dir"
        continue
    end

    set operation_name (basename "$operation_dir")
    set operation_file "$operation_dir/$operation_name.py"
    set agent_context_dir "$operation_dir/agent-context"
    set agents_file "$agent_context_dir/AGENTS.md"

    if not test -f "$operation_file"
        print_color red "dir agent-operation/python/operations/$operation_name/ has invalid strucutre."
        exit 1
    end

    if not test -d "$agent_context_dir"
        print_color red "dir agent-operation/python/operations/$operation_name/ has invalid strucutre."
        exit 1
    end

    if not test -f "$agents_file"
        print_color red "dir agent-operation/python/operations/$operation_name/ has invalid strucutre."
        exit 1
    end
end

print_color green "agent-operation strucutre is good."
