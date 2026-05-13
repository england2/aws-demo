package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	sharedproto "conductor-testing/proto"
	util "go-conductor/util"

	scheduler "go-conductor/go-db-scheduler"


	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var serverAddr = flag.String("addr", "localhost:50055", "The server address in the format of host:port")
var dbLocation = flag.String("test-db-loc", "", "path to the test database file")

type conductorServer struct {
	sharedproto.UnimplementedWorkerEventReceiverServiceServer
	registry *workerRegistry
}

// =============================================================
// gRPC Server-side procedure implementations
// =============================================================
//
// These functions are handled concurrently by the conductor's gRPC server.
// Each request carries WorkerIdentity so the handler can attach work to the
// conductor's in-memory representation of the worker that sent it.
//

func (s *conductorServer) WorkerStartsTest(ctx context.Context, msg *sharedproto.Test) (*sharedproto.TestResponse, error) {
	workerID, err := workerIDFromIdentity(msg.GetWorker())
	if err != nil {
		return nil, err
	}

	worker, ok := s.registry.getWorker(workerID)
	if !ok {
		return nil, status.Errorf(codes.FailedPrecondition, "worker %q was not spawned by conductor", workerID)
	}
	if !worker.didHandshakeSucceed() {
		return nil, status.Errorf(codes.FailedPrecondition, "worker %q has not completed handshake", workerID)
	}

	worker.recordEvent(WorkerEventStartsTest, msg)

	fmt.Printf("[from worker: %q] %q\n", worker.ID, msg.GetWorkerMessage())

	return &sharedproto.TestResponse{
		Worker:        msg.GetWorker(),
		WorkerMessage: fmt.Sprintf("Conductor: I got your message %s", msg.GetWorkerMessage()),
	}, nil
}

func (s *conductorServer) WorkerStartsHandshake(ctx context.Context, msg *sharedproto.Handshake) (*sharedproto.HandshakeResponse, error) {
	workerID, err := workerIDFromIdentity(msg.GetWorker())
	if err != nil {
		return nil, err
	}

	worker, err := s.registry.registerWorkerHandshake(workerID, msg)
	if err != nil {
		return nil, err
	}

	fmt.Printf("[from worker: %q] %q\n", worker.ID, msg.GetWorkerMessage())

	return &sharedproto.HandshakeResponse{
		WorkerMessage: fmt.Sprintf("Conductor gets message: \"%s\"", msg.GetWorkerMessage()),
	}, nil
}

func (s *conductorServer) WorkerStartsShutdown(ctx context.Context, msg *sharedproto.Shutdown) (*sharedproto.GeneralResponse, error) {
	workerID, err := workerIDFromIdentity(msg.GetWorker())
	if err != nil {
		return nil, err
	}

	worker, err := s.registry.registerWorkerSafelyEnded(workerID, msg)
	if err != nil {
		return nil, err
	}

	fmt.Printf("[from worker: %q] %q\n", worker.ID, msg.GetWorkerMessage())

	if err := s.registry.waitFargateAndDeregister(workerID); err != nil {
		return nil, err
	}

	return &sharedproto.GeneralResponse{
		WorkerId:      worker.ID,
		WorkerMessage: fmt.Sprintf("Conductor gets message: \"%s\"", msg.GetWorkerMessage()),
	}, nil
}

func (s *conductorServer) WorkerSendsCodexError(ctx context.Context, msg *sharedproto.CodexError) (*sharedproto.GeneralResponse, error) {
	workerID := msg.GetWorkerId()
	if workerID == "" {
		return nil, status.Error(codes.InvalidArgument, "worker_id is required")
	}

	worker, err := s.registry.recordWorkerErrorAndDeregister(workerID, msg)
	if err != nil {
		return nil, err
	}

	fmt.Printf("[from worker: %q] %q\n", worker.ID, msg.GetWorkerMessage())

	return &sharedproto.GeneralResponse{
		WorkerId:      worker.ID,
		WorkerMessage: fmt.Sprintf("Conductor gets message: \"%s\"", msg.GetWorkerMessage()),
	}, nil
}

