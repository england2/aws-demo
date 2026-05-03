package main

import (
	"codex-wrapper/tools"
	"fmt"
	"os"
	"time"
)

func main() {
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
