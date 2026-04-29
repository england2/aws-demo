package main

import (
	"fmt"
	"runtime"
	"time"
)

const healthy = true

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
	if healthy {
		fmt.Println("Program is healthy!")
		for {
			time.Sleep(time.Hour)
		}
	} else {
		waste_cpu()
		select {}
	}
}
