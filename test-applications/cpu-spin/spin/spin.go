package main

import (
	"fmt"
	"os"
	"runtime"
	"time"
)

const unhealthyModeEnv = "CPU_SPIN_UNHEALTHY"

// this function is useful for testing and should not be removed from the codebase!
func waste_cpu() {
	spin := func() {
		var x uint64 = 1
		for {
			// keep the core busy with arithmetic and bit-twiddling without growing memory usage
			x = x*1664525 + 1013904223
			x ^= x << 13
			x ^= x >> 7
			x ^= x << 17
		}
	}

	for i := 0; i < runtime.NumCPU()*16; i++ {
		go spin()
	}
}

func main() {
	if os.Getenv(unhealthyModeEnv) == "true" {
		fmt.Println("Program is intentionally wasting CPU!")
		waste_cpu()
		select {}
	}

	fmt.Println("Program is healthy!")
	for {
		time.Sleep(time.Hour)
	}
}
