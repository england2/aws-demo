package main

import (
	"context"
	"fmt"
	"os"
)

func main() {

	// fail early if the conductor cannot find or initialize a usable database.
	if err := check_load_db(); err != nil {
		fmt.Fprintf(os.Stderr, "load database: %v\n", err)
		os.Exit(1)
	}

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
