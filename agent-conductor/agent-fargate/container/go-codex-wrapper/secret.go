package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

func get_openai_key() string {
	secretName := "openai-key-aws-demo-agent-fargate"
	region := "us-west-2"

	config, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	if err != nil {
		log.Fatal(err)
	}

	// Create Secrets Manager client
	svc := secretsmanager.NewFromConfig(config)

	input := &secretsmanager.GetSecretValueInput{
		SecretId:     aws.String(secretName),
		VersionStage: aws.String("AWSCURRENT"), // VersionStage defaults to AWSCURRENT if unspecified
	}

	result, err := svc.GetSecretValue(context.TODO(), input)
	if err != nil {
		// For a list of exceptions thrown, see
		// https://docs.aws.amazon.com/secretsmanager/latest/apireference/API_GetSecretValue.html
		log.Fatal(err.Error())
	}

	// Decrypts secret using the associated KMS key.
	return *result.SecretString
}

func codex_login(openaiAPIKey string) error {
	openaiAPIKey = strings.TrimSpace(openaiAPIKey)
	if openaiAPIKey == "" {
		return fmt.Errorf("OPENAI API key is empty")
	}

	cmd := exec.Command("codex", "login", "--with-api-key")
	cmd.Stdin = strings.NewReader(openaiAPIKey + "\n")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "OPENAI_API_KEY="+openaiAPIKey)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("codex login: %w", err)
	}

	return nil
}
