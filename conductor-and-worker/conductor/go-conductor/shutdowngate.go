package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type shutdownGateConfig struct {
	ShutdownRequestPath       string
	SafeShutdownSucceededPath string
	StopPolling               context.CancelFunc
	PollDone                  chan struct{}
	Registry                  *workerRegistry
}

func shutdownGate(config shutdownGateConfig) error {
	pollStopSignaled := false

	for {
		time.Sleep(5 * time.Second)

		shutdownOkayBytes, err := os.ReadFile(config.ShutdownRequestPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read shutdown file: %v\n", err)
			continue
		}

		isShutdownOkay, err := strconv.ParseBool(strings.TrimSpace(string(shutdownOkayBytes)))
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse shutdown file %q: %v\n", strings.TrimSpace(string(shutdownOkayBytes)), err)
			continue
		}
		if !isShutdownOkay {
			continue
		}

		if !pollStopSignaled {
			close(config.PollDone)
			config.StopPolling()
			pollStopSignaled = true
		}

		numActiveWorkers := config.Registry.getNumActiveWorkers()
		fmt.Printf("SHUTDOWN_OKAY is true, waiting for %d workers to finish", numActiveWorkers)
		if numActiveWorkers == 0 {
			break
		}
	}

	if err := os.WriteFile(config.SafeShutdownSucceededPath, []byte("true\n"), 0o644); err != nil {
		return fmt.Errorf("write safe shutdown succeeded file: %w", err)
	}

	return nil
}
