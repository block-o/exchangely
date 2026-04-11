package auth

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// BenchmarkCreateToken measures token generation throughput: crypto random bytes,
// SHA-256 hashing, and mock repository insert.
func BenchmarkCreateToken(b *testing.B) {
	tokenRepo := newMockAPITokenRepo()
	userRepo := newMockUserRepo()
	cfg := DefaultAPITokenConfig()
	// Allow unlimited tokens so the benchmark doesn't hit the cap.
	cfg.MaxTokensPerUser = b.N + 1
	svc := NewAPITokenService(tokenRepo, userRepo, cfg)

	userID := uuid.New()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := svc.CreateToken(ctx, userID, "bench-token")
		if err != nil {
			b.Fatalf("CreateToken failed: %v", err)
		}
	}
}

// BenchmarkValidateToken measures token validation throughput: SHA-256 hashing
// of the raw token and mock repository lookup.
func BenchmarkValidateToken(b *testing.B) {
	tokenRepo := newMockAPITokenRepo()
	userRepo := newMockUserRepo()
	cfg := DefaultAPITokenConfig()
	svc := NewAPITokenService(tokenRepo, userRepo, cfg)

	userID := uuid.New()
	ctx := context.Background()

	// Seed a user so ValidateToken can resolve the owner.
	_ = userRepo.Create(ctx, &User{
		ID:    userID,
		Email: "bench@example.com",
		Name:  "Bench User",
		Role:  "user",
	})

	// Pre-create a token to benchmark the validation path.
	rawToken, _, err := svc.CreateToken(ctx, userID, "bench-token")
	if err != nil {
		b.Fatalf("setup CreateToken failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := svc.ValidateToken(ctx, rawToken)
		if err != nil {
			b.Fatalf("ValidateToken failed: %v", err)
		}
	}
}

// BenchmarkValidateToken_Parallel verifies thread safety of ValidateToken under
// concurrent load using b.RunParallel.
func BenchmarkValidateToken_Parallel(b *testing.B) {
	tokenRepo := newMockAPITokenRepo()
	userRepo := newMockUserRepo()
	cfg := DefaultAPITokenConfig()
	svc := NewAPITokenService(tokenRepo, userRepo, cfg)

	userID := uuid.New()
	ctx := context.Background()

	// Seed a user so ValidateToken can resolve the owner.
	_ = userRepo.Create(ctx, &User{
		ID:    userID,
		Email: "bench-parallel@example.com",
		Name:  "Bench Parallel User",
		Role:  "user",
	})

	// Pre-create a token to benchmark the validation path.
	rawToken, _, err := svc.CreateToken(ctx, userID, "bench-parallel-token")
	if err != nil {
		b.Fatalf("setup CreateToken failed: %v", err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, err := svc.ValidateToken(ctx, rawToken)
			if err != nil {
				b.Errorf("ValidateToken failed: %v", err)
			}
		}
	})
}
