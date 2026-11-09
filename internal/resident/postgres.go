package resident

import (
	"context"
	"fmt"
	"math"

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
//
// The API field condominiumId is still free text in this phase; it is stored
// as condominium_ref and keeps scoping the unit-number uniqueness exactly as
// before. In 1b it becomes the tenant itself.
func NewPostgresRepository(pool *pgxpool.Pool) Repository {
	return &postgresRepository{q: dbgen.New(pool)}
}

func (r *postgresRepository) Save(ctx context.Context, res *Resident) error {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return err
	}
	floor, err := toInt32(res.Floor)
	if err != nil {
		return err
	}
	id, err := r.q.InsertResident(ctx, dbgen.InsertResidentParams{
		CondominiumID:  condoID,
		CondominiumRef: res.CondominiumID,
		UnitNumber:     res.UnitNumber,
		Floor:          floor,
		OwnerID:        res.OwnerID,
	})
	if db.IsUniqueViolation(err) {
		return ErrDuplicate
	}
	if err != nil {
		return err
	}
	res.ID = id.String()
	return nil
}

func (r *postgresRepository) FindByID(ctx context.Context, id string) (*Resident, error) {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return nil, err
	}
	uid, ok := db.ParseID(id)
	if !ok {
		return nil, ErrNotFound
	}
	row, err := r.q.GetResidentByID(ctx, dbgen.GetResidentByIDParams{CondominiumID: condoID, ID: uid})
	if db.IsNoRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return fromRow(row), nil
}

func (r *postgresRepository) FindAll(ctx context.Context) ([]*Resident, error) {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := r.q.ListResidents(ctx, condoID)
	if err != nil {
		return nil, err
	}
	list := make([]*Resident, 0, len(rows))
	for _, row := range rows {
		list = append(list, fromRow(row))
	}
	return list, nil
}

func (r *postgresRepository) Update(ctx context.Context, res *Resident) error {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return err
	}
	uid, ok := db.ParseID(res.ID)
	if !ok {
		return ErrNotFound
	}
	floor, err := toInt32(res.Floor)
	if err != nil {
		return err
	}
	n, err := r.q.UpdateResident(ctx, dbgen.UpdateResidentParams{
		CondominiumID:  condoID,
		ID:             uid,
		CondominiumRef: res.CondominiumID,
		UnitNumber:     res.UnitNumber,
		Floor:          floor,
		OwnerID:        res.OwnerID,
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
	n, err := r.q.DeleteResident(ctx, dbgen.DeleteResidentParams{CondominiumID: condoID, ID: uid})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *postgresRepository) ExistsUnit(ctx context.Context, condominiumID, unitNumber, excludeID string) (bool, error) {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return false, err
	}
	var exclude *uuid.UUID
	if uid, ok := db.ParseID(excludeID); ok {
		exclude = &uid
	}
	return r.q.ResidentUnitExists(ctx, dbgen.ResidentUnitExistsParams{
		CondominiumID:  condoID,
		CondominiumRef: condominiumID,
		UnitNumber:     unitNumber,
		ExcludeID:      exclude,
	})
}

func fromRow(row dbgen.Resident) *Resident {
	return &Resident{
		ID:            row.ID.String(),
		UnitNumber:    row.UnitNumber,
		Floor:         int(row.Floor),
		CondominiumID: row.CondominiumRef,
		OwnerID:       row.OwnerID,
	}
}

func toInt32(floor int) (int32, error) {
	if floor < math.MinInt32 || floor > math.MaxInt32 {
		return 0, fmt.Errorf("%w: floor out of range", ErrValidation)
	}
	return int32(floor), nil
}
