package main

import (
	"errors"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
)

const (
	digitsPerTerm = 14
	guardDigits   = 10
	maxDigits     = 1_000_000
)

var (
	chudnovskyMultiplier = big.NewInt(426880)
	chudnovskyConstant   = big.NewInt(640320 * 640320 * 640320 / 24)
)

type binarySplitResult struct {
	p *big.Int
	q *big.Int
	t *big.Int
}

func main() {
	digits, err := parseDigits(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		fmt.Fprintf(os.Stderr, "usage: %s <digits-after-decimal>\n", os.Args[0])
		os.Exit(2)
	}

	fmt.Println(computePi(digits))
}

func parseDigits(args []string) (int, error) {
	if len(args) != 1 {
		return 0, errors.New("expected exactly one digit count")
	}

	digits, err := strconv.Atoi(args[0])
	if err != nil {
		return 0, fmt.Errorf("invalid digit count %q", args[0])
	}

	if digits < 0 {
		return 0, errors.New("digit count must be non-negative")
	}

	if digits > maxDigits {
		return 0, fmt.Errorf("digit count must be at most %d", maxDigits)
	}

	return digits, nil
}

func computePi(digitsAfterDecimal int) string {
	precision := digitsAfterDecimal + guardDigits
	scale := pow10(precision)
	terms := precision/digitsPerTerm + 1

	sum := binarySplit(0, terms)
	sqrt := new(big.Int).Sqrt(new(big.Int).Mul(big.NewInt(10005), new(big.Int).Mul(scale, scale)))

	numerator := new(big.Int).Mul(sum.q, chudnovskyMultiplier)
	numerator.Mul(numerator, sqrt)
	scaledPi := numerator.Quo(numerator, sum.t)
	scaledPi.Quo(scaledPi, pow10(guardDigits))

	return formatFixedDecimal(scaledPi, digitsAfterDecimal)
}

func binarySplit(start int, end int) binarySplitResult {
	if end-start == 1 {
		return binarySplitTerm(start)
	}

	mid := (start + end) / 2
	left := binarySplit(start, mid)
	right := binarySplit(mid, end)

	p := new(big.Int).Mul(left.p, right.p)
	q := new(big.Int).Mul(left.q, right.q)
	t := new(big.Int).Add(
		new(big.Int).Mul(left.t, right.q),
		new(big.Int).Mul(left.p, right.t),
	)

	return binarySplitResult{p: p, q: q, t: t}
}

func binarySplitTerm(k int) binarySplitResult {
	if k == 0 {
		return binarySplitResult{p: big.NewInt(1), q: big.NewInt(1), t: big.NewInt(13591409)}
	}

	kBig := big.NewInt(int64(k))

	p := big.NewInt(6*int64(k) - 5)
	p.Mul(p, big.NewInt(2*int64(k)-1))
	p.Mul(p, big.NewInt(6*int64(k)-1))

	q := new(big.Int).Mul(kBig, kBig)
	q.Mul(q, kBig)
	q.Mul(q, chudnovskyConstant)

	t := new(big.Int).Mul(p, big.NewInt(545140134*int64(k)+13591409))
	if k%2 == 1 {
		t.Neg(t)
	}

	return binarySplitResult{p: p, q: q, t: t}
}

func formatFixedDecimal(value *big.Int, digitsAfterDecimal int) string {
	if digitsAfterDecimal == 0 {
		return value.String()
	}

	scale := pow10(digitsAfterDecimal)
	integerPart := new(big.Int).Quo(value, scale)
	fractionalPart := new(big.Int).Mod(value, scale).String()

	if missingDigits := digitsAfterDecimal - len(fractionalPart); missingDigits > 0 {
		fractionalPart = strings.Repeat("0", missingDigits) + fractionalPart
	}

	return integerPart.String() + "." + fractionalPart
}

func pow10(exponent int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil)
}
