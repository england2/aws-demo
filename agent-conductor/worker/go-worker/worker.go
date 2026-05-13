package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"

	sharedproto "conductor-testing/proto"

	_ "embed"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	codex "github.com/pmenglund/codex-sdk-go"
	// "github.com/pmenglund/codex-sdk-go/protocol"
)

// =============================================================
// Set Vars and Embeds
// =============================================================

//go:embed embedded-text/initial-prompt.md
var initialWorkerPrompt string

//go:embed embedded-text/ending-report.md
var endingReport string

var (
	serverAddr        string
	workerID          string
	skipConductorFlag = flag.Bool("skip-conductor", false, "run the worker Codex path without conductor gRPC")
)

func setVars() {
	getEnvVarOrPanic := func(envVarName string) string {
		envVarValue := os.Getenv(envVarName)
		if envVarValue == "" {
			panic(envVarName + " must be set")
		}

		return envVarValue
	}

	serverAddr = getEnvVarOrPanic("CONDUCTOR_GRPC_SERVER_ADDR")
	workerID = getEnvVarOrPanic("WORKER_ID")
}

// =============================================================
// Codex Utilities
// =============================================================

func sendCodexError(
	ctx context.Context,
	conductorClient sharedproto.WorkerEventReceiverServiceClient,
	workerIdentity *sharedproto.WorkerIdentity,
	codexErr error,
) error {
	workerID := workerIdentity.GetWorkerId()
	workerMessage := fmt.Sprintf("[%s]: codex error: %v", workerID, codexErr)
	codexErrorResponse, err := conductorClient.WorkerSendsCodexError(ctx, &sharedproto.CodexError{
		WorkerId:      workerID,
		WorkerMessage: workerMessage,
	})
	if err != nil {
		return fmt.Errorf("send codex error: %w", err)
	}

	fmt.Printf("[internal %s]: conductor codex error response received: %s\n", workerID, codexErrorResponse.GetWorkerMessage())

	return nil
}

func reportCodexErrorAndExit(
	ctx context.Context,
	conductorClient sharedproto.WorkerEventReceiverServiceClient,
	workerIdentity *sharedproto.WorkerIdentity,
	codexErr error,
) {
	if reportErr := sendCodexError(ctx, conductorClient, workerIdentity, codexErr); reportErr != nil {
		log.Fatalf("report codex error: %v; original codex error: %v", reportErr, codexErr)
	}

	log.Fatalf("%v", codexErr)
}

func main() {
	// Skip conductor if flag is passed to run a manual debug test.
	flag.Parse()
	if *skipConductorFlag {
		skipConductor()
		return
	}

	setVars()

	// =============================================================
	// Setup Codex SDK Client
	// =============================================================

	// Note: We MUST establish the Codex Client before we start the gRPC client, or we get a cryptic error:
	//   Error: error loading default config after config error: No such file or directory (os error 2)
	//   panic: EOF

	// codexContext is the Go cancellation/deadline context for SDK calls. It is not the
	// Codex conversation context.
	codexContext := context.Background()
	codexLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// The codexClient owns the connection to the `codex app-server` process. These
	// settings assume the outer container is the guardrail: no Codex approvals
	// and no Codex sandbox.
	codexClient, err := codex.New(codexContext, codex.Options{
		Logger: codexLogger,
		ApprovalHandler: codex.AutoApproveHandler{
			Logger: codexLogger,
		},
		Spawn: codex.SpawnOptions{
			ConfigOverrides: []string{
				`approval_policy="never"`,
				`sandbox_mode="danger-full-access"`,
				`default_permissions=":danger-no-sandbox"`,
			},
		},
	})
	if err != nil {
		panic(err)
	}
	defer codexClient.Close()

	// A "codexThread" is the sdk's notion of a single agent's context. We reuse threads to reuse context.
	codexThread, err := codexClient.StartThread(codexContext, codex.ThreadStartOptions{
		ApprovalPolicy: codex.ApprovalPolicyNever,
		SandboxPolicy:  codex.SandboxModeDangerFullAccess,
	})
	if err != nil {
		panic(err)
	}
	fmt.Printf("Codex thread ID: %s\n\n", codexThread.ID())

	// =============================================================
	// Setup gRPC client, handshake with server, get work files
	// =============================================================

	fmt.Printf("STARTING GRPC SERVER")
	fmt.Printf("[internal %s]: worker process started\n", workerID)
	fmt.Printf("[internal %s]: conductor dial target is %s\n", workerID, serverAddr)

	conn, err := grpc.NewClient(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("dial conductor: %v", err)
	}
	defer conn.Close()

	conductorClient := sharedproto.NewWorkerEventReceiverServiceClient(conn)

	grpcContext := context.Background()
	workerIdentity := &sharedproto.WorkerIdentity{
		WorkerId: workerID,
	}

	handshakeResponse, err := conductorClient.WorkerStartsHandshake(grpcContext, &sharedproto.Handshake{
		Worker:        workerIdentity,
		WorkerMessage: fmt.Sprintf("[%s]: starting handshake", workerID),
	})
	if err != nil {
		log.Fatalf("start worker handshake: %v", err)
	}
	fmt.Printf("[internal %s]: conductor handshake response received: %s\n", workerID, handshakeResponse.GetWorkerMessage())

	if err := requestWorkFiles(grpcContext, conductorClient, workerIdentity); err != nil {
		log.Fatalf("request work files: %v", err)
	}

	// =============================================================
	// Do main agent work
	// =============================================================

	mainTaskResult, err := codexThread.Run(codexContext, initialWorkerPrompt, nil)
	if err != nil {
		panic(err)
	}
	fmt.Println(mainTaskResult.FinalResponse)

	// =============================================================
	// Produce ending report and transcript
	// =============================================================

	endingReportResult, err := codexThread.Run(codexContext, endingReport, nil)
	if err != nil {
		panic(err)
	}

	fmt.Println(endingReportResult.FinalResponse)

	if err := uploadFiles(grpcContext, conductorClient, workerIdentity); err != nil {
		log.Fatalf("upload files: %v", err)
	}

	// =============================================================
	// Send shutdown
	// =============================================================

	// Now that we've finished our work, we can safely shutdown.
	shutdownResponse, err := conductorClient.WorkerStartsShutdown(grpcContext, &sharedproto.Shutdown{
		Worker:        workerIdentity,
		WorkerMessage: fmt.Sprintf("[%s]: safely ended", workerID),
	})
	if err != nil {
		log.Fatalf("start worker shutdown: %v", err)
	}
	fmt.Printf("[internal %s]: conductor shutdown response received: %s\n", workerID, shutdownResponse.GetWorkerMessage())
}
