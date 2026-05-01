package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Println("Program is healthy!")
	for {
		time.Sleep(time.Hour)
	}
}
