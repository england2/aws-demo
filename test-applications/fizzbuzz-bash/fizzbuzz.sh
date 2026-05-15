#!/usr/bin/env bash

set -euo pipefail

usage() {
  echo "Usage: $0 [limit]" >&2
}

is_positive_integer() {
  [[ "$1" =~ ^[1-9][0-9]*$ ]]
}

print_fizzbuzz() {
  local limit="$1"

  for ((number = 1; number <= limit; number++)); do
    if ((number % 15 == 0)); then
      echo "FizzBuzz"
    elif ((number % 3 == 0)); then
      echo "Fizz"
    elif ((number % 5 == 0)); then
      echo "Buzz"
    else
      echo "$number"
    fi
  done
}

main() {
  local limit="${1:-100}"

  if (($# > 1)); then
    usage
    return 1
  fi

  if ! is_positive_integer "$limit"; then
    echo "limit must be a positive integer" >&2
    usage
    return 1
  fi

  print_fizzbuzz "$limit"
}

main "$@"
