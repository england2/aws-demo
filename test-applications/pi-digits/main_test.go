package main

import (
	"math/big"
	"strings"
	"testing"
	"testing/quick"
)

const piFirst100Digits = "3.14159265358979323846264338327950288419716939937510" +
	"58209749445923078164062862089986280348253421170679"

func TestComputePiKnownDigits(t *testing.T) {
	tests := []struct {
		name   string
		digits int
		want   string
	}{
		{name: "integer part only", digits: 0, want: "3"},
		{name: "one digit", digits: 1, want: "3.1"},
		{name: "ten digits", digits: 10, want: "3.1415926535"},
		{name: "one hundred digits", digits: 100, want: piFirst100Digits},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := computePi(test.digits)
			if got != test.want {
				t.Fatalf("computePi(%d) = %q, want %q", test.digits, got, test.want)
			}
		})
	}
}

func TestComputePiPrefixProperty(t *testing.T) {
	check := func(digits uint8) bool {
		requestedDigits := int(digits % 75)
		got := computePi(requestedDigits)
		want := piFirst100Digits[:2+requestedDigits]

		if requestedDigits == 0 {
			want = "3"
		}

		return got == want
	}

	if err := quick.Check(check, nil); err != nil {
		t.Fatal(err)
	}
}

func TestParseDigits(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		want      int
		wantError bool
	}{
		{name: "valid zero", args: []string{"0"}, want: 0},
		{name: "valid positive", args: []string{"250"}, want: 250},
		{name: "missing", args: nil, wantError: true},
		{name: "too many", args: []string{"1", "2"}, wantError: true},
		{name: "not an integer", args: []string{"1.5"}, wantError: true},
		{name: "negative", args: []string{"-1"}, wantError: true},
		{name: "too large", args: []string{"1000001"}, wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseDigits(test.args)
			if test.wantError {
				if err == nil {
					t.Fatal("parseDigits returned nil error")
				}
				return
			}

			if err != nil {
				t.Fatalf("parseDigits returned error: %v", err)
			}

			if got != test.want {
				t.Fatalf("parseDigits(%v) = %d, want %d", test.args, got, test.want)
			}
		})
	}
}

func TestFormatFixedDecimalPadsFractionalZeros(t *testing.T) {
	got := formatFixedDecimal(big.NewInt(30001), 4)
	if got != "3.0001" {
		t.Fatalf("formatFixedDecimal(...) = %q, want %q", got, "3.0001")
	}
}

func TestComputePiOutputShape(t *testing.T) {
	check := func(digits uint8) bool {
		requestedDigits := int(digits)
		got := computePi(requestedDigits)

		if requestedDigits == 0 {
			return got == "3"
		}

		return strings.HasPrefix(got, "3.") && len(got) == requestedDigits+2
	}

	if err := quick.Check(check, nil); err != nil {
		t.Fatal(err)
	}
}
