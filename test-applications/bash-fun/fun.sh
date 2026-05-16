#!/usr/bin/env bash
set -euo pipefail

colors_enabled=true

usage() {
  cat <<'USAGE'
Usage: bash-fun [--no-color] [--help]

Print a tiny randomized terminal celebration.
USAGE
}

color() {
  local code="$1"

  if [[ "${colors_enabled}" == true && -t 1 ]]; then
    printf '\033[%sm' "${code}"
  fi
}

reset_color() {
  color "0"
}

pick_one() {
  local -n choices="$1"
  local index=$((RANDOM % ${#choices[@]}))

  printf '%s\n' "${choices[$index]}"
}

print_banner() {
  color "1;36"
  printf 'Bash Fun Machine\n'
  reset_color
  printf '================\n\n'
}

print_countdown() {
  local number

  for number in 3 2 1; do
    color "33"
    printf '%s... ' "${number}"
    reset_color
    sleep 0.2
  done

  color "1;32"
  printf 'launch!\n\n'
  reset_color
}

print_fireworks() {
  local bursts=(
    '      .       *       .'
    '   *     \ | /     *'
    ' .   --  BOOM  --   .'
    '   *     / | \     *'
    '      .       *       .'
  )
  local line

  for line in "${bursts[@]}"; do
    color "$((31 + RANDOM % 6))"
    printf '%s\n' "${line}"
    reset_color
    sleep 0.08
  done

  printf '\n'
}

print_fortune() {
  local fortunes=(
    "Today's bug becomes tomorrow's test."
    "A well-named variable is a tiny act of kindness."
    "The fastest build is the one you do not need to run twice."
    "Logs are breadcrumbs for future you."
    "Ship small, learn quickly, keep the shell happy."
  )

  color "35"
  printf 'Fortune: '
  reset_color
  pick_one fortunes
}

main() {
  while (($# > 0)); do
    case "$1" in
      --no-color)
        colors_enabled=false
        ;;
      --help|-h)
        usage
        return 0
        ;;
      *)
        printf 'Unknown option: %s\n\n' "$1" >&2
        usage >&2
        return 1
        ;;
    esac

    shift
  done

  print_banner
  print_countdown
  print_fireworks
  print_fortune
}

main "$@"
