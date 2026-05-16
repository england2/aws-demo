package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	sharedproto "conductor-testing/proto"
	scheduler "go-conductor/go-db-scheduler"
	util "go-conductor/util"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	serverAddr       = flag.String("addr", "localhost:50055", "The server address in the format of host:port")
	workerDialAddr   = flag.String("worker-dial-addr", "", "The conductor address workers should dial; defaults to addr")
	dbLocation       = flag.String("test-db-loc", "", "path to the test database file")
	debugAlwaysNewDB = flag.Bool("debug_always_new_db", false, "create a fresh sibling scheduler database before polling")
)

type conductorServer struct {
	// The conductor's gRPC server invokes our server-side RPC methods without giving us a
	// convenient client identity. Each worker request carries worker_id in the protobuf payload,
	// and the registry maps that ID to the conductor's in-memory worker representation (that is, an
	// instance the `spawnedWorker` struct).
	sharedproto.UnimplementedWorkerEventReceiverServiceServer
	registry *workerRegistry
}

// =============================================================
// gRPC Server-side procedure implementations
// =============================================================
//
// These functions are handled concurrently by the conductor's gRPC server. Each request carries
// WorkerIdentity so the handler can attach work to the conductor's in-memory representation of the
// worker that sent it.
//

func (s *conductorServer) WorkerStartsHandshake(ctx context.Context, msg *sharedproto.Handshake) (*sharedproto.HandshakeResponse, error) {
	workerID, err := workerIDFromIdentity(msg.GetWorker())
	if err != nil {
		return nil, err
	}

	worker, err := s.registry.registerWorkerHandshake(workerID, msg)
	if err != nil {
		return nil, err
	}

	printWorkerMessage(worker.ID, msg.GetWorkerMessage())

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

	printWorkerMessage(worker.ID, msg.GetWorkerMessage())

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

	printWorkerMessage(worker.ID, msg.GetWorkerMessage())

	return &sharedproto.GeneralResponse{
		WorkerId:      worker.ID,
		WorkerMessage: fmt.Sprintf("Conductor gets message: \"%s\"", msg.GetWorkerMessage()),
	}, nil
}

// This file (`false` by default) will be flipped by a simple CI step.
// After this file is flipped, the Conductor will not schedule any more agent jobs, and will wait for current jobs to finish.
const (
	conductorShuttingDown = "IS_CONDUCTOR_SHUTTING_DOWN"
	conductorRootPath     = "/conductor"
	conductorRunDirName   = "run"
)

var isGlobalShutdownOkay bool

// temporary reference function showing how to use the scheduler package

// conductorWorkerDialAddress returns the address passed into worker container env.
// It is intentionally separate from the listen address because AWS workers must dial the EC2 private IP,
// while the conductor process listens on 0.0.0.0 inside Docker.
func conductorWorkerDialAddress() string {
	if strings.TrimSpace(*workerDialAddr) != "" {
		return *workerDialAddr
	}

	return *serverAddr
}

// FIXME: 252 LoC in main, surely we can do better!
func main() {
	if err := runConductor(); err != nil {
		fmt.Fprintf(os.Stderr, "conductor exited: %v\n", err)
		os.Exit(1)
	}
}

