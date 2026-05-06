package tools

import (
	_ "embed"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

//go:embed check-time.go.md
var CheckTimeGuide string

// CheckTime prints elapsed wrapper runtime for the headless Codex agent.
// It depends on /tmp/agent-meta/start-time.txt written by the wrapper entrypoint.
// If the runtime exceeds the configured budget, it prints the ending report guide
// so Codex can stop and produce final output instead of running until Fargate kills it.
func CheckTime() {
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
	if elapsedMinutes >= 12 {

		fmt.Printf("Your elapsed time is >= 12 minutes. Therefore, due to this Fargate task's configured runtime budget, you have failed this task. Please heed the following instructions")
		Ending()
		return
	}

	fmt.Printf("%d\n have elasped; continue working.", elapsedMinutes)
}
