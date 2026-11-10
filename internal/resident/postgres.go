package resident

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"
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
//
// The API field condominiumId is a legacy free-text field, stored as
// condominium_ref; the unit number is unique inside its block.
func NewPostgresRepository(pool *pgxpool.Pool) Repository {
	return &postgresRepository{q: db.Queries(pool)}
}

func (r *postgresRepository) Save(ctx context.Context, res *Resident) error {
	condoID, err := tenant.UUID(ctx)
	if err != nil {
		return err
	}
	floor, err := toInt32(res.Floor)
	if err != nil {
		return err
	}
	block, err := blockParam(res.BlockID)
	if err != nil {
		return err
	}
	id, err := r.q.InsertResident(ctx, dbgen.InsertResidentParams{
		ID:             uuid.New(),
		CondominiumID:  condoID,
		CondominiumRef: res.CondominiumID,
		UnitNumber:     res.UnitNumber,
		Floor:          floor,
		OwnerID:        res.OwnerID,
		BlockID:        block,
	})
	if db.IsUniqueViolation(err) {
		return ErrDuplicate
	}
	if db.IsForeignKeyViolation(err) {
		return fmt.Errorf("%w: block not found", ErrValidation)
	}
	if err != nil {
		return err
	}
	res.ID = id.String()
	return nil
}

func (r *postgresRepository) FindByID(ctx context.Context, id string) (*Resident, error) {
	condoID, err := tenant.UUID(ctx)
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
	return &Resident{ID: row.ID.String(), UnitNumber: row.UnitNumber, Floor: int(row.Floor), CondominiumID: row.CondominiumRef, OwnerID: row.OwnerID, BlockID: idString(row.BlockID), BlockName: deref(row.BlockName)}, nil
}

func (r *postgresRepository) FindAll(ctx context.Context, f Filter, p page.Params) ([]*Resident, int64, error) {
	condoID, err := tenant.UUID(ctx)
	if err != nil {
		return nil, 0, err
	}
	var viewer *string
	if f.Viewer != "" {
		v := strings.ToLower(f.Viewer)
		viewer = &v
	}
	block, err := blockParam(f.BlockID)
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.q.ListResidents(ctx, dbgen.ListResidentsParams{
		CondominiumID: condoID, Viewer: viewer, BlockID: block, ByNumber: f.ByNumber, PageLimit: p.Limit(), PageOffset: p.Offset(),
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountResidents(ctx, dbgen.CountResidentsParams{CondominiumID: condoID, Viewer: viewer, BlockID: block})
	if err != nil {
		return nil, 0, err
	}
	list := make([]*Resident, 0, len(rows))
	for _, row := range rows {
		list = append(list, &Resident{ID: row.ID.String(), UnitNumber: row.UnitNumber, Floor: int(row.Floor), CondominiumID: row.CondominiumRef,
			OwnerID: row.OwnerID, BlockID: idString(row.BlockID), BlockName: deref(row.BlockName)})
	}
	return list, total, nil
}

func (r *postgresRepository) IDsByOwner(ctx context.Context, ownerID string) ([]string, error) {
	condoID, err := tenant.UUID(ctx)
	if err != nil {
		return nil, err
	}
	ids, err := r.q.ListResidentIDsByViewer(ctx, dbgen.ListResidentIDsByViewerParams{CondominiumID: condoID, Viewer: strings.ToLower(ownerID)})
	if err != nil {
		return nil, err
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	return out, nil
}

func (r *postgresRepository) Update(ctx context.Context, res *Resident) error {
	condoID, err := tenant.UUID(ctx)
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
	block, err := blockParam(res.BlockID)
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
		BlockID:        block,
	})
	if db.IsUniqueViolation(err) {
		return ErrDuplicate
	}
	if db.IsForeignKeyViolation(err) {
		return fmt.Errorf("%w: block not found", ErrValidation)
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
	condoID, err := tenant.UUID(ctx)
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

func (r *postgresRepository) ExistsUnit(ctx context.Context, blockID, unitNumber, excludeID string) (bool, error) {
	condoID, err := tenant.UUID(ctx)
	if err != nil {
		return false, err
	}
	block, err := blockParam(blockID)
	if err != nil {
		return false, err
	}
	var exclude *uuid.UUID
	if uid, ok := db.ParseID(excludeID); ok {
		exclude = &uid
	}
	return r.q.ResidentUnitExists(ctx, dbgen.ResidentUnitExistsParams{
		CondominiumID: condoID,
		BlockID:       block,
		UnitNumber:    unitNumber,
		ExcludeID:     exclude,
	})
}

// blockParam turns the API block id ("" = none) into the query parameter.
func blockParam(id string) (*uuid.UUID, error) {
	if id == "" {
		return nil, nil
	}
	u, ok := db.ParseID(id)
	if !ok {
		return nil, fmt.Errorf("%w: blockId is invalid", ErrValidation)
	}
	return &u, nil
}

func idString(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func toInt32(floor int) (int32, error) {
	if floor < math.MinInt32 || floor > math.MaxInt32 {
		return 0, fmt.Errorf("%w: floor out of range", ErrValidation)
	}
	return int32(floor), nil
}
