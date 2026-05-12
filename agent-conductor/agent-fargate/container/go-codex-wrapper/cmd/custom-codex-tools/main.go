package main

import (
	_ "embed"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const runtimeBudgetMinutes = 12

//go:embed check-time.md
var checkTimeGuide string

//go:embed print-tool-guides.md
var printToolGuidesGuide string

var toolGuides = map[string]string{
	"check-time":        checkTimeGuide,
	"print-tool-guides": printToolGuidesGuide,
}

func main() {
	if len(os.Args) != 2 {
		printUsage()
		os.Exit(1)
	}

	toolName, ok := strings.CutPrefix(os.Args[1], "--")
	if !ok || toolName == "" {
		printUsage()
		os.Exit(1)
	}

	switch toolName {
	case "check-time":
		checkTime()
	case "print-tool-guides":
		printToolGuides()
	default:
		fmt.Printf("unknown tool: %s\navailable tools: %s\n", toolName, strings.Join(registeredToolNames(), ", "))
		os.Exit(1)
	}
}

func checkTime() {
	startTimeBytes, err := os.ReadFile("/tmp/agent-meta/start-time.txt")
	if err != nil {
		fmt.Printf("failed to read start time: %v\n", err)
		return
	}

	startTime, err := strconv.ParseInt(strings.TrimSpace(string(startTimeBytes)), 10, 64)
	if err != nil {
		fmt.Printf("failed to parse start time: %v\n", err)
		return
	}

	elapsedMinutes := int(time.Since(time.Unix(startTime, 0)).Minutes())
	if elapsedMinutes >= runtimeBudgetMinutes {
		fmt.Printf("Elapsed time is %d minutes, which meets or exceeds the %d minute Fargate task budget. Stop active work and produce the final result now.\n", elapsedMinutes, runtimeBudgetMinutes)
		return
	}

	fmt.Printf("%d minutes have elapsed; continue working.\n", elapsedMinutes)
}

func printToolGuides() {
	for _, name := range registeredToolNames() {
		fmt.Printf("# =============================================================\n%s\n\n", strings.TrimSpace(toolGuides[name]))
	}
}

func registeredToolNames() []string {
	names := make([]string, 0, len(toolGuides))
	for name := range toolGuides {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func printUsage() {
	fmt.Printf("usage: custom-codex-tools --<tool>\navailable tools: %s\n", strings.Join(registeredToolNames(), ", "))
}
