#!/usr/bin/env bash
set -euo pipefail

readonly PROGRAM_NAME="$(basename "$0")"

name="${USER:-friend}"
use_color=true

usage() {
  cat <<USAGE
Usage: $PROGRAM_NAME [--name NAME] [--no-color]

Print a tiny randomized adventure.

Options:
  --name NAME   Name to include in the adventure.
  --no-color    Disable ANSI color output.
  -h, --help    Show this help text.
USAGE
}

fail() {
  printf 'error: %s\n' "$1" >&2
  printf 'Run "%s --help" for usage.\n' "$PROGRAM_NAME" >&2
  exit 1
}

parse_args() {
  while (($# > 0)); do
    case "$1" in
      --name)
        (($# >= 2)) || fail "--name requires a value"
        name="$2"
        shift 2
        ;;
      --no-color)
        use_color=false
        shift
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        fail "unknown argument: $1"
        ;;
    esac
  done

  [[ -n "$name" ]] || fail "name cannot be empty"
}

color() {
  local code="$1"
  local text="$2"

  if [[ "$use_color" == true ]]; then
    printf '\033[%sm%s\033[0m' "$code" "$text"
  else
    printf '%s' "$text"
  fi
}

pick_one() {
  local -n values=$1
  printf '%s' "${values[RANDOM % ${#values[@]}]}"
}

print_banner() {
  color '1;36' 'BASH FUN ADVENTURE'
  printf '\n'
  printf '       /\\\n'
  printf '      /  \\\n'
  printf '     /____\\\n'
  printf '     |    |\n'
  printf '     |_||_|\n'
  printf '\n'
}

main() {
  parse_args "$@"

  local places=(
    'a moonlit terminal'
    'the shell prompt arcade'
    'a secret server room'
    'the command-line carnival'
    'a cozy text-mode spaceship'
  )
  local companions=(
    'a pocket-sized debugger'
    'a very patient linter'
    'an overcaffeinated build script'
    'a tiny pager window'
    'a bash array with opinions'
  )
  local quests=(
    'teaches a loop to dance'
    'turns exit code 0 into confetti'
    'rescues a missing semicolon'
    'formats the clouds with printf'
    'deploys a joke to localhost'
  )

  local place
  local companion
  local quest
  place="$(pick_one places)"
  companion="$(pick_one companions)"
  quest="$(pick_one quests)"

  print_banner
  color '1;33' "$name"
  printf ' enters %s with %s and %s.\n' "$place" "$companion" "$quest"
  color '1;32' 'Result:'
  printf ' fun completed successfully.\n'
}

main "$@"
