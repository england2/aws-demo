package main

import (
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"strconv"
)

const guardDigits = 20

var errInvalidDigitCount = errors.New("digit count must be a positive integer")

func main() {
	os.Exit(run(os.Args, os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) != 2 {
		fmt.Fprintf(stderr, "Usage: %s <digits>\n", programName(args))
		return 1
	}

	digits, err := strconv.Atoi(args[1])
	if err != nil || digits <= 0 {
		fmt.Fprintln(stderr, errInvalidDigitCount)
		return 1
	}

	pi, err := piDigits(digits)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	fmt.Fprintln(stdout, pi)
	return 0
}

func programName(args []string) string {
	if len(args) == 0 {
		return "pi-digits"
	}

	return args[0]
}

func piDigits(totalDigits int) (string, error) {
	if totalDigits <= 0 {
		return "", errInvalidDigitCount
	}

	// Machin's formula converges quickly and uses exact fixed-point integer math:
	// pi = 16*atan(1/5) - 4*atan(1/239).
	decimalDigits := totalDigits - 1
	scale := powerOfTen(decimalDigits + guardDigits)

	pi := new(big.Int).Mul(big.NewInt(16), arctanReciprocal(5, scale))
	pi.Sub(pi, new(big.Int).Mul(big.NewInt(4), arctanReciprocal(239, scale)))
	pi.Quo(pi, powerOfTen(guardDigits))

	digits := pi.String()
	for len(digits) < totalDigits {
		digits = "0" + digits
	}

	if totalDigits == 1 {
		return digits[:1], nil
	}

	return digits[:1] + "." + digits[1:totalDigits], nil
}

func arctanReciprocal(x int64, scale *big.Int) *big.Int {
	xBig := big.NewInt(x)
	xSquared := new(big.Int).Mul(xBig, xBig)
	term := new(big.Int).Quo(new(big.Int).Set(scale), xBig)
	sum := new(big.Int).Set(term)

	subtractTerm := true
	for k := int64(1); ; k++ {
		term.Quo(term, xSquared)
		addend := new(big.Int).Quo(new(big.Int).Set(term), big.NewInt(2*k+1))
		if addend.Sign() == 0 {
			break
		}

		if subtractTerm {
			sum.Sub(sum, addend)
		} else {
			sum.Add(sum, addend)
		}
		subtractTerm = !subtractTerm
	}

	return sum
}

func powerOfTen(exponent int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil)
}
