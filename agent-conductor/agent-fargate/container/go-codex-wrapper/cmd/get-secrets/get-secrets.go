package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

const defaultRegion = "us-west-2"

func print_secret(region string, secretName string) {
	value, err := readSecretValue(context.Background(), region, secretName, "")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(value)
}

func main() {
	region := flag.String("region", defaultRegion, "AWS region")
	field := flag.String("field", "", "optional JSON field to print from the secret value")
	flag.Parse()

	args := flag.Args()
	if len(args) != 1 {
		log.Fatal("usage: get-secrets <secret-name>")
	}
	secretName := args[0]

	value, err := readSecretValue(context.Background(), *region, secretName, *field)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(value)
}

func readSecretValue(ctx context.Context, region string, secretName string, field string) (string, error) {
	secretString, err := getSecretString(ctx, region, secretName)
	if err != nil {
		return "", err
	}

	value := strings.TrimSpace(secretString)
	if field != "" {
		value, err = getSecretJSONField(secretString, field)
		if err != nil {
			return "", err
		}
	}

	if value == "" {
		return "", fmt.Errorf("secret value is empty")
	}

	return value, nil
}

func getSecretString(ctx context.Context, region string, secretName string) (string, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return "", fmt.Errorf("load AWS config: %w", err)
	}

	client := secretsmanager.NewFromConfig(cfg)
	result, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId:     aws.String(secretName),
		VersionStage: aws.String("AWSCURRENT"),
	})
	if err != nil {
		return "", fmt.Errorf("get secret value: %w", err)
	}
	if result.SecretString == nil {
		return "", fmt.Errorf("secret %q has no string value", secretName)
	}

	return *result.SecretString, nil
}

func getSecretJSONField(secretString string, field string) (string, error) {
	var secret map[string]string
	if err := json.Unmarshal([]byte(secretString), &secret); err != nil {
		return "", fmt.Errorf("parse secret JSON: %w", err)
	}

	value, ok := secret[field]
	if !ok {
		return "", fmt.Errorf("secret JSON field %q not found", field)
	}

	return strings.TrimSpace(value), nil
}
