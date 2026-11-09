package account

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/victorradael/condoguard/api/internal/db"
)

// postgresStore talks to the SECURITY DEFINER functions of migration 00004:
// the tokens table is closed to the API role, and none of these steps has a
// condominium or a user in the session yet.
type postgresStore struct {
	pool *pgxpool.Pool
	scp  *db.Scoped
}

// NewPostgresStore returns a Store backed by the shared pgx pool.
func NewPostgresStore(pool *pgxpool.Pool) Store {
	return &postgresStore{pool: pool, scp: db.NewScoped(pool)}
}

func (s *postgresStore) FindByEmail(ctx context.Context, email string) (Account, bool, error) {
	var (
		id       uuid.UUID
		username string
		mail     string
		verified *time.Time
	)
	err := s.pool.QueryRow(ctx, `SELECT id, username, email, email_verified_at FROM auth_user_by_email($1)`, email).Scan(&id, &username, &mail, &verified)
	if db.IsNoRows(err) {
		return Account{}, false, nil
	}
	if err != nil {
		return Account{}, false, err
	}
	return Account{ID: id.String(), Email: mail, Username: username, Verified: verified != nil}, true, nil
}

func (s *postgresStore) Issue(ctx context.Context, userID, purpose string, hash []byte, expires time.Time) error {
	uid, ok := db.ParseID(userID)
	if !ok {
		return ErrInvalidToken
	}
	_, err := s.pool.Exec(ctx, `SELECT account_token_issue($1, $2, $3, $4)`, uid, purpose, hash, expires)
	return err
}

func (s *postgresStore) IssueForMember(ctx context.Context, userID, purpose string, hash []byte, expires time.Time, issuerID string) (bool, error) {
	uid, ok := db.ParseID(userID)
	if !ok {
		return false, nil
	}
	var issuer *uuid.UUID
	if iid, ok := db.ParseID(issuerID); ok {
		issuer = &iid
	}
	var done bool
	// The function reads the condominium from the transaction setting.
	err := s.scp.QueryRow(ctx, `SELECT account_token_issue_for_member($1, $2, $3, $4, $5)`, uid, purpose, hash, expires, issuer).Scan(&done)
	return done, err
}

func (s *postgresStore) Peek(ctx context.Context, hash []byte, purpose string) (Peek, error) {
	var (
		id          uuid.UUID
		email, name string
	)
	err := s.pool.QueryRow(ctx, `SELECT user_id, email, username FROM account_token_peek($1, $2)`, hash, purpose).Scan(&id, &email, &name)
	if db.IsNoRows(err) {
		return Peek{}, ErrInvalidToken
	}
	if err != nil {
		return Peek{}, err
	}
	return Peek{UserID: id.String(), Email: email, Username: name}, nil
}

func (s *postgresStore) VerifyEmail(ctx context.Context, hash []byte) (string, error) {
	var id *uuid.UUID
	if err := s.pool.QueryRow(ctx, `SELECT account_verify_email($1)`, hash).Scan(&id); err != nil {
		return "", err
	}
	if id == nil {
		return "", ErrInvalidToken
	}
	return id.String(), nil
}

func (s *postgresStore) ResetPassword(ctx context.Context, hash []byte, passwordHash string) (string, error) {
	var id *uuid.UUID
	if err := s.pool.QueryRow(ctx, `SELECT account_reset_password($1, $2)`, hash, passwordHash).Scan(&id); err != nil {
		return "", err
	}
	if id == nil {
		return "", ErrInvalidToken
	}
	return id.String(), nil
}

func (s *postgresStore) RevokeSessions(ctx context.Context, userID string) error {
	uid, ok := db.ParseID(userID)
	if !ok {
		return nil
	}
	_, err := s.scp.Exec(ctx, `SELECT revoke_user_sessions($1)`, uid)
	return err
}
