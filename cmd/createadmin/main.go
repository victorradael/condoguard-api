// Command createadmin creates an administrator (ROLE_ADMIN) account.
//
// Public sign-up (POST /auth/register) only creates residents, and only an
// admin can create other admins (POST /users), so the first admin of a
// condominium is created with this command:
//
//	ADMIN_EMAIL=sindico@exemplo.com ADMIN_USERNAME=sindico \
//	  go run ./cmd/createadmin < senha.txt
//
// The password is read from the first line of stdin (or ADMIN_PASSWORD) and
// must meet the password policy. It reads DATABASE_URL and
// DEFAULT_CONDOMINIUM_ID (default "default"); migrations must be applied.
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/internal/db"
	"github.com/victorradael/condoguard/api/internal/tenant"
	"github.com/victorradael/condoguard/api/internal/user"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "createadmin:", err)
		os.Exit(1)
	}
}

func run() error {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return fmt.Errorf("DATABASE_URL not set")
	}
	condoID := os.Getenv("DEFAULT_CONDOMINIUM_ID")
	if condoID == "" {
		condoID = "default"
	}
	email, username := os.Getenv("ADMIN_EMAIL"), os.Getenv("ADMIN_USERNAME")
	if email == "" || username == "" {
		return fmt.Errorf("set ADMIN_EMAIL and ADMIN_USERNAME")
	}
	pw := os.Getenv("ADMIN_PASSWORD")
	if pw == "" {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return fmt.Errorf("read password from stdin: %w", err)
		}
		pw = strings.TrimRight(line, "\r\n")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool, err := db.NewPool(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()

	ctx = tenant.WithCondominiumID(ctx, condoID)
	u, err := user.NewService(user.NewPostgresRepository(pool)).Create(ctx, user.CreateRequest{
		Username: username,
		Email:    email,
		Password: pw,
		Roles:    []string{authz.RoleAdmin},
	})
	if err != nil {
		return err
	}
	fmt.Printf("admin %s created in condominium %q (id %s)\n", u.Email, condoID, u.ID)
	return nil
}
