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
	q *dbgen.Queries
}

// NewPostgresRepository returns a Repository backed by the shared pgx pool.
// Every query is scoped by the condominium ID carried in the context.
func NewPostgresRepository(pool *pgxpool.Pool) Repository {
	return &postgresRepository{q: dbgen.New(pool)}
}

func (r *postgresRepository) Save(ctx context.Context, user *User) error {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return err
	}
	roles := user.Roles
	if roles == nil {
		roles = []string{}
	}
	id, err := r.q.InsertUser(ctx, dbgen.InsertUserParams{
		CondominiumID: condoID,
		Username:      user.Username,
		Email:         user.Email,
		PasswordHash:  user.Password,
		Roles:         roles,
	})
	if db.IsUniqueViolation(err) {
		return ErrDuplicateEmail
	}
	if err != nil {
		return err
	}
	user.ID = id.String()
	user.CondominiumID = condoID
	return nil
}

func (r *postgresRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return nil, err
	}
	row, err := r.q.GetUserByEmail(ctx, dbgen.GetUserByEmailParams{CondominiumID: condoID, Email: email})
	return fromRow(row, err)
}

func (r *postgresRepository) FindByID(ctx context.Context, id string) (*User, error) {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return nil, err
	}
	uid, ok := db.ParseID(id)
	if !ok {
		return nil, ErrNotFound
	}
	row, err := r.q.GetUserByID(ctx, dbgen.GetUserByIDParams{CondominiumID: condoID, ID: uid})
	return fromRow(row, err)
}

func fromRow(row dbgen.User, err error) (*User, error) {
	if db.IsNoRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &User{
		ID:            row.ID.String(),
		CondominiumID: row.CondominiumID,
		Username:      row.Username,
		Email:         row.Email,
		Password:      row.PasswordHash,
		Roles:         row.Roles,
	}, nil
}

type postgresTokenStore struct {
	q *dbgen.Queries
}

// NewPostgresTokenStore returns a TokenStore backed by the shared pgx pool.
func NewPostgresTokenStore(pool *pgxpool.Pool) TokenStore {
	return &postgresTokenStore{q: dbgen.New(pool)}
}

func (s *postgresTokenStore) Insert(ctx context.Context, sess Session, hash []byte, expiresAt time.Time) error {
	condoID, err := tenant.CondominiumID(ctx)
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
	condoID, err := tenant.CondominiumID(ctx)
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
	condoID, err := tenant.CondominiumID(ctx)
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
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(familyID)
	if err != nil {
		return ErrTokenNotFound
	}
	return s.q.RevokeRefreshTokenFamily(ctx, dbgen.RevokeRefreshTokenFamilyParams{CondominiumID: condoID, FamilyID: id})
}
