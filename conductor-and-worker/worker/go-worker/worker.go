package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"

	sharedproto "conductor-testing/proto"

	_ "embed"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	codex "github.com/pmenglund/codex-sdk-go"
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
	workerMessage := fmt.Sprintf("codex error: %v", codexErr)
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
		skipConductorContext := context.Background()
		skipConductorCodexClient, skipConductorThread, err := startSkipConductorDebuggingThread(skipConductorContext)
		if err != nil {
			panic(err)
		}
		defer skipConductorCodexClient.Close()

		prompt := "Make a new directory named foo and write a python fizzbuzz inside"
		mainTaskResult, err := skipConductorThread.Run(skipConductorContext, prompt, nil)
		if err != nil {
			panic(err)
		}
		fmt.Println(mainTaskResult.FinalResponse)

		endingReportResult, err := skipConductorThread.Run(skipConductorContext, endingReport, nil)
		if err != nil {
			panic(err)
		}
		fmt.Println(endingReportResult.FinalResponse)

		transcriptJSON, err := readCodexThreadTranscriptJSON(skipConductorContext, skipConductorCodexClient, skipConductorThread)
		if err != nil {
			panic(err)
		}

		fmt.Printf("\n==== persisted transcript for thread %s ====\n", skipConductorThread.ID())
		fmt.Println(string(transcriptJSON))
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
		WorkerMessage: "starting handshake",
	})
	if err != nil {
		log.Fatalf("start worker handshake: %v", err)
	}
	fmt.Printf("[internal %s]: conductor handshake response received: %s\n", workerID, handshakeResponse.GetWorkerMessage())

	if err := requestWorkFiles(grpcContext, conductorClient, workerIdentity); err != nil {
		log.Fatalf("request work files: %v", err)
	}

	workerRuntimePaths := defaultWorkerRuntimePaths()
	// /worker/work/repo/: contains the single task repository under one variable child directory.
	// /worker/work/agent-meta/: contains worker-agent protocol files, final reports, and GitHub body markdown.
	if err := ensureDirsExist([]string{
		workerRuntimePaths.RepoRootDir,
		workerRuntimePaths.AgentMetaDir,
	}); err != nil {
		log.Fatalf("create worker runtime directories: %v", err)
	}

	// =============================================================
	// Do agent work and validate worker artifacts
	// =============================================================

	mainTaskResult, err := codexThread.Run(codexContext, initialWorkerPrompt, nil)
	if err != nil {
		reportCodexErrorAndExit(grpcContext, conductorClient, workerIdentity, fmt.Errorf("run initial worker prompt: %w", err))
	}
	fmt.Println(mainTaskResult.FinalResponse)

	reportResult, err := codexThread.Run(codexContext, endingReport, nil)
	if err != nil {
		reportCodexErrorAndExit(grpcContext, conductorClient, workerIdentity, fmt.Errorf("run ending report prompt: %w", err))
	}
	fmt.Println(reportResult.FinalResponse)

	var workerCodexRunResult WorkerCodexRunResult
	for validationAttemptNumber := 1; validationAttemptNumber <= maxWorkerArtifactValidationAttempts; validationAttemptNumber++ {
		fmt.Printf("[internal %s]: validating worker artifacts attempt %d/%d\n", workerID, validationAttemptNumber, maxWorkerArtifactValidationAttempts)

		var validationErrors []string
		workerCodexRunResult, validationErrors = validateWorkerCodexArtifacts(workerRuntimePaths)
		if len(validationErrors) == 0 {
			fmt.Printf(
				"[internal %s]: worker artifact validation passed should_create_pull_request=%t repo_path=%q\n",
				workerID,
				workerCodexRunResult.ShouldCreatePullRequest,
				workerCodexRunResult.RepoPath,
			)
			break
		}

		fmt.Printf("[internal %s]: worker artifact validation failed: %s\n", workerID, strings.Join(validationErrors, "; "))

		if validationAttemptNumber == maxWorkerArtifactValidationAttempts {
			reportCodexErrorAndExit(grpcContext, conductorClient, workerIdentity, buildWorkerArtifactValidationFailureError(validationErrors))
		}

		fmt.Printf("[internal %s]: running artifact correction prompt\n", workerID)
		correctionPrompt := buildWorkerArtifactCorrectionPrompt(workerRuntimePaths, validationErrors, validationAttemptNumber+1, maxWorkerArtifactValidationAttempts)
		correctionResult, err := codexThread.Run(codexContext, correctionPrompt, nil)
		if err != nil {
			reportCodexErrorAndExit(grpcContext, conductorClient, workerIdentity, fmt.Errorf("run worker artifact correction prompt: %w", err))
		}
		fmt.Println(correctionResult.FinalResponse)
	}

	// =============================================================
	// Produce GitHub report and create a PR or failure issue
	// =============================================================

	fmt.Printf("[internal %s]: reading Codex transcript for GitHub report\n", workerID)
	transcriptJSON, err := readCodexThreadTranscriptJSON(codexContext, codexClient, codexThread)
	if err != nil {
		reportCodexErrorAndExit(grpcContext, conductorClient, workerIdentity, err)
	}

	fmt.Printf("[internal %s]: writing GitHub report markdown\n", workerID)
	gitHubReportMarkdownResult, err := writeGitHubReportMarkdown(workerRuntimePaths, transcriptJSON)
	if err != nil {
		reportCodexErrorAndExit(grpcContext, conductorClient, workerIdentity, err)
	}
	fmt.Printf("[internal %s]: GitHub report markdown written to %s title=%q\n", workerID, gitHubReportMarkdownResult.Path, gitHubReportMarkdownResult.Title)

	gitHubPublicationURL := ""
	if workerCodexRunResult.ShouldCreatePullRequest {
		fmt.Printf("[internal %s]: worker succeeded; creating GitHub pull request from %s\n", workerID, workerCodexRunResult.RepoPath)
		pullRequestCreationResult, err := createPullRequestFromWorkerRepo(codexContext, workerCodexRunResult.RepoPath, gitHubReportMarkdownResult.Path, gitHubReportMarkdownResult.Title, workerID)
		if err != nil {
			reportCodexErrorAndExit(grpcContext, conductorClient, workerIdentity, err)
		}
		fmt.Printf("[internal %s]: created pull request from branch %s:\n%s\n", workerID, pullRequestCreationResult.BranchName, pullRequestCreationResult.Output)
		gitHubPublicationURL = pullRequestCreationResult.URL
	} else if repoPath, repoAvailable := findOptionalWorkerGitRepo(workerRuntimePaths); repoAvailable {
		fmt.Printf("[internal %s]: worker did not request PR; creating failed-worker GitHub issue from %s\n", workerID, repoPath)
		gitHubIssueCreationResult, err := createFailedWorkerGitHubIssue(codexContext, repoPath, gitHubReportMarkdownResult.Path, gitHubReportMarkdownResult.Title, workerID)
		if err != nil {
			reportCodexErrorAndExit(grpcContext, conductorClient, workerIdentity, err)
		}
		fmt.Printf("[internal %s]: created failed-worker GitHub issue:\n%s\n", workerID, gitHubIssueCreationResult.Output)
		gitHubPublicationURL = gitHubIssueCreationResult.URL
	} else {
		fmt.Printf("[internal %s]: worker did not request PR and no Git repo was available; skipping GitHub publication\n", workerID)
	}

	workerShutdownMessage := "safely ended"
	if gitHubPublicationURL != "" {
		if err := writeGitHubLinkFile(workerRuntimePaths, gitHubPublicationURL); err != nil {
			reportCodexErrorAndExit(grpcContext, conductorClient, workerIdentity, err)
		}
		workerShutdownMessage = fmt.Sprintf("safely ended; github link: %s", gitHubPublicationURL)
	}

	if err := uploadFiles(grpcContext, conductorClient, workerIdentity); err != nil {
		log.Fatalf("upload files: %v", err)
	}

	// =============================================================
	// Send shutdown
	// =============================================================

	// Now that we've finished our work, we can safely shutdown.
	shutdownResponse, err := conductorClient.WorkerStartsShutdown(grpcContext, &sharedproto.Shutdown{
		Worker:        workerIdentity,
		WorkerMessage: workerShutdownMessage,
	})
	if err != nil {
		log.Fatalf("start worker shutdown: %v", err)
	}
	fmt.Printf("[internal %s]: conductor shutdown response received: %s\n", workerID, shutdownResponse.GetWorkerMessage())
}
