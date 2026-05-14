package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	codex "github.com/pmenglund/codex-sdk-go"
)

func startSkipConductorDebuggingThread(ctx context.Context) (*codex.Codex, *codex.Thread, error) {
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
		return nil, nil, err
	}

	// A "thread" is the sdk's notion of a single agent's context. We reuse threads to reuse context.
	thread, err := client.StartThread(ctx, codex.ThreadStartOptions{
		ApprovalPolicy: codex.ApprovalPolicyNever,
		SandboxPolicy:  codex.SandboxModeDangerFullAccess,
	})
	if err != nil {
		if closeErr := client.Close(); closeErr != nil {
			return nil, nil, fmt.Errorf("start skip-conductor thread: %w; close client: %v", err, closeErr)
		}
		return nil, nil, err
	}
	fmt.Printf("Codex thread ID: %s\n\n", thread.ID())

	return client, thread, nil
}
