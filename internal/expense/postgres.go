package expense

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
	return &postgresRepository{q: db.Queries(pool)}
}

func (r *postgresRepository) Save(ctx context.Context, e *Expense) error {
	condoID, err := tenant.UUID(ctx)
	if err != nil {
		return err
	}
	id, err := r.q.InsertExpense(ctx, dbgen.InsertExpenseParams{
		CondominiumID: condoID,
		Description:   e.Description,
		AmountCents:   e.AmountCents,
		DueDate:       e.DueDate,
		ResidentID:    e.ResidentID,
		ShopOwnerID:   e.ShopOwnerID,
	})
	if err != nil {
		return err
	}
	e.ID = id.String()
	return nil
}

func (r *postgresRepository) FindByID(ctx context.Context, id string) (*Expense, error) {
	condoID, err := tenant.UUID(ctx)
	if err != nil {
		return nil, err
	}
	uid, ok := db.ParseID(id)
	if !ok {
		return nil, ErrNotFound
	}
	row, err := r.q.GetExpenseByID(ctx, dbgen.GetExpenseByIDParams{CondominiumID: condoID, ID: uid})
	if db.IsNoRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return fromRow(row), nil
}

// FindAll lists expenses ordered by due date, optionally within [From, To].
func (r *postgresRepository) FindAll(ctx context.Context, f Filter, p page.Params) ([]*Expense, int64, error) {
	condoID, err := tenant.UUID(ctx)
	if err != nil {
		return nil, 0, err
	}
	restricted := !f.Scope.All
	residentIDs, shopIDs := nonNil(f.Scope.Units.ResidentIDs), nonNil(f.Scope.Units.ShopOwnerIDs)
	rows, err := r.q.ListExpenses(ctx, dbgen.ListExpensesParams{
		CondominiumID: condoID, DueFrom: f.From, DueTo: f.To,
		Restricted: restricted, ResidentIds: residentIDs, ShopOwnerIds: shopIDs,
		PageLimit: p.Limit(), PageOffset: p.Offset(),
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountExpenses(ctx, dbgen.CountExpensesParams{
		CondominiumID: condoID, DueFrom: f.From, DueTo: f.To,
		Restricted: restricted, ResidentIds: residentIDs, ShopOwnerIds: shopIDs,
	})
	if err != nil {
		return nil, 0, err
	}
	list := make([]*Expense, 0, len(rows))
	for _, row := range rows {
		list = append(list, fromRow(row))
	}
	return list, total, nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func (r *postgresRepository) Update(ctx context.Context, e *Expense) error {
	condoID, err := tenant.UUID(ctx)
	if err != nil {
		return err
	}
	uid, ok := db.ParseID(e.ID)
	if !ok {
		return ErrNotFound
	}
	n, err := r.q.UpdateExpense(ctx, dbgen.UpdateExpenseParams{
		CondominiumID: condoID,
		ID:            uid,
		Description:   e.Description,
		AmountCents:   e.AmountCents,
		DueDate:       e.DueDate,
		ResidentID:    e.ResidentID,
		ShopOwnerID:   e.ShopOwnerID,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *postgresRepository) Delete(ctx context.Context, id string) error {
	condoID, err := tenant.UUID(ctx)
	if err != nil {
		return err
	}
	uid, ok := db.ParseID(id)
	if !ok {
		return ErrNotFound
	}
	n, err := r.q.DeleteExpense(ctx, dbgen.DeleteExpenseParams{CondominiumID: condoID, ID: uid})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func fromRow(row dbgen.Expense) *Expense {
	return &Expense{
		ID:          row.ID.String(),
		Description: row.Description,
		AmountCents: row.AmountCents,
		DueDate:     row.DueDate.UTC(),
		ResidentID:  row.ResidentID,
		ShopOwnerID: row.ShopOwnerID,
	}
}
