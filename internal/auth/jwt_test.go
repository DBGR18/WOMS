package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/d11nn/woms/internal/domain"
)

func TestCreateAndVerifyToken(t *testing.T) {
	token, err := CreateToken("secret", Claims{
		Subject: "user-1",
		Role:    domain.RoleScheduler,
		LineID:  "A",
	}, time.Hour)
	if err != nil {
		t.Fatalf("CreateToken returned error: %v", err)
	}

	claims, err := VerifyToken("secret", token)
	if err != nil {
		t.Fatalf("VerifyToken returned error: %v", err)
	}
	if claims.Subject != "user-1" || claims.Role != domain.RoleScheduler || claims.LineID != "A" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestVerifyTokenRejectsTampering(t *testing.T) {
	token, err := CreateToken("secret", Claims{Subject: "user-1", Role: domain.RoleSales}, time.Hour)
	if err != nil {
		t.Fatalf("CreateToken returned error: %v", err)
	}

	_, err = VerifyToken("other-secret", token)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestVerifyTokenRejectsExpiredToken(t *testing.T) {
	token, err := CreateToken("secret", Claims{Subject: "user-1", Role: domain.RoleSales}, -time.Hour)
	if err != nil {
		t.Fatalf("CreateToken returned error: %v", err)
	}

	_, err = VerifyToken("secret", token)
	if !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("expected ErrExpiredToken, got %v", err)
	}
}
