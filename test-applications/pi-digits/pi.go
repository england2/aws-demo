package main

import (
	"errors"
	"math/big"
	"strings"
)

var errNegativeDigits = errors.New("digits must be non-negative")

type spigot struct {
	q *big.Int
	r *big.Int
	t *big.Int
	k *big.Int
	n *big.Int
	l *big.Int
}

func newSpigot() *spigot {
	return &spigot{
		q: big.NewInt(1),
		r: big.NewInt(0),
		t: big.NewInt(1),
		k: big.NewInt(1),
		n: big.NewInt(3),
		l: big.NewInt(3),
	}
}

func (s *spigot) nextDigit() int {
	for {
		if s.hasSafeDigit() {
			return s.produceDigit()
		}

		s.consumeTerm()
	}
}

func (s *spigot) hasSafeDigit() bool {
	left := new(big.Int).Mul(s.q, big.NewInt(4))
	left.Add(left, s.r)
	left.Sub(left, s.t)

	right := new(big.Int).Mul(s.n, s.t)

	return left.Cmp(right) < 0
}

func (s *spigot) produceDigit() int {
	digit := int(s.n.Int64())

	newR := new(big.Int).Mul(s.n, s.t)
	newR.Sub(s.r, newR)
	newR.Mul(newR, big.NewInt(10))

	newN := new(big.Int).Mul(s.q, big.NewInt(3))
	newN.Add(newN, s.r)
	newN.Mul(newN, big.NewInt(10))
	newN.Quo(newN, s.t)
	newN.Sub(newN, new(big.Int).Mul(s.n, big.NewInt(10)))

	s.q.Mul(s.q, big.NewInt(10))
	s.r = newR
	s.n = newN

	return digit
}

func (s *spigot) consumeTerm() {
	newR := new(big.Int).Mul(s.q, big.NewInt(2))
	newR.Add(newR, s.r)
	newR.Mul(newR, s.l)

	sevenKPlusTwo := new(big.Int).Mul(s.k, big.NewInt(7))
	sevenKPlusTwo.Add(sevenKPlusTwo, big.NewInt(2))

	newN := new(big.Int).Mul(s.q, sevenKPlusTwo)
	newN.Add(newN, new(big.Int).Mul(s.r, s.l))

	denominator := new(big.Int).Mul(s.t, s.l)
	newN.Quo(newN, denominator)

	s.q.Mul(s.q, s.k)
	s.t.Mul(s.t, s.l)
	s.l.Add(s.l, big.NewInt(2))
	s.k.Add(s.k, big.NewInt(1))
	s.n = newN
	s.r = newR
}

func PiDecimal(fractionalDigits int) (string, error) {
	if fractionalDigits < 0 {
		return "", errNegativeDigits
	}

	generator := newSpigot()
	totalDigits := fractionalDigits + 1

	var builder strings.Builder
	builder.Grow(totalDigits + 1)

	for i := 0; i < totalDigits; i++ {
		if i == 1 {
			builder.WriteByte('.')
		}

		builder.WriteByte(byte('0' + generator.nextDigit()))
	}

	return builder.String(), nil
}
