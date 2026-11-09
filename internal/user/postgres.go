package user

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/victorradael/condoguard/api/internal/db"
	"github.com/victorradael/condoguard/api/internal/db/dbgen"
	"github.com/victorradael/condoguard/api/internal/tenant"
	"github.com/victorradael/condoguard/api/pkg/page"
)

type postgresRepository struct {
	q *dbgen.Queries
}

// NewPostgresRepository returns a Repository backed by the shared pgx pool.
// Every query is scoped by the condominium ID carried in the context.
func NewPostgresRepository(pool *pgxpool.Pool) Repository {
	return &postgresRepository{q: dbgen.New(pool)}
}

func (r *postgresRepository) Save(ctx context.Context, u *User) error {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return err
	}
	id, err := r.q.InsertUser(ctx, dbgen.InsertUserParams{
		CondominiumID: condoID,
		Username:      u.Username,
		Email:         u.Email,
		PasswordHash:  u.Password,
		Roles:         nonNil(u.Roles),
	})
	if db.IsUniqueViolation(err) {
		return ErrDuplicate
	}
	if err != nil {
		return err
	}
	u.ID = id.String()
	return nil
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
	if db.IsNoRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return fromRow(row), nil
}

func (r *postgresRepository) FindAll(ctx context.Context, p page.Params) ([]*User, int64, error) {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.q.ListUsers(ctx, dbgen.ListUsersParams{CondominiumID: condoID, PageLimit: p.Limit(), PageOffset: p.Offset()})
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountUsers(ctx, condoID)
	if err != nil {
		return nil, 0, err
	}
	list := make([]*User, 0, len(rows))
	for _, row := range rows {
		list = append(list, fromRow(row))
	}
	return list, total, nil
}

func (r *postgresRepository) Update(ctx context.Context, u *User) error {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return err
	}
	uid, ok := db.ParseID(u.ID)
	if !ok {
		return ErrNotFound
	}
	n, err := r.q.UpdateUser(ctx, dbgen.UpdateUserParams{
		CondominiumID: condoID,
		ID:            uid,
		Username:      u.Username,
		Email:         u.Email,
		PasswordHash:  u.Password,
		Roles:         nonNil(u.Roles),
	})
	if db.IsUniqueViolation(err) {
		return ErrDuplicate
	}
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *postgresRepository) Delete(ctx context.Context, id string) error {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return err
	}
	uid, ok := db.ParseID(id)
	if !ok {
		return ErrNotFound
	}
	n, err := r.q.DeleteUser(ctx, dbgen.DeleteUserParams{CondominiumID: condoID, ID: uid})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *postgresRepository) RevokeSessions(ctx context.Context, id string) error {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return err
	}
	uid, ok := db.ParseID(id)
	if !ok {
		return ErrNotFound
	}
	return r.q.RevokeUserRefreshTokens(ctx, dbgen.RevokeUserRefreshTokensParams{CondominiumID: condoID, UserID: uid})
}

func fromRow(row dbgen.User) *User {
	return &User{
		ID:       row.ID.String(),
		Username: row.Username,
		Email:    row.Email,
		Password: row.PasswordHash,
		Roles:    row.Roles,
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
