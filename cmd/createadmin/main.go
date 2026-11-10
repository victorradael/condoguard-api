// Command createadmin creates the first manager of a condominium.
//
// Public sign-up (POST /auth/register) only creates residents, and only a
// síndico or administradora can create other users (POST /users), so the first
// one of a condominium is created with this command:
//
//	ADMIN_EMAIL=sindico@exemplo.com ADMIN_USERNAME=sindico \
//	  go run ./cmd/createadmin < senha.txt
//
// The password is read from the first line of stdin (or ADMIN_PASSWORD) and
// must meet the password policy. It reads DATABASE_URL and
// DEFAULT_CONDOMINIUM_ID (an id or a name; default "default"); migrations must
// be applied. When no condominium matches, one with that name is created
// (under the administradora named in ADMINISTRATOR_NAME, if set). ADMIN_ROLE
// is sindico (default) or administradora.
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/internal/condominium"
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

	role := os.Getenv("ADMIN_ROLE")
	if role == "" {
		role = authz.RoleSindico
	}
	if role != authz.RoleSindico && role != authz.RoleAdministradora {
		return fmt.Errorf("ADMIN_ROLE must be %s or %s", authz.RoleSindico, authz.RoleAdministradora)
	}

	condos := condominium.NewPostgresRepository(pool)
	id, found, err := condos.Lookup(ctx, condoID)
	if err != nil {
		return err
	}
	if !found {
		c, cerr := condos.Create(ctx, condoID, os.Getenv("ADMINISTRATOR_NAME"))
		if cerr != nil {
			return cerr
		}
		id = c.ID
		fmt.Printf("condominium %q created (id %s)\n", c.Name, c.ID)
	}

	ctx = authz.AsOperator(tenant.WithCondominiumID(ctx, id))
	u, err := user.NewService(user.NewPostgresRepository(pool)).Create(ctx, user.CreateRequest{
		Username: username,
		Email:    email,
		Password: pw,
		Roles:    []string{role},
	})
	if err != nil {
		return err
	}
	fmt.Printf("%s %s created in condominium %s (id %s)\n", role, u.Email, id, u.ID)
	return nil
}
