package main

import (
	"fmt"
	"time"
)

func main() {
	for i := 1; i <= 30; i++ {
		fmt.Println(i)
		if i != 10 {
			time.Sleep(1 * time.Second)
		}
	}

	ending()
}
