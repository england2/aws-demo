#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
fizzbuzz="${script_dir}/fizzbuzz.sh"

fail() {
  echo "FAIL: $1" >&2
  exit 1
}

assert_output() {
  local description="$1"
  local expected="$2"
  shift 2

  local actual
  actual="$("$fizzbuzz" "$@")"

  if [[ "$actual" != "$expected" ]]; then
    echo "Expected:" >&2
    printf '%s\n' "$expected" >&2
    echo "Actual:" >&2
    printf '%s\n' "$actual" >&2
    fail "$description"
  fi
}

assert_failure() {
  local description="$1"
  shift

  if "$fizzbuzz" "$@" >/dev/null 2>&1; then
    fail "$description"
  fi
}

assert_default_limit() {
  local line_count
  line_count="$("$fizzbuzz" | wc -l)"

  local last_line
  last_line="$("$fizzbuzz" | tail -n 1)"

  if [[ "$line_count" != "100" || "$last_line" != "Buzz" ]]; then
    fail "defaults to 100 values"
  fi
}

expected_15="$(cat <<'OUTPUT'
1
2
Fizz
4
Buzz
Fizz
7
8
Fizz
Buzz
11
Fizz
13
14
FizzBuzz
OUTPUT
)"

assert_output "prints FizzBuzz values through 15" "$expected_15" 15
assert_default_limit
assert_failure "rejects non-integer limit" abc
assert_failure "rejects zero limit" 0
assert_failure "rejects too many arguments" 1 2

echo "All fizzbuzz tests passed."
