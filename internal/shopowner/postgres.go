package shopowner

import (
	"context"
	"fmt"
	"math"

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

func (r *postgresRepository) Save(ctx context.Context, s *ShopOwner) error {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return err
	}
	floor, err := toInt32(s.Floor)
	if err != nil {
		return err
	}
	id, err := r.q.InsertShopOwner(ctx, dbgen.InsertShopOwnerParams{
		CondominiumID: condoID,
		ShopName:      s.ShopName,
		Cnpj:          s.CNPJ,
		Floor:         floor,
		OwnerID:       s.OwnerID,
	})
	if db.IsUniqueViolation(err) {
		return ErrDuplicate
	}
	if err != nil {
		return err
	}
	s.ID = id.String()
	return nil
}

func (r *postgresRepository) FindByID(ctx context.Context, id string) (*ShopOwner, error) {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return nil, err
	}
	uid, ok := db.ParseID(id)
	if !ok {
		return nil, ErrNotFound
	}
	row, err := r.q.GetShopOwnerByID(ctx, dbgen.GetShopOwnerByIDParams{CondominiumID: condoID, ID: uid})
	if db.IsNoRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return fromRow(row), nil
}

func (r *postgresRepository) FindAll(ctx context.Context) ([]*ShopOwner, error) {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := r.q.ListShopOwners(ctx, condoID)
	if err != nil {
		return nil, err
	}
	list := make([]*ShopOwner, 0, len(rows))
	for _, row := range rows {
		list = append(list, fromRow(row))
	}
	return list, nil
}

func (r *postgresRepository) Update(ctx context.Context, s *ShopOwner) error {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return err
	}
	uid, ok := db.ParseID(s.ID)
	if !ok {
		return ErrNotFound
	}
	floor, err := toInt32(s.Floor)
	if err != nil {
		return err
	}
	n, err := r.q.UpdateShopOwner(ctx, dbgen.UpdateShopOwnerParams{
		CondominiumID: condoID,
		ID:            uid,
		ShopName:      s.ShopName,
		Cnpj:          s.CNPJ,
		Floor:         floor,
		OwnerID:       s.OwnerID,
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
	n, err := r.q.DeleteShopOwner(ctx, dbgen.DeleteShopOwnerParams{CondominiumID: condoID, ID: uid})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func fromRow(row dbgen.ShopOwner) *ShopOwner {
	return &ShopOwner{
		ID:       row.ID.String(),
		ShopName: row.ShopName,
		CNPJ:     row.Cnpj,
		Floor:    int(row.Floor),
		OwnerID:  row.OwnerID,
	}
}

func toInt32(floor int) (int32, error) {
	if floor < math.MinInt32 || floor > math.MaxInt32 {
		return 0, fmt.Errorf("%w: floor out of range", ErrValidation)
	}
	return int32(floor), nil
}
