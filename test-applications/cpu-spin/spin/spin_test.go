package main

import "testing"

func TestShouldWasteCpuDefaultsToHealthy(t *testing.T) {
	t.Setenv(unhealthyModeEnv, "")

	if shouldWasteCpu() {
		t.Fatal("expected CPU spin to be disabled by default")
	}
}

func TestShouldWasteCpuRequiresExplicitUnhealthyMode(t *testing.T) {
	t.Setenv(unhealthyModeEnv, "true")

	if !shouldWasteCpu() {
		t.Fatal("expected CPU spin to be enabled when explicitly requested")
	}
}
