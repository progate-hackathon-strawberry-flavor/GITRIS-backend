package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

// User represents a user in the system
type User struct {
	ID                uuid.UUID
	CreatedAt         time.Time
	DisplayName       string
	GitHubID          string
	IconURL           string
	UserName          string
	GithubAccessToken string
}

// UserRepository provides database operations for users
type UserRepository struct {
	db *sql.DB
}

// NewUserRepository creates a new UserRepository
func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// GetUserByGitHubID retrieves a user by GitHub ID
func (ur *UserRepository) GetUserByGitHubID(ctx context.Context, githubID string) (*User, error) {
	user := &User{}

	err := ur.db.QueryRowContext(ctx,
		`SELECT id, created_at, display_name, github_id, icon_url, user_name, github_access_token 
		 FROM users WHERE github_id = $1`,
		githubID,
	).Scan(&user.ID, &user.CreatedAt, &user.DisplayName, &user.GitHubID, &user.IconURL, &user.UserName, &user.GithubAccessToken)

	if err == sql.ErrNoRows {
		return nil, nil // User not found
	}
	if err != nil {
		log.Printf("Error querying user by GitHub ID: %v", err)
		return nil, fmt.Errorf("failed to query user: %w", err)
	}

	return user, nil
}

// CreateUser creates a new user
func (ur *UserRepository) CreateUser(ctx context.Context, user *User) (*User, error) {
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}

	err := ur.db.QueryRowContext(ctx,
		`INSERT INTO users (id, display_name, github_id, icon_url, user_name, github_access_token)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, created_at, display_name, github_id, icon_url, user_name, github_access_token`,
		user.ID,
		user.DisplayName,
		user.GitHubID,
		user.IconURL,
		user.UserName,
		user.GithubAccessToken,
	).Scan(&user.ID, &user.CreatedAt, &user.DisplayName, &user.GitHubID, &user.IconURL, &user.UserName, &user.GithubAccessToken)

	if err != nil {
		log.Printf("Error creating user: %v", err)
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	log.Printf("Created new user: %s (GitHub ID: %s)", user.ID, user.GitHubID)
	return user, nil
}

// UpdateUser updates an existing user
func (ur *UserRepository) UpdateUser(ctx context.Context, user *User) (*User, error) {
	err := ur.db.QueryRowContext(ctx,
		`UPDATE users 
		 SET display_name = $1, icon_url = $2, user_name = $3, github_access_token = $4
		 WHERE id = $5
		 RETURNING id, created_at, display_name, github_id, icon_url, user_name, github_access_token`,
		user.DisplayName,
		user.IconURL,
		user.UserName,
		user.GithubAccessToken,
		user.ID,
	).Scan(&user.ID, &user.CreatedAt, &user.DisplayName, &user.GitHubID, &user.IconURL, &user.UserName, &user.GithubAccessToken)

	if err != nil {
		log.Printf("Error updating user: %v", err)
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	log.Printf("Updated user: %s", user.ID)
	return user, nil
}

// GetUserByID retrieves a user by UUID
func (ur *UserRepository) GetUserByID(ctx context.Context, id uuid.UUID) (*User, error) {
	user := &User{}

	err := ur.db.QueryRowContext(ctx,
		`SELECT id, created_at, display_name, github_id, icon_url, user_name, github_access_token 
		 FROM users WHERE id = $1`,
		id,
	).Scan(&user.ID, &user.CreatedAt, &user.DisplayName, &user.GitHubID, &user.IconURL, &user.UserName, &user.GithubAccessToken)

	if err == sql.ErrNoRows {
		return nil, nil // User not found
	}
	if err != nil {
		log.Printf("Error querying user by ID: %v", err)
		return nil, fmt.Errorf("failed to query user: %w", err)
	}

	return user, nil
}
