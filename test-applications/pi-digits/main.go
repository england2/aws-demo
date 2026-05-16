package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
)

var errMissingDigits = errors.New("missing digit count")

func main() {
	digits, err := parseDigits(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	pi, err := PiDecimal(digits)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	fmt.Println(pi)
}

func parseDigits(args []string) (int, error) {
	flags := flag.NewFlagSet("pi-digits", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	digitsFlag := flags.Int("digits", -1, "number of digits to print after the decimal point")

	if err := flags.Parse(args); err != nil {
		return 0, err
	}

	digitsFlagWasSet := false
	flags.Visit(func(flag *flag.Flag) {
		if flag.Name == "digits" {
			digitsFlagWasSet = true
		}
	})

	digits := *digitsFlag
	switch flags.NArg() {
	case 0:
		if !digitsFlagWasSet {
			return 0, errMissingDigits
		}
	case 1:
		if digitsFlagWasSet {
			return 0, fmt.Errorf("use either -digits or a positional digit count, not both")
		}

		parsed, err := strconv.Atoi(flags.Arg(0))
		if err != nil {
			return 0, fmt.Errorf("invalid digit count %q: %w", flags.Arg(0), err)
		}

		digits = parsed
	default:
		return 0, fmt.Errorf("expected one digit count, got %d arguments", flags.NArg())
	}

	if digits < 0 {
		return 0, errNegativeDigits
	}

	return digits, nil
}
