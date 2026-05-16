package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// =============================================================
// Worker and Registry Core Logic
// =============================================================

type workerEventRecord struct {
	Kind       workerEvent
	ReceivedAt time.Time
	Payload    any
}

// spawnedWorker represents a worker that has been spawned by the conductor. The mental model is
// basically: "We spawn a remote fargate instance, and our conductor needs to track the state of
// that instance locally so we can manage it".
type spawnedWorker struct {
	mu sync.Mutex

	workerSpawnConfig
	ID  string
	Job string
	// Worker state is interpreted from recorded events instead of separate booleans for each state.
	Events []workerEventRecord
}

type workerRegistry struct {
	// workerRegistry scopes RPC messages from a unknown worker client and uses the carried
	// worker_id to scope a worker struct in the RPC prodecure implementation. Note: while gRPC uses
	// the verb 'register' in our code, our worker registry doesn't implement any proto-generated
	// code, even though it is always used in the methods that *do* implement the generated gRPC
	// interfaces.
	mu sync.Mutex
	// This slice is a shared resource between gRPC prodecures that are executed concurrently, so we
	// protect shared access using the above mutex.
	workers []*spawnedWorker
}

func newWorkerRegistry() *workerRegistry {
	return &workerRegistry{
		workers: []*spawnedWorker{},
	}
}

func (wr *workerRegistry) getWorker(workerID string) (*spawnedWorker, bool) {
	wr.mu.Lock()
	defer wr.mu.Unlock()

	for _, worker := range wr.workers {
		if worker.ID == workerID {
			return worker, true
		}
	}

	return nil, false
}

func (wr *workerRegistry) getRequiredWorker(workerID string) (*spawnedWorker, error) {
	worker, ok := wr.getWorker(workerID)
	if !ok {
		return nil, status.Errorf(codes.FailedPrecondition, "worker %q was not spawned by conductor", workerID)
	}

	return worker, nil
}

func (wr *workerRegistry) registerSpawnedWorker(conf workerSpawnConfig) (*spawnedWorker, error) {
	wr.mu.Lock()
	defer wr.mu.Unlock()

	for _, worker := range wr.workers {
		if worker.ID == conf.WorkerID {
			return nil, fmt.Errorf("worker %q is already registered", conf.WorkerID)
		}
	}

	worker := &spawnedWorker{
		ID:                conf.WorkerID,
		workerSpawnConfig: conf,
	}
	wr.workers = append(wr.workers, worker)

	return worker, nil
}

func (wr *workerRegistry) registerWorkerHandshake(workerID string, payload any) (*spawnedWorker, error) {
	worker, err := wr.getRequiredWorker(workerID)
	if err != nil {
		return nil, err
	}
	worker.recordEvent(handshakeSucceeded, payload)

	return worker, nil
}

func (wr *workerRegistry) registerWorkerSafelyEnded(workerID string, payload any) (*spawnedWorker, error) {
	worker, err := wr.getRequiredWorker(workerID)
	if err != nil {
		return nil, err
	}

	worker.recordEvent(safelyEnded, payload)

	return worker, nil
}

func (wr *workerRegistry) recordWorkerErrorAndDeregister(workerID string, payload any) (*spawnedWorker, error) {
	worker, err := wr.getRequiredWorker(workerID)
	if err != nil {
		return nil, err
	}

	worker.recordEvent(codexError, payload)
	if err := wr.removeWorker(workerID); err != nil {
		return nil, err
	}

	return worker, nil
}

func (wr *workerRegistry) waitFargateAndDeregister(workerID string) error {
	// Future production path waits for the worker's Fargate task to stop before deregistering.
	return wr.removeWorker(workerID)
}

func (wr *workerRegistry) removeWorker(workerID string) error {
	wr.mu.Lock()
	defer wr.mu.Unlock()

	for workerIndex, worker := range wr.workers {
		if worker.ID != workerID {
			continue
		}

		if !worker.hasEvent(safelyEnded) && !worker.hasEvent(codexError) && !worker.hasEvent(badlyEnded) {
			fmt.Printf("warning: removing worker %s without a %s event\n", worker.ID, safelyEnded)
		}

		wr.workers = append(wr.workers[:workerIndex], wr.workers[workerIndex+1:]...)
		return nil
	}

	return status.Errorf(codes.FailedPrecondition, "worker %q was not registered", workerID)
}

