package handlers

import (
	"context"

	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/api/middleware"
)

// GetUserIDFromContext retrieves the user ID from the context.
func GetUserIDFromContext(ctx context.Context) (string, bool) {
	return middleware.GetUserIDFromContext(ctx)
} 