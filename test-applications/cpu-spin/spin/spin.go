package main

import (
	"fmt"
	"runtime"
	"time"
)

const isHealthy = false

// this function is useful for testing and should not be removed from the codebase!
func wasteCpu() {
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
	if isHealthy {
		fmt.Println("Program is intentionally wasting CPU!")
		wasteCpu()
		select {}
	}

	fmt.Println("Program is healthy!")
	for {
		time.Sleep(time.Hour)
	}
}
