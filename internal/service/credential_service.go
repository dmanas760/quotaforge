package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"github.com/google/uuid"
	"github.com/quotaforge/quotaforge/internal/domain"
	"golang.org/x/crypto/argon2"
)

// credentialVerifier is implemented by CredentialService.
type credentialVerifier interface {
	GetByPrefix(ctx context.Context, keyPrefix string) (*domain.Credential, error)
	VerifyKey(rawKey string, hash string) bool
}

// CredentialService manages API key lifecycle.
type CredentialService struct {
	repo      domain.CredentialRepository
	audit     domain.AuditRepository
	argonTime uint32
	argonMem  uint32
	argonThr  uint8
	argonKey  uint32
	salt      []byte // fixed per-instance salt (not per-key; per-key salt is embedded in hash)
}

// NewCredentialService creates a CredentialService.
func NewCredentialService(repo domain.CredentialRepository, audit domain.AuditRepository,
	argonTime, argonMem uint32, argonThreads uint8, argonKeyLen uint32) *CredentialService {
	return &CredentialService{
		repo:      repo,
		audit:     audit,
		argonTime: argonTime,
		argonMem:  argonMem,
		argonThr:  argonThreads,
		argonKey:  argonKeyLen,
	}
}

// GetByPrefix proxies to the repository for auth middleware.
func (s *CredentialService) GetByPrefix(ctx context.Context, keyPrefix string) (*domain.Credential, error) {
	return s.repo.GetByPrefix(ctx, keyPrefix)
}

// VerifyKey compares a raw key against an Argon2id hash string.
// Hash format: "argon2id$salt_b64$hash_b64"
func (s *CredentialService) VerifyKey(rawKey, hashStr string) bool {
	salt, storedHash, err := parseHash(hashStr)
	if err != nil {
		return false
	}
	computed := argon2.IDKey([]byte(rawKey), salt, s.argonTime, s.argonMem, s.argonThr, s.argonKey)
	return constantTimeEqual(computed, storedHash)
}

// Create generates a new raw API key, hashes it, and persists the credential.
// Returns the credential record and the raw key (shown only once).
func (s *CredentialService) Create(ctx context.Context, tenantID uuid.UUID, _ string) (*domain.Credential, string, error) {
	rawKey, err := generateRawKey()
	if err != nil {
		return nil, "", fmt.Errorf("generating raw key: %w", err)
	}
	prefix := rawKey[:8]
	salt, hash, err := s.hashKey(rawKey)
	if err != nil {
		return nil, "", fmt.Errorf("hashing key: %w", err)
	}

	cred := &domain.Credential{
		TenantID:  tenantID,
		KeyPrefix: prefix,
		KeyHash:   encodeHash(salt, hash),
		Status:    domain.CredentialStatusActive,
	}
	if err := s.repo.Create(ctx, cred); err != nil {
		return nil, "", err
	}
	return cred, rawKey, nil
}

// List returns credentials for a tenant.
func (s *CredentialService) List(ctx context.Context, tenantID uuid.UUID) ([]*domain.Credential, error) {
	return s.repo.List(ctx, tenantID)
}

// Rotate revokes the old key and creates a new one atomically.
func (s *CredentialService) Rotate(ctx context.Context, tenantID, id uuid.UUID) (*domain.Credential, string, error) {
	if err := s.repo.Revoke(ctx, tenantID, id); err != nil {
		return nil, "", err
	}
	return s.Create(ctx, tenantID, "")
}

// Revoke marks a credential as revoked.
func (s *CredentialService) Revoke(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.repo.Revoke(ctx, tenantID, id)
}

// --- helpers ----------------------------------------------------------------

func generateRawKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil // 64-char hex string
}

func (s *CredentialService) hashKey(rawKey string) (salt []byte, hash []byte, err error) {
	salt = make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, nil, err
	}
	hash = argon2.IDKey([]byte(rawKey), salt, s.argonTime, s.argonMem, s.argonThr, s.argonKey)
	return salt, hash, nil
}

func encodeHash(salt, hash []byte) string {
	return "argon2id$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(hash)
}

func parseHash(h string) (salt, hash []byte, err error) {
	// format: argon2id$<salt_b64>$<hash_b64>
	if len(h) < 10 {
		return nil, nil, fmt.Errorf("invalid hash format")
	}
	// find '$' separators
	start := 9 // len("argon2id$")
	mid := -1
	for i := start; i < len(h); i++ {
		if h[i] == '$' {
			mid = i
			break
		}
	}
	if mid == -1 {
		return nil, nil, fmt.Errorf("invalid hash format: missing separator")
	}
	salt, err = base64.RawStdEncoding.DecodeString(h[start:mid])
	if err != nil {
		return nil, nil, err
	}
	hash, err = base64.RawStdEncoding.DecodeString(h[mid+1:])
	return salt, hash, err
}

func constantTimeEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
