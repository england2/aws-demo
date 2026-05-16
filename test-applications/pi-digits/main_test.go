package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestPiDigits(t *testing.T) {
	tests := []struct {
		name   string
		digits int
		want   string
	}{
		{
			name:   "one digit",
			digits: 1,
			want:   "3",
		},
		{
			name:   "two digits",
			digits: 2,
			want:   "3.1",
		},
		{
			name:   "ten digits",
			digits: 10,
			want:   "3.141592653",
		},
		{
			name:   "fifty digits",
			digits: 50,
			want:   "3.1415926535897932384626433832795028841971693993751",
		},
		{
			name:   "one hundred digits",
			digits: 100,
			want:   "3.141592653589793238462643383279502884197169399375105820974944592307816406286208998628034825342117067",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := piDigits(test.digits)
			if err != nil {
				t.Fatalf("piDigits(%d) returned error: %v", test.digits, err)
			}

			if got != test.want {
				t.Fatalf("piDigits(%d) = %q, want %q", test.digits, got, test.want)
			}
		})
	}
}

func TestPiDigitsRejectsInvalidDigitCount(t *testing.T) {
	_, err := piDigits(0)
	if err == nil {
		t.Fatal("piDigits(0) returned nil error")
	}
}

func TestRunPrintsPiDigits(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"pi-digits", "10"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("run returned exit code %d, want 0; stderr: %s", exitCode, stderr.String())
	}

	if stdout.String() != "3.141592653\n" {
		t.Fatalf("stdout = %q, want pi output", stdout.String())
	}

	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunRejectsMissingDigitCount(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"pi-digits"}, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("run returned exit code %d, want 1", exitCode)
	}

	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}

	if !strings.Contains(stderr.String(), "Usage: pi-digits <digits>") {
		t.Fatalf("stderr = %q, want usage", stderr.String())
	}
}

func TestRunRejectsMissingProgramNameAndDigitCount(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run(nil, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("run returned exit code %d, want 1", exitCode)
	}

	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}

	if !strings.Contains(stderr.String(), "Usage: pi-digits <digits>") {
		t.Fatalf("stderr = %q, want fallback usage", stderr.String())
	}
}

func TestRunRejectsInvalidDigitCount(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"pi-digits", "-1"}, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("run returned exit code %d, want 1", exitCode)
	}

	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}

	if !strings.Contains(stderr.String(), errInvalidDigitCount.Error()) {
		t.Fatalf("stderr = %q, want invalid digit count error", stderr.String())
	}
}
