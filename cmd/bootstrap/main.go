package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/quotaforge/quotaforge/internal/repository/postgres"
	"github.com/quotaforge/quotaforge/internal/service"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://quotaforge:quotaforge@localhost:5432/quotaforge?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	credRepo := postgres.NewCredentialRepo(pool)
	auditRepo := postgres.NewAuditRepo(pool)
	credSvc := service.NewCredentialService(credRepo, auditRepo, 1, 64*1024, 4, 32)

	tenantID := uuid.New()
	_, err = pool.Exec(ctx, "INSERT INTO tenants (id, name) VALUES ($1, $2)", tenantID, "Bootstrap Tenant")
	if err != nil {
		log.Fatalf("failed to insert tenant: %v", err)
	}

	cred, rawKey, err := credSvc.Create(ctx, tenantID, "Bootstrap Key")
	if err != nil {
		log.Fatalf("failed to create credential: %v", err)
	}

	fmt.Println("==================================================")
	fmt.Println("Bootstrap Successful!")
	fmt.Printf("Tenant ID: %s\n", tenantID)
	fmt.Printf("Key ID   : %s\n", cred.ID)
	fmt.Printf("Raw Key  : %s\n", rawKey)
	fmt.Println("==================================================")
	fmt.Println("Use this Raw Key as your Bearer token to authenticate.")
	fmt.Println("Example: -H \"Authorization: Bearer " + rawKey + "\"")
}
