package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

// DatabaseSecretは、データベース接続に必要な情報を保持する構造体です。
type DatabaseSecret struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	DBName   string `json:"dbname"`
	Engine   string `json:"engine"`
}

// JWTSecretは、JWT署名に必要なシークレットを保持する構造体です。
type JWTSecret struct {
	Secret string `json:"secret"`
}

// GitHubOAuthSecretは、GitHub OAuthの認証情報を保持する構造体です。
type GitHubOAuthSecret struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// SecretsManagerClientはAWS Secrets Managerへのアクセスを提供する構造体です。
type SecretsManagerClient struct {
	client *secretsmanager.Client
}

// NewSecretsManagerClientは新しいSecrets Managerクライアントを作成します。
func NewSecretsManagerClient(ctx context.Context) (*SecretsManagerClient, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to load AWS configuration: %w", err)
	}

	return &SecretsManagerClient{
		client: secretsmanager.NewFromConfig(cfg),
	}, nil
}

// Secrets Managerクライアントを閉じます。
func (smc *SecretsManagerClient) Close() {
	smc.client = nil
}

// データベースの接続情報をSecrets Managerから取得します。
func (smc *SecretsManagerClient) GetDatabaseSecret(ctx context.Context, secretName string) (*DatabaseSecret, error) {
	result, err := smc.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretName),
	})
	if err != nil {
		log.Printf("Error retrieving database secret: %v", err)
		return nil, fmt.Errorf("failed to retrieve database secret: %w", err)
	}

	var secret DatabaseSecret
	if err := json.Unmarshal([]byte(*result.SecretString), &secret); err != nil {
		return nil, fmt.Errorf("failed to unmarshal database secret: %w", err)
	}

	return &secret, nil
}

// secret managerからJWT署名用のシークレットを取得します。
func (smc *SecretsManagerClient) GetJWTSecret(ctx context.Context, secretName string) (*JWTSecret, error) {
	result, err := smc.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretName),
	})
	if err != nil {
		log.Printf("Error retrieving JWT secret: %v", err)
		return nil, fmt.Errorf("failed to retrieve JWT secret: %w", err)
	}

	var secret JWTSecret
	if err := json.Unmarshal([]byte(*result.SecretString), &secret); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JWT secret: %w", err)
	}

	return &secret, nil
}

// Secret ManagerからGitHub OAuthの認証情報を取得します。
func (smc *SecretsManagerClient) GetGitHubOAuthSecret(ctx context.Context, secretName string) (*GitHubOAuthSecret, error) {
	result, err := smc.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretName),
	})
	if err != nil {
		log.Printf("Error retrieving GitHub OAuth secret: %v", err)
		return nil, fmt.Errorf("failed to retrieve GitHub OAuth secret: %w", err)
	}

	var secret GitHubOAuthSecret
	if err := json.Unmarshal([]byte(*result.SecretString), &secret); err != nil {
		return nil, fmt.Errorf("failed to unmarshal GitHub OAuth secret: %w", err)
	}

	return &secret, nil
}
