package main

import (
	"fmt"
	"sync"
	"time"

	sharedproto "conductor-testing/proto"
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

// spawnedWorker *represents* a worker that has been spawned by the conductor.
// The mental model is basically: "We spawn a remote fargate instance, and our conductor needs to
// track the state of that instance locally so we can manage it".
type spawnedWorker struct {
	mu sync.Mutex

	workerSpawnConfig
	ID     string
	Job    string
	Events []workerEventRecord
}

func (w *spawnedWorker) didHandshakeSucceed() bool {
	// We interpet worker state by inspecting seen worker events rather than creating and fliping
	// booleans fields in workers structs for every possible worker state.
	return w.hasEvent(WorkerEventHandshakeSucceeded)
}

func (w *spawnedWorker) didSafelyEnd() bool {
	return w.hasEvent(WorkerEventSafelyEnded)
}

type workerRegistry struct {
	// workerRegistry scopes RPC messages from a unknown worker client and uses the carried
	// worker_id to scope a worker struct in the RPC prodecure implementation.
	// Note: while gRPC uses the verb 'register' in our code, our worker registry doesn't implement
	// any proto-generated code, even though it is often used in the functions that do implement the
	// generated gRPC interfaces.
	mu sync.Mutex
	// This slice is a shared resource between gRPC prodecures that are executed concurrently, hence the mutex.
	workers []*spawnedWorker
}

func newWorkerRegistry() *workerRegistry {
	return &workerRegistry{
		workers: []*spawnedWorker{},
	}
}

func (awr *workerRegistry) getWorker(workerID string) (*spawnedWorker, bool) {
	awr.mu.Lock()
	defer awr.mu.Unlock()

	for _, worker := range awr.workers {
		if worker.ID == workerID {
			return worker, true
		}
	}

	return nil, false
}

func (awr *workerRegistry) registerSpawnedWorker(conf workerSpawnConfig) (*spawnedWorker, error) {
	awr.mu.Lock()
	defer awr.mu.Unlock()

	for _, worker := range awr.workers {
		if worker.ID == conf.WorkerID {
			return nil, fmt.Errorf("worker %q is already registered", conf.WorkerID)
		}
	}

	worker := &spawnedWorker{
		ID:                conf.WorkerID,
		workerSpawnConfig: conf,
	}
	awr.workers = append(awr.workers, worker)

	return worker, nil
}

func (awr *workerRegistry) registerWorkerHandshake(workerID string, payload any) (*spawnedWorker, error) {
	worker, ok := awr.getWorker(workerID)
	if !ok {
		return nil, status.Errorf(codes.FailedPrecondition, "worker %q was not spawned by conductor", workerID)
	}
	if !worker.recordEventIfMissing(WorkerEventHandshakeSucceeded, payload) {
		return nil, status.Errorf(codes.AlreadyExists, "worker %q already completed handshake", workerID)
	}

	return worker, nil
}

func (awr *workerRegistry) registerWorkerSafelyEnded(workerID string, payload any) (*spawnedWorker, error) {
	worker, ok := awr.getWorker(workerID)
	if !ok {
		return nil, status.Errorf(codes.FailedPrecondition, "worker %q was not spawned by conductor", workerID)
	}

	worker.recordEvent(WorkerEventSafelyEnded, payload)

	return worker, nil
}

func (awr *workerRegistry) recordWorkerErrorAndDeregister(workerID string, payload any) (*spawnedWorker, error) {
	worker, ok := awr.getWorker(workerID)
	if !ok {
		return nil, status.Errorf(codes.FailedPrecondition, "worker %q was not spawned by conductor", workerID)
	}

	worker.recordEvent(WorkerEventCodexError, payload)
	if err := awr.removeWorker(workerID); err != nil {
		return nil, err
	}

	return worker, nil
}

func (awr *workerRegistry) waitFargateAndDeregister(workerID string) error {
	// Future production path waits for the worker's Fargate task to stop before deregistering.
	return awr.removeWorker(workerID)
}

func (awr *workerRegistry) removeWorker(workerID string) error {
	awr.mu.Lock()
	defer awr.mu.Unlock()

	for workerIndex, worker := range awr.workers {
		if worker.ID != workerID {
			continue
		}

		if !worker.didSafelyEnd() && !worker.hasEvent(WorkerEventCodexError) {
			fmt.Printf("warning: removing worker %s without a %s event\n", worker.ID, WorkerEventSafelyEnded.String())
		}

		awr.workers = append(awr.workers[:workerIndex], awr.workers[workerIndex+1:]...)
		return nil
	}

	return status.Errorf(codes.FailedPrecondition, "worker %q was not registered", workerID)
}

func (awr *workerRegistry) getNumActiveWorkers() int {
	awr.mu.Lock()
	defer awr.mu.Unlock()

	numActiveWorkers := 0
	for _, worker := range awr.workers {
		if !worker.didSafelyEnd() {
			numActiveWorkers++
		}
	}

	return numActiveWorkers
}

// ai extra event
type workerSpawnStartedEvent struct {
	Config workerSpawnConfig
}

func (awr *workerRegistry) spawnWorker(conf workerSpawnConfig) error {
	worker, err := awr.registerSpawnedWorker(conf)
	if err != nil {
		return err
	}
	worker.recordEvent(WorkerEventSpawnStarted, workerSpawnStartedEvent{
		Config: conf,
	})

	if _, err := execWorkerProcessTestingDocker(conf); err != nil {
		return err
	}

	return nil
}

func workerIDFromIdentity(identity *sharedproto.WorkerIdentity) (string, error) {
	workerID := identity.GetWorkerId()
	if workerID == "" {
		return "", status.Error(codes.InvalidArgument, "worker.worker_id is required")
	}

	return workerID, nil
}

// =============================================================
//  Events and Event Fuctions
// =============================================================

// Some worker events map to RPC procedures and some are unique within the conductor.
//
//go:generate /home/t/go/bin/stringer -type=workerEvent -linecomment serverside-worker.go
const (
	WorkerEventSpawnStarted           workerEvent = iota // worker_spawn_started
	WorkerEventHandshakeSucceeded                        // worker_handshake_succeeded
	WorkerEventHandshakeFailed                           // worker_handshake_failed
	WorkerEventGotFilesSucceeded                         // worker_got_files_succeeded
	WorkerEventGotFilesFailed                            // worker_got_files_failed
	WorkerEventUploadedFilesSucceeded                    // worker_uploaded_files_succeeded
	WorkerEventStartsTest                                // worker_starts_test
	WorkerEventCodexError                                // worker_codex_error
	WorkerEventSafelyEnded                               // worker_safely_ended
	WorkerEventBadlyEnded                                // worker_badly_ended
)

type workerEvent int

// workerSpawnConfig contains configuration needed to spawn workers, and is later put in the struct representing an active worker.

func (w *spawnedWorker) recordEvent(kind workerEvent, payload any) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.Events = append(w.Events, workerEventRecord{
		Kind:       kind,
		ReceivedAt: time.Now(),
		Payload:    payload,
	})
}

func (w *spawnedWorker) recordEventIfMissing(kind workerEvent, payload any) bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, event := range w.Events {
		if event.Kind == kind {
			return false
		}
	}

	w.Events = append(w.Events, workerEventRecord{
		Kind:       kind,
		ReceivedAt: time.Now(),
		Payload:    payload,
	})

	return true
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