// runConductor wires startup, SQS polling, scheduling, worker spawning, and the gRPC server.
// Startup errors are returned to main, while per-message and shutdown-file transient errors are printed
// so one bad AWS call or half-written shutdown flag does not kill active worker coordination.
func runConductor() error {
	// =============================================================
	// Runtime path, Files, and Args
	// =============================================================

	// build runtime and shutdown files
	runDir := filepath.Join(conductorRootPath, conductorRunDirName)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return fmt.Errorf("create run dir: %w", err)
	}
	shutdownOkayPath := filepath.Join(runDir, conductorShuttingDown)
	if err := os.Remove(shutdownOkayPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove old shutdown file: %w", err)
	}
	if err := os.WriteFile(shutdownOkayPath, []byte("false\n"), 0o644); err != nil {
		return fmt.Errorf("initialize shutdown file: %w", err)
	}

	fmt.Printf("in main_testing\n\n")

	flag.Parse()

	// =============================================================
	// Database Setup and Polling Setup
	// =============================================================

	schedulerDatabasePath := testSchedulerDatabasePathFromFlags()
	if schedulerDatabasePath == "" {
		return fmt.Errorf("test-db-loc is required")
	}

	dbContext := context.Background()

	if *debugAlwaysNewDB {
		createdDatabasePath, err := debugCreateNewDbAndSetLocation(dbContext, schedulerDatabasePath)
		if err != nil {
			return fmt.Errorf("create debug scheduler database: %w", err)
		}
		schedulerDatabasePath = createdDatabasePath
	} else if !checkIsDbCompliant(dbContext, schedulerDatabasePath) {
		fmt.Fprintf(os.Stderr, "database is not compliant: %s\n", schedulerDatabasePath)
		return nil
	}

	schedulerWorker, err := scheduler.Open(dbContext, scheduler.Config{
		DBPath: schedulerDatabasePath,
	})
	if err != nil {
		return fmt.Errorf("open scheduler: %w", err)
	}
	defer schedulerWorker.Close()

	pollingContext, cancelPollingContext := context.WithCancel(context.Background())
	defer cancelPollingContext()

	sqsPoller, err := NewTicketCloudWatchSQSPoller(pollingContext)
	if err != nil {
		return fmt.Errorf("create sqs poller: %w", err)
	}

	// =============================================================
	// gRPC server setup
	// =============================================================

	fmt.Printf("conductor listening address: %s\n", *serverAddr)
	fmt.Printf("worker dial address: %s\n", conductorWorkerDialAddress())
	fmt.Printf("conductor run directory: %s\n", runDir)

	listener, err := net.Listen("tcp", *serverAddr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	registry := newWorkerRegistry()
	conductorServiceImplementation := &conductorServer{
		registry: registry,
	}

	grpcServer := grpc.NewServer()
	sharedproto.RegisterWorkerEventReceiverServiceServer(grpcServer, conductorServiceImplementation)

	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			fmt.Fprintf(os.Stderr, "serve grpc: %v\n", err)
		}
	}()

	// =============================================================
	// Poll loop
	// =============================================================

	messages, pollErrors := sqsPoller.Start(pollingContext)

	fmt.Printf("polling SQS queue with scheduler DB %s\n", schedulerDatabasePath)

	// This channel is used to stop polling SQS messages (and therefore stop accepting jobs) when we're deploying a new version of the conductor.
	chanPollDone := make(chan struct{})

	go func(pollLoopContext context.Context, chanPollDone <-chan struct{}) {
		defer fmt.Println("Polling loop ended")

		for {
			select {
			case <-chanPollDone:
				return
			case <-pollLoopContext.Done():
				return
			case polledSQSMessage, ok := <-messages:
				if !ok {
					return
				}

				// ==============================================================================
				// This block is executed whenever the poller gets an SQS message. Largely, it is
				// one of the major brains of the program in addition to the scheduler logic and
				// worker-managing gRPC prodecures.
				// ==============================================================================

				// Correctly gaurd against milisecond scenarios where we get a SQS message in the
				// space of Go's for-select turn speed after we should be draining the server.
				select {
				case <-chanPollDone:
					return
				case <-pollLoopContext.Done():
					return
				default:
				}

				fmt.Println("[Conductor] got sqs message!")

				// Call to the scheduler.
				scheduleDecisions, err := insertPolledSQSMessageAndRunScheduler(pollLoopContext, schedulerWorker, polledSQSMessage)
				if err == nil {
					err = sqsPoller.DeleteMessage(pollLoopContext, polledSQSMessage.ReceiptHandle)
				} else {
					fmt.Fprintf(os.Stderr, "handle sqs message %q: %v\n", polledSQSMessage.ExternalMessageID, err)
					continue
				}
				if err := printSchedulerDecisions(scheduleDecisions); err != nil {
					fmt.Fprintf(os.Stderr, "print scheduler decisions for sqs message %q: %v\n", polledSQSMessage.ExternalMessageID, err)
				}

				// =========================================================
				// Worker Spawn Block
				// =========================================================
				for _, scheduleDecision := range scheduleDecisions {
					fmt.Printf("[Conductor] scheduler decision type: %s\n", scheduleDecision.MessageType)

					// Ignore non-scheduled messages for now.
					if !scheduleDecision.ToSchedule {
						fmt.Printf("[Conductor] scheduler decision type %s does not request spawn\n", scheduleDecision.MessageType)
						continue
					}

					// Create a new worker spawn config
					newWorkerName := util.GenerateWorkerName()
					newWorkerSpawnConfig, err := prepareWorkerSpawnConfig(workerSpawnConfig{
						ScheduleDecision:        scheduleDecision,
						ConductorGrpcServerAddr: conductorWorkerDialAddress(),
						WorkerID:                newWorkerName,
						RunDir:                  runDir,
					})
					if err != nil {
						fmt.Fprintf(os.Stderr, "prepare worker work files for sqs message %q: %v\n", polledSQSMessage.ExternalMessageID, err)
						continue
					}

					fargateWorkerSpawnRequest := BuildFargateSpawnRequest(newWorkerSpawnConfig)

					spawnFunc := buildFargateWorkerLauncher(fargateWorkerSpawnRequest)

					// Add the new worker to the registry and launch its Fargate task.
					if err := registry.spawnWorker(
						pollLoopContext,
						newWorkerSpawnConfig,
						// ai--done
						spawnFunc,
					); err != nil {
						fmt.Fprintf(os.Stderr, "spawn worker %q for sqs message %q: %v\n", newWorkerSpawnConfig.WorkerID, polledSQSMessage.ExternalMessageID, err)
						continue
					}
				}

			case err, ok := <-pollErrors:
				if !ok {
					pollErrors = nil
					continue
				}
				fmt.Fprintf(os.Stderr, "poll sqs: %v\n", err)
			}
		}
	}(pollingContext, chanPollDone)

	// Safe Shutdown Gate
	//
	if err := shutdownGate(shutdownGateConfig{
		ShutdownRequestPath:       shutdownOkayPath,
		SafeShutdownSucceededPath: filepath.Join(runDir, "CONDUCTOR_READY_FOR_SAFE_SHUTDOWN"),
		StopPolling:               cancelPollingContext,
		PollDone:                  chanPollDone,
		Registry:                  registry,
	}); err != nil {
		return err
	}

	return nil
}
