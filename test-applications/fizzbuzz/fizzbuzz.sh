#!/usr/bin/env bash
set -euo pipefail

limit="${1:-100}"

if ! [[ "$limit" =~ ^[1-9][0-9]*$ ]]; then
    echo "Usage: $0 [positive-integer-limit]" >&2
    exit 1
fi

for ((number = 1; number <= limit; number++)); do
    output=""

    if ((number % 3 == 0)); then
        output="Fizz"
    fi

    if ((number % 5 == 0)); then
        output="${output}Buzz"
    fi

    if [[ -z "$output" ]]; then
        output="$number"
    fi

    echo "$output"
done
