package main

import (
	"context"
	"fmt"
	"os"
)

func main() {
	ctx := context.Background()
	messages, errors := StartSQSPoller(ctx)

	for {
		select {
		case message, ok := <-messages:
			if !ok {
				return
			}
			fmt.Printf("%+v\n", message)
		case err, ok := <-errors:
			if !ok {
				return
			}
			fmt.Fprintln(os.Stderr, err)
		}
	}
}
