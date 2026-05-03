package tools

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

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

		fmt.Printf("Your elapsed time is >= 12 minutes! Therefore, due to AWS Lambda limitations, you have failed this task. Please heed the following instructions")
		Ending()
		return
	}

	fmt.Printf("%d\n have elasped; continue working.", elapsedMinutes)
}