// This file (`false` by default) will be flipped by a simple CI step.
// After this file is flipped, the Conductor will not schedule any more agent jobs, and will wait for current jobs to finish.
const conductorShuttingDown = "IS_CONDUCTOR_SHUTTING_DOWN"

var isGlobalShutdownOkay bool



func dbtestingmain() {
	flag.Parse()

	// ============================================================
	// Before new message
	// ============================================================

	decisions, err := scheduler.Run(context.Background(), scheduler.Config{
		DBPath: *dbLocation,
	})
	if err != nil {
		panic(err)
	}

	encoded, err := json.MarshalIndent(decisions, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(string(encoded))
}



func main_real() {
	flag.Parse()

	// build runtime and shutdown files
	runDir := filepath.Join(os.TempDir(), "conductor-run")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		log.Fatalf("create run dir: %v", err)
	}
	shutdownOkayPath := filepath.Join(runDir, conductorShuttingDown)
	if err := os.Remove(shutdownOkayPath); err != nil && !os.IsNotExist(err) {
		log.Fatalf("remove old shutdown file: %v", err)
	}
	if err := os.WriteFile(shutdownOkayPath, []byte("false\n"), 0o644); err != nil {
		log.Fatalf("initialize shutdown file: %v", err)
	}

	fmt.Printf("conductor listening address: %s\n", *serverAddr)
	fmt.Printf("conductor run directory: %s\n", runDir)

	listener, err := net.Listen("tcp", *serverAddr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	// The conductor's gRPC server invokes our server-side RPC methods without giving us a
	// convenient client identity. Each worker request carries worker_id in the protobuf payload,
	// and the registry maps that ID to the conductor's in-memory worker representation (that is, an
	// instance the `spawnedWorker` struct).
	registry := newWorkerRegistry()
	conductorServiceImplementation := &conductorServer{
		registry: registry,
	}

	grpcServer := grpc.NewServer()
	sharedproto.RegisterWorkerEventReceiverServiceServer(grpcServer, conductorServiceImplementation)

	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			log.Fatalf("serve: %v", err)
		}
	}()

	// ---- test spawning ----
	if err := registry.spawnWorker(workerSpawnConfig{
		ConductorGrpcServerAddr: *serverAddr,
		WorkerID:                util.GenerateWorkerName(),
		RunDir:                  runDir,
	}); err != nil {
		log.Fatalf("spawn worker: %v", err)
	}
	// ---- test spawning over ----

	// Safe shutdown gate.
	var numActiveWorkers int
	for {
		time.Sleep(5 * time.Second)

		shutdownOkayBytes, err := os.ReadFile(shutdownOkayPath)
		if err != nil {
			log.Fatalf("read shutdown file: %v", err)
		}

		isShutdownOkay, err := strconv.ParseBool(strings.TrimSpace(string(shutdownOkayBytes)))
		if err != nil {
			log.Fatalf("parse shutdown file: %v", err)
		}

		if isShutdownOkay {
			numActiveWorkers = registry.getNumActiveWorkers()
			fmt.Printf("SHUTDOWN_OKAY is true, waiting for %d workers to finish", numActiveWorkers)
		} else {
			continue
		}

		if isShutdownOkay && numActiveWorkers == 0 {
			break
		}

	}

	// Writes `CONDUCTOR_READY_FOR_SAFE_SHUTDOWN`, which the CI waits to exit
	// Also ensure that the CI deployment waiter also waits on `pgrep -f '^/usr/local/bin/go-conductor$'` to have a non-zero exit code.
	safeShutdownSucceededPath := filepath.Join(runDir, "CONDUCTOR_READY_FOR_SAFE_SHUTDOWN")
	if err := os.WriteFile(safeShutdownSucceededPath, []byte("true\n"), 0o644); err != nil {
		log.Fatalf("write safe shutdown succeeded file: %v", err)
	}
}


func main() {
	main_testing()
}


// ai--- can we import stuff????
func main_testing() {
	fmt.Println("CALLING DBTESTING MAIN")
	dbtestingmain()
}
