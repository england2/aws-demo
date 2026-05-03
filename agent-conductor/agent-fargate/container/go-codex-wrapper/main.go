package main

import (
	"codex-wrapper/tools"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func write_start_time() {
	metaDir := "/tmp/agent-meta"
	if err := os.MkdirAll(metaDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "create agent meta dir: %v\n", err)
		return
	}

	startTime := strconv.FormatInt(time.Now().Unix(), 10)
	startTimePath := filepath.Join(metaDir, "start-time.txt")
	if err := os.WriteFile(startTimePath, []byte(startTime+"\n"), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write start time: %v\n", err)
	}
}

func main() {
	write_start_time()

	registerBuiltinTools()

	if len(os.Args) > 1 && runToolArgument(os.Args[1]) {
		return
	}

	for i := 1; i <= 20; i++ {
		fmt.Println(i)
		if i != 10 {
			time.Sleep(1 * time.Second)
		}
	}

	tools.Ending()
}
