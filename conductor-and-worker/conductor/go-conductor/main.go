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
	// =============================================================
	// Runtime path, Files, and Args
	// =============================================================

	// build runtime and shutdown files
	runDir := filepath.Join(conductorRootPath, conductorRunDirName)
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

	fmt.Printf("in main_testing\n\n")

	flag.Parse()

	// =============================================================
	// Database Setup and Polling Setup
	// =============================================================

	schedulerDatabasePath := testSchedulerDatabasePathFromFlags()
	if schedulerDatabasePath == "" {
		log.Fatal("test-db-loc is required")
	}

	dbContext := context.Background()

	if *debugAlwaysNewDB {
		createdDatabasePath, err := debugCreateNewDbAndSetLocation(dbContext, schedulerDatabasePath)
		if err != nil {
			log.Fatalf("create debug scheduler database: %v", err)
		}
		schedulerDatabasePath = createdDatabasePath
	} else if !checkIsDbCompliant(dbContext, schedulerDatabasePath) {
		log.Fatalf("database is not compliant: %s", schedulerDatabasePath)
	}

	schedulerWorker, err := scheduler.Open(dbContext, scheduler.Config{
		DBPath: schedulerDatabasePath,
	})
	if err != nil {
		log.Fatalf("open scheduler: %v", err)
	}
	defer schedulerWorker.Close()

	pollingContext, cancelPollingContext := context.WithCancel(context.Background())
	defer cancelPollingContext()

	sqsPoller, err := NewTicketCloudWatchSQSPoller(pollingContext)
	if err != nil {
		log.Fatalf("create sqs poller: %v", err)
	}

	// =============================================================
	// gRPC server setup
	// =============================================================

	fmt.Printf("conductor listening address: %s\n", *serverAddr)
	fmt.Printf("worker dial address: %s\n", conductorWorkerDialAddress())
	fmt.Printf("conductor run directory: %s\n", runDir)

	listener, err := net.Listen("tcp", *serverAddr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

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
						log.Fatalf("prepare worker work files: %v", err)
					}

					fargateWorkerSpawnRequest := BuildFargateSpawnRequest(
						newWorkerSpawnConfig,
						buildAdhocFargateInfrastructureConfig(),
					)
					fmt.Printf(
						"[Conductor] spawning fargate worker %s for %s\n",
						newWorkerSpawnConfig.WorkerID,
						scheduleDecision.MessageType,
					)

					// Defining our spawn function.
					spawnFunc := func(spawnContext context.Context, launchedWorkerConfig workerSpawnConfig) error {
						spawnResult, err := Spawn(spawnContext, fargateWorkerSpawnRequest)
						if err != nil {
							return err
						}
						fmt.Printf(
							"[Conductor] spawned fargate worker %s task=%s\n",
							launchedWorkerConfig.WorkerID,
							spawnResult.TaskARN,
						)
						return nil
					}

					// Add the new worker to the registry and launch its Fargate task.
					if err := registry.spawnWorker(
						pollLoopContext,
						newWorkerSpawnConfig,
						spawnFunc,
					); err != nil {
						log.Fatalf("spawn worker: %v", err)
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

	// ============================================================
	// Safe Shutdown Gate
	// ============================================================

	var numActiveWorkers int
	pollStopSignaled := false
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
			if !pollStopSignaled {
				close(chanPollDone)
				cancelPollingContext()
				pollStopSignaled = true
			}
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
