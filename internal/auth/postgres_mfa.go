package auth

import (
	"context"

	"github.com/google/uuid"

	"github.com/victorradael/condoguard/api/internal/db"
	"github.com/victorradael/condoguard/api/internal/db/dbgen"
	"github.com/victorradael/condoguard/api/internal/tenant"
)

var _ MFAStore = (*postgresRepository)(nil)

// ids parses the condominium of ctx and the account id the call is about.
func ids(ctx context.Context, userID string) (condo, user uuid.UUID, err error) {
	c, err := tenant.UUID(ctx)
	if err != nil {
		return condo, user, err
	}
	u, ok := db.ParseID(userID)
	if !ok {
		return condo, user, ErrNotFound
	}
	return c, u, nil
}

func (r *postgresRepository) Security(ctx context.Context, userID string) (*Security, error) {
	condo, uid, err := ids(ctx, userID)
	if err != nil {
		return nil, err
	}
	row, err := r.q.GetAccountSecurity(ctx, dbgen.GetAccountSecurityParams{CondominiumID: condo, ID: uid})
	if db.IsNoRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &Security{
		UserID: row.ID.String(), Username: row.Username, Email: row.Email,
		EmailVerified: row.EmailVerifiedAt != nil,
		SealedSecret:  row.TotpSecret, TOTPEnabled: row.TotpEnabledAt != nil, LastStep: row.TotpLastStep,
	}, nil
}

func (r *postgresRepository) SetPending(ctx context.Context, userID string, sealed []byte) (bool, error) {
	condo, uid, err := ids(ctx, userID)
	if err != nil {
		return false, err
	}
	n, err := r.q.SetPendingTotp(ctx, dbgen.SetPendingTotpParams{TotpSecret: sealed, ID: uid, CondominiumID: condo})
	return n == 1, err
}

func (r *postgresRepository) Enable(ctx context.Context, userID string, step int64) (bool, error) {
	condo, uid, err := ids(ctx, userID)
	if err != nil {
		return false, err
	}
	n, err := r.q.EnableTotp(ctx, dbgen.EnableTotpParams{Step: step, ID: uid, CondominiumID: condo})
	return n == 1, err
}

func (r *postgresRepository) AdvanceStep(ctx context.Context, userID string, step int64) (bool, error) {
	condo, uid, err := ids(ctx, userID)
	if err != nil {
		return false, err
	}
	n, err := r.q.AdvanceTotpStep(ctx, dbgen.AdvanceTotpStepParams{Step: step, ID: uid, CondominiumID: condo})
	return n == 1, err
}

func (r *postgresRepository) Clear(ctx context.Context, userID string) error {
	condo, uid, err := ids(ctx, userID)
	if err != nil {
		return err
	}
	return db.InTx(ctx, r.pool, func(ctx context.Context) error {
		if _, err := r.q.ClearTotp(ctx, dbgen.ClearTotpParams{ID: uid, CondominiumID: condo}); err != nil {
			return err
		}
		return r.q.DeleteRecoveryCodes(ctx, uid)
	})
}

func (r *postgresRepository) ReplaceRecoveryCodes(ctx context.Context, userID string, hashes [][]byte) error {
	_, uid, err := ids(ctx, userID)
	if err != nil {
		return err
	}
	return db.InTx(ctx, r.pool, func(ctx context.Context) error {
		if err := r.q.DeleteRecoveryCodes(ctx, uid); err != nil {
			return err
		}
		for _, h := range hashes {
			if err := r.q.InsertRecoveryCode(ctx, dbgen.InsertRecoveryCodeParams{UserID: uid, CodeHash: h}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *postgresRepository) UseRecoveryCode(ctx context.Context, userID string, hash []byte) (bool, error) {
	_, uid, err := ids(ctx, userID)
	if err != nil {
		return false, err
	}
	n, err := r.q.UseRecoveryCode(ctx, dbgen.UseRecoveryCodeParams{UserID: uid, CodeHash: hash})
	return n == 1, err
}

func (r *postgresRepository) RecoveryCodesLeft(ctx context.Context, userID string) (int, error) {
	_, uid, err := ids(ctx, userID)
	if err != nil {
		return 0, err
	}
	n, err := r.q.CountRecoveryCodes(ctx, uid)
	return int(n), err
}
