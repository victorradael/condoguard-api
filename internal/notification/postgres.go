package notification

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/victorradael/condoguard/api/internal/authz"
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
		ResidentIds:   n.ResidentIDs,
		ShopOwnerIds:  n.ShopOwnerIDs,
	})
	if err != nil {
		return err
	}
	n.ID = id.String()
	return nil
}

func (r *postgresRepository) FindByID(ctx context.Context, id, readerID string) (*Notification, error) {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return nil, err
	}
	uid, ok := db.ParseID(id)
	if !ok {
		return nil, ErrNotFound
	}
	row, err := r.q.GetNotificationByID(ctx, dbgen.GetNotificationByIDParams{CondominiumID: condoID, ID: uid, ReaderID: optionalID(readerID)})
	if db.IsNoRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return fromRow(dbgen.ListNotificationsRow(row)), nil
}

// FindAll lists the notifications visible in scope, newest first.
func (r *postgresRepository) FindAll(ctx context.Context, scope authz.Scope, readerID string, p page.Params) ([]*Notification, int64, error) {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return nil, 0, err
	}
	restricted := !scope.All
	residentIDs, shopIDs := nonNil(scope.Units.ResidentIDs), nonNil(scope.Units.ShopOwnerIDs)
	rows, err := r.q.ListNotifications(ctx, dbgen.ListNotificationsParams{
		CondominiumID: condoID, ReaderID: optionalID(readerID),
		Restricted: restricted, ResidentIds: residentIDs, ShopOwnerIds: shopIDs,
		PageLimit: p.Limit(), PageOffset: p.Offset(),
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountNotifications(ctx, dbgen.CountNotificationsParams{
		CondominiumID: condoID, Restricted: restricted, ResidentIds: residentIDs, ShopOwnerIds: shopIDs,
	})
	if err != nil {
		return nil, 0, err
	}
	list := make([]*Notification, 0, len(rows))
	for _, row := range rows {
		list = append(list, fromRow(row))
	}
	return list, total, nil
}

func (r *postgresRepository) MarkRead(ctx context.Context, id, userID string) (time.Time, error) {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return time.Time{}, err
	}
	nid, ok := db.ParseID(id)
	if !ok {
		return time.Time{}, ErrNotFound
	}
	uid, ok := db.ParseID(userID)
	if !ok {
		return time.Time{}, ErrNotFound
	}
	at, err := r.q.MarkNotificationRead(ctx, dbgen.MarkNotificationReadParams{CondominiumID: condoID, NotificationID: nid, UserID: uid})
	if db.IsNoRows(err) {
		return time.Time{}, ErrNotFound
	}
	if err != nil {
		return time.Time{}, err
	}
	return at.UTC(), nil
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

func fromRow(row dbgen.ListNotificationsRow) *Notification {
	return &Notification{
		ID:           row.ID.String(),
		Message:      row.Message,
		CreatedByID:  row.CreatedByID,
		CreatedAt:    row.CreatedAt.UTC(),
		Read:         row.ReadAt != nil,
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

// optionalID is nil for anything that is not a user UUID (no read state).
func optionalID(id string) *uuid.UUID {
	if u, ok := db.ParseID(id); ok {
		return &u
	}
	return nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
