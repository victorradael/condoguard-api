package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/victorradael/condoguard/api/internal/db"
	"github.com/victorradael/condoguard/api/internal/db/dbgen"
	"github.com/victorradael/condoguard/api/internal/tenant"
)

type postgresRepository struct {
	pool *pgxpool.Pool
	q    *dbgen.Queries
}

// NewPostgresRepository returns a Repository backed by the shared pgx pool.
// Per-condominium queries are scoped by the condominium ID carried in the
// context; the lookups that happen before login go through the restricted
// SQL functions of migration 00003.
func NewPostgresRepository(pool *pgxpool.Pool) Repository {
	return &postgresRepository{pool: pool, q: db.Queries(pool)}
}

func (r *postgresRepository) Save(ctx context.Context, user *User) error {
	condoID, err := tenant.UUID(ctx)
	if err != nil {
		return err
	}
	roles := user.Roles
	if roles == nil {
		roles = []string{}
	}
	id := uuid.New()
	err = db.InTx(ctx, r.pool, func(ctx context.Context) error {
		if err := r.q.InsertUser(ctx, dbgen.InsertUserParams{ID: id, Username: user.Username, Email: user.Email, PasswordHash: user.Password}); err != nil {
			return err
		}
		return r.q.InsertMembership(ctx, dbgen.InsertMembershipParams{UserID: id, CondominiumID: condoID, Roles: roles})
	})
	if db.IsUniqueViolation(err) {
		return ErrDuplicateEmail
	}
	if err != nil {
		return err
	}
	user.ID = id.String()
	user.CondominiumID = condoID.String()
	return nil
}

func (r *postgresRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
	var (
		id                           uuid.UUID
		username, mail, passwordHash string
		createdAt                    time.Time
	)
	err := r.pool.QueryRow(ctx, `SELECT id, username, email, password_hash, created_at FROM auth_user_by_email($1)`, email).
		Scan(&id, &username, &mail, &passwordHash, &createdAt)
	if db.IsNoRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &User{ID: id.String(), Username: username, Email: mail, Password: passwordHash}, nil
}

func (r *postgresRepository) Memberships(ctx context.Context, userID string) ([]Membership, error) {
	uid, ok := db.ParseID(userID)
	if !ok {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT condominium_id, name, roles FROM auth_memberships($1)`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Membership
	for rows.Next() {
		var (
			cid uuid.UUID
			m   Membership
		)
		if err := rows.Scan(&cid, &m.Name, &m.Roles); err != nil {
			return nil, err
		}
		m.CondominiumID = cid.String()
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *postgresRepository) FindByID(ctx context.Context, id string) (*User, error) {
	condoID, err := tenant.UUID(ctx)
	if err != nil {
		return nil, err
	}
	uid, ok := db.ParseID(id)
	if !ok {
		return nil, ErrNotFound
	}
	row, err := r.q.GetUserByID(ctx, dbgen.GetUserByIDParams{CondominiumID: condoID, ID: uid})
	if db.IsNoRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &User{
		ID:            row.ID.String(),
		CondominiumID: row.CondominiumID.String(),
		Username:      row.Username,
		Email:         row.Email,
		Password:      row.PasswordHash,
		Roles:         row.Roles,
	}, nil
}

type postgresTokenStore struct {
	pool *pgxpool.Pool
	q    *dbgen.Queries
}

// NewPostgresTokenStore returns a TokenStore backed by the shared pgx pool.
func NewPostgresTokenStore(pool *pgxpool.Pool) TokenStore {
	return &postgresTokenStore{pool: pool, q: db.Queries(pool)}
}

func (s *postgresTokenStore) Insert(ctx context.Context, sess Session, hash []byte, expiresAt time.Time) error {
	condoID, err := tenant.UUID(ctx)
	if err != nil {
		return err
	}
	userID, err := uuid.Parse(sess.UserID)
	if err != nil {
		return err
	}
	familyID, err := uuid.Parse(sess.FamilyID)
	if err != nil {
		return err
	}
	return s.q.InsertRefreshToken(ctx, dbgen.InsertRefreshTokenParams{
		CondominiumID: condoID,
		UserID:        userID,
		FamilyID:      familyID,
		TokenHash:     hash,
		ExpiresAt:     expiresAt,
	})
}

func (s *postgresTokenStore) Consume(ctx context.Context, hash []byte) (Session, error) {
	condoID, err := tenant.UUID(ctx)
	if err != nil {
		return Session{}, err
	}
	row, err := s.q.ConsumeRefreshToken(ctx, dbgen.ConsumeRefreshTokenParams{CondominiumID: condoID, TokenHash: hash})
	if db.IsNoRows(err) {
		return Session{}, ErrTokenNotFound
	}
	if err != nil {
		return Session{}, err
	}
	return Session{UserID: row.UserID.String(), FamilyID: row.FamilyID.String()}, nil
}

func (s *postgresTokenStore) Family(ctx context.Context, hash []byte) (string, error) {
	condoID, err := tenant.UUID(ctx)
	if err != nil {
		return "", err
	}
	id, err := s.q.GetRefreshTokenFamily(ctx, dbgen.GetRefreshTokenFamilyParams{CondominiumID: condoID, TokenHash: hash})
	if db.IsNoRows(err) {
		return "", ErrTokenNotFound
	}
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

func (s *postgresTokenStore) RevokeFamily(ctx context.Context, familyID string) error {
	condoID, err := tenant.UUID(ctx)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(familyID)
	if err != nil {
		return ErrTokenNotFound
	}
	return s.q.RevokeRefreshTokenFamily(ctx, dbgen.RevokeRefreshTokenFamilyParams{CondominiumID: condoID, FamilyID: id})
}

func (s *postgresTokenStore) CondominiumOf(ctx context.Context, hash []byte) (string, error) {
	var id *uuid.UUID
	if err := s.pool.QueryRow(ctx, `SELECT auth_refresh_condominium($1)`, hash).Scan(&id); err != nil {
		return "", err
	}
	if id == nil {
		return "", ErrTokenNotFound
	}
	return id.String(), nil
}
