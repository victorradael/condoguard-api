package notification

import (
	"context"
	"time"

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

func (r *postgresRepository) Save(ctx context.Context, n *Notification) error {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return err
	}
	n.ResidentIDs = nonNil(n.ResidentIDs)
	n.ShopOwnerIDs = nonNil(n.ShopOwnerIDs)
	id, err := r.q.InsertNotification(ctx, dbgen.InsertNotificationParams{
		CondominiumID: condoID,
		Message:       n.Message,
		CreatedByID:   n.CreatedByID,
		CreatedAt:     n.CreatedAt,
		Read:          n.Read,
		ReadAt:        n.ReadAt,
		ResidentIds:   n.ResidentIDs,
		ShopOwnerIds:  n.ShopOwnerIDs,
	})
	if err != nil {
		return err
	}
	n.ID = id.String()
	return nil
}

func (r *postgresRepository) FindByID(ctx context.Context, id string) (*Notification, error) {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return nil, err
	}
	uid, ok := db.ParseID(id)
	if !ok {
		return nil, ErrNotFound
	}
	row, err := r.q.GetNotificationByID(ctx, dbgen.GetNotificationByIDParams{CondominiumID: condoID, ID: uid})
	if db.IsNoRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return fromRow(row), nil
}

// FindAll lists notifications, newest first.
func (r *postgresRepository) FindAll(ctx context.Context) ([]*Notification, error) {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := r.q.ListNotifications(ctx, condoID)
	if err != nil {
		return nil, err
	}
	list := make([]*Notification, 0, len(rows))
	for _, row := range rows {
		list = append(list, fromRow(row))
	}
	return list, nil
}

func (r *postgresRepository) Update(ctx context.Context, n *Notification) error {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return err
	}
	uid, ok := db.ParseID(n.ID)
	if !ok {
		return ErrNotFound
	}
	count, err := r.q.UpdateNotification(ctx, dbgen.UpdateNotificationParams{
		CondominiumID: condoID,
		ID:            uid,
		Message:       n.Message,
		CreatedByID:   n.CreatedByID,
		Read:          n.Read,
		ReadAt:        n.ReadAt,
		ResidentIds:   nonNil(n.ResidentIDs),
		ShopOwnerIds:  nonNil(n.ShopOwnerIDs),
	})
	if err != nil {
		return err
	}
	if count == 0 {
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
	count, err := r.q.DeleteNotification(ctx, dbgen.DeleteNotificationParams{CondominiumID: condoID, ID: uid})
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func fromRow(row dbgen.Notification) *Notification {
	return &Notification{
		ID:           row.ID.String(),
		Message:      row.Message,
		CreatedByID:  row.CreatedByID,
		CreatedAt:    row.CreatedAt.UTC(),
		Read:         row.Read,
		ReadAt:       utcPtr(row.ReadAt),
		ResidentIDs:  nonNil(row.ResidentIds),
		ShopOwnerIDs: nonNil(row.ShopOwnerIds),
	}
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
