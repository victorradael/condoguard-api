package auth

import (
	"context"

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
	return nil
}

func (r *postgresRepository) FindByUsername(ctx context.Context, username string) (*User, error) {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return nil, err
	}
	row, err := r.q.GetUserByUsername(ctx, dbgen.GetUserByUsernameParams{CondominiumID: condoID, Username: username})
	return fromRow(row, err)
}

func (r *postgresRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return nil, err
	}
	row, err := r.q.GetUserByEmail(ctx, dbgen.GetUserByEmailParams{CondominiumID: condoID, Email: email})
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
		ID:       row.ID.String(),
		Username: row.Username,
		Email:    row.Email,
		Password: row.PasswordHash,
		Roles:    row.Roles,
	}, nil
}
