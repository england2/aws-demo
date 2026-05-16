package main

import (
	"errors"
	"testing"
)

func TestPiDecimalKnownPrefixes(t *testing.T) {
	tests := []struct {
		name             string
		fractionalDigits int
		want             string
	}{
		{name: "whole number", fractionalDigits: 0, want: "3"},
		{name: "one fractional digit", fractionalDigits: 1, want: "3.1"},
		{name: "ten fractional digits", fractionalDigits: 10, want: "3.1415926535"},
		{name: "fifty fractional digits", fractionalDigits: 50, want: "3.14159265358979323846264338327950288419716939937510"},
		{name: "one hundred fractional digits", fractionalDigits: 100, want: "3.1415926535897932384626433832795028841971693993751058209749445923078164062862089986280348253421170679"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := PiDecimal(test.fractionalDigits)
			if err != nil {
				t.Fatalf("PiDecimal(%d) returned error: %v", test.fractionalDigits, err)
			}

			if got != test.want {
				t.Fatalf("PiDecimal(%d) = %q, want %q", test.fractionalDigits, got, test.want)
			}
		})
	}
}

func TestPiDecimalRejectsNegativeDigits(t *testing.T) {
	_, err := PiDecimal(-1)
	if !errors.Is(err, errNegativeDigits) {
		t.Fatalf("PiDecimal(-1) error = %v, want %v", err, errNegativeDigits)
	}
}

func TestParseDigits(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
	}{
		{name: "flag", args: []string{"-digits", "25"}, want: 25},
		{name: "positional", args: []string{"25"}, want: 25},
		{name: "zero", args: []string{"0"}, want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseDigits(test.args)
			if err != nil {
				t.Fatalf("parseDigits(%v) returned error: %v", test.args, err)
			}

			if got != test.want {
				t.Fatalf("parseDigits(%v) = %d, want %d", test.args, got, test.want)
			}
		})
	}
}

func TestParseDigitsRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing count", args: nil},
		{name: "negative flag", args: []string{"-digits", "-1"}},
		{name: "negative positional", args: []string{"-1"}},
		{name: "non number", args: []string{"abc"}},
		{name: "too many args", args: []string{"10", "20"}},
		{name: "flag and positional", args: []string{"-digits", "10", "20"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseDigits(test.args)
			if err == nil {
				t.Fatalf("parseDigits(%v) returned nil error", test.args)
			}
		})
	}
}