func (wr *workerRegistry) getNumActiveWorkers() int {
	wr.mu.Lock()
	defer wr.mu.Unlock()

	numActiveWorkers := 0
	for _, worker := range wr.workers {
		if !worker.hasEvent(safelyEnded) {
			numActiveWorkers++
		}
	}

	return numActiveWorkers
}

// workerSpawnStartedEvent records the prepared config attached to a registry spawn event.
// It is written before the launcher starts the worker, so later event inspection can see what identity and
// work-file directory the conductor intended to launch.
type workerSpawnStartedEvent struct {
	Config workerSpawnConfig
}

// workerLauncher is the boundary between registry bookkeeping and the selected spawn mechanism.
// main passes either the local Docker smoke launcher or a Fargate closure, so spawnWorker can keep one event
// sequence without knowing how the worker process is started.
type workerLauncher func(context.Context, workerSpawnConfig) error

// spawnWorker registers one prepared worker and then invokes the caller-selected launcher.
// It must run after prepareWorkerSpawnConfig has created WorkFilesDir, because worker RPCs may request files as
// soon as the launcher starts the remote or local worker process.
func (wr *workerRegistry) spawnWorker(ctx context.Context, conf workerSpawnConfig, launchWorker workerLauncher) error {
	worker, err := wr.registerSpawnedWorker(conf)
	if err != nil {
		return err
	}
	worker.recordEvent(spawnStarted, workerSpawnStartedEvent{
		Config: conf,
	})

	if err := launchWorker(ctx, conf); err != nil {
		worker.recordEvent(badlyEnded, err)
		if removeErr := wr.removeWorker(conf.WorkerID); removeErr != nil {
			return fmt.Errorf("launch worker %q: %w; additionally failed to remove registered worker: %v", conf.WorkerID, err, removeErr)
		}
		return fmt.Errorf("launch worker %q: %w", conf.WorkerID, err)
	}

	return nil
}

func validateWorkerID(workerID string) error {
	if workerID == "" {
		return status.Error(codes.InvalidArgument, "worker_id is required")
	}

	return nil
}

func printWorkerMessage(workerID string, workerMessage string) {
	fmt.Printf("[from worker %s]: %q\n", workerID, workerMessage)
}

// =============================================================
//  Events and Event Fuctions
// =============================================================

// Some worker events map to RPC procedures and some are unique within the conductor.
const (
	spawnStarted           workerEvent = "worker_spawn_started"
	handshakeSucceeded     workerEvent = "worker_handshake_succeeded"
	handshakeFailed        workerEvent = "worker_handshake_failed"
	gotFilesSucceeded      workerEvent = "worker_got_files_succeeded"
	gotFilesFailed         workerEvent = "worker_got_files_failed"
	uploadedFilesSucceeded workerEvent = "worker_uploaded_files_succeeded"
	startsTest             workerEvent = "worker_starts_test"
	codexError             workerEvent = "worker_codex_error"
	safelyEnded            workerEvent = "worker_safely_ended"
	badlyEnded             workerEvent = "worker_badly_ended"
)

type workerEvent string

func (w *spawnedWorker) recordEvent(kind workerEvent, payload any) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.Events = append(w.Events, workerEventRecord{
		Kind:       kind,
		ReceivedAt: time.Now(),
		Payload:    payload,
	})
}

func (w *spawnedWorker) getLastEvent(kind workerEvent) *workerEventRecord {
	w.mu.Lock()
	defer w.mu.Unlock()

	for i := len(w.Events) - 1; i >= 0; i-- {
		if w.Events[i].Kind == kind {
			event := w.Events[i]
			return &event
		}
	}

	return nil
}

func (w *spawnedWorker) hasEvent(kind workerEvent) bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, event := range w.Events {
		if event.Kind == kind {
			return true
		}
	}

	return false
}
