package user

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/victorradael/condoguard/api/internal/db"
	"github.com/victorradael/condoguard/api/internal/db/dbgen"
	"github.com/victorradael/condoguard/api/internal/tenant"
	"github.com/victorradael/condoguard/api/pkg/page"
)

type postgresRepository struct {
	pool *pgxpool.Pool
	q    *dbgen.Queries
}

// NewPostgresRepository returns a Repository backed by the shared pgx pool.
// An account is global; the roles belong to its membership in the condominium
// carried in the context, and every query is scoped by that condominium.
func NewPostgresRepository(pool *pgxpool.Pool) Repository {
	return &postgresRepository{pool: pool, q: db.Queries(pool)}
}

// Save creates the account and its membership in one transaction.
func (r *postgresRepository) Save(ctx context.Context, u *User) error {
	condoID, err := tenant.UUID(ctx)
	if err != nil {
		return err
	}
	id := uuid.New()
	err = db.InTx(ctx, r.pool, func(ctx context.Context) error {
		if ierr := r.q.InsertUser(ctx, dbgen.InsertUserParams{ID: id, Username: u.Username, Email: u.Email, PasswordHash: u.Password}); ierr != nil {
			return ierr
		}
		return r.q.InsertMembership(ctx, dbgen.InsertMembershipParams{UserID: id, CondominiumID: condoID, Roles: nonNil(u.Roles)})
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
	return fromRow(row.ID, row.Username, row.Email, row.PasswordHash, row.Roles), nil
}

func (r *postgresRepository) FindAll(ctx context.Context, p page.Params) ([]*User, int64, error) {
	condoID, err := tenant.UUID(ctx)
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
		list = append(list, fromRow(row.ID, row.Username, row.Email, row.PasswordHash, row.Roles))
	}
	return list, total, nil
}

// Update changes the account's name and the roles of the membership together.
func (r *postgresRepository) Update(ctx context.Context, u *User) error {
	condoID, err := tenant.UUID(ctx)
	if err != nil {
		return err
	}
	uid, ok := db.ParseID(u.ID)
	if !ok {
		return ErrNotFound
	}
	err = db.InTx(ctx, r.pool, func(ctx context.Context) error {
		n, uerr := r.q.UpdateUser(ctx, dbgen.UpdateUserParams{
			CondominiumID: condoID, ID: uid, Username: u.Username, Email: u.Email, PasswordHash: u.Password,
		})
		if uerr != nil {
			return uerr
		}
		if n == 0 {
			return ErrNotFound
		}
		_, err = r.q.UpdateMembershipRoles(ctx, dbgen.UpdateMembershipRolesParams{CondominiumID: condoID, UserID: uid, Roles: nonNil(u.Roles)})
		return err
	})
	if db.IsUniqueViolation(err) {
		return ErrDuplicate
	}
	return err
}

func (r *postgresRepository) Delete(ctx context.Context, id string) error {
	if _, err := tenant.UUID(ctx); err != nil {
		return err
	}
	uid, ok := db.ParseID(id)
	if !ok {
		return ErrNotFound
	}
	n, err := r.q.RemoveMembership(ctx, uid)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *postgresRepository) RevokeSessions(ctx context.Context, id string) error {
	condoID, err := tenant.UUID(ctx)
	if err != nil {
		return err
	}
	uid, ok := db.ParseID(id)
	if !ok {
		return ErrNotFound
	}
	return r.q.RevokeUserRefreshTokens(ctx, dbgen.RevokeUserRefreshTokensParams{CondominiumID: condoID, UserID: uid})
}

func fromRow(id uuid.UUID, username, email, hash string, roles []string) *User {
	return &User{ID: id.String(), Username: username, Email: email, Password: hash, Roles: roles}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
