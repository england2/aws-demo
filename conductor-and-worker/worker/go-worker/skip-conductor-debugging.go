package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	codex "github.com/pmenglund/codex-sdk-go"
	"github.com/pmenglund/codex-sdk-go/protocol"
)

func skipConductor() {
	// ctx is the Go cancellation/deadline context for SDK calls. It is not the
	// Codex conversation context.
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// The client owns the connection to the `codex app-server` process. These
	// settings assume the outer container is the guardrail: no Codex approvals
	// and no Codex sandbox.
	client, err := codex.New(ctx, codex.Options{
		Logger: logger,
		ApprovalHandler: codex.AutoApproveHandler{
			Logger: logger,
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
	defer client.Close()

	// A "thread" is the sdk's notion of a single agent's context. We reuse threads to reuse context.
	thread, err := client.StartThread(ctx, codex.ThreadStartOptions{
		ApprovalPolicy: codex.ApprovalPolicyNever,
		SandboxPolicy:  codex.SandboxModeDangerFullAccess,
	})
	if err != nil {
		panic(err)
	}
	fmt.Printf("Codex thread ID: %s\n\n", thread.ID())

	// ==== main task =============================================

	prompt := "Make a new directory named foo and write a python fizzbuzz inside"
	mainTaskResult, err := thread.Run(ctx, prompt, nil)
	if err != nil {
		panic(err)
	}
	fmt.Println(mainTaskResult.FinalResponse)

	// ==== task 2 ================================================

	endingReportResult, err := thread.Run(ctx, endingReport, nil)
	if err != nil {
		panic(err)
	}

	fmt.Println(endingReportResult.FinalResponse)

	// ==== print full agent transcript ===========================

	// The thread ID is the durable handle you use to map this Go-run agent to
	// Codex's local saved transcript/session history.
	transcript, err := client.Client().ThreadRead(ctx, protocol.ThreadReadParams{
		ThreadID:     thread.ID(),
		IncludeTurns: true,
	})
	if err != nil {
		panic(err)
	}

	fmt.Printf("\n==== persisted transcript for thread %s ====\n", thread.ID())
	printJSON(transcript)
}
