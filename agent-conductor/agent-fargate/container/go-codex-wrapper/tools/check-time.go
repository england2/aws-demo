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

// should replace this with codex SDK and a simple countdown timer in the future.
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
