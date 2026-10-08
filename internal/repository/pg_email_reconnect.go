package repository

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/infrastructure/db"
	"github.com/warmbly/warmbly/internal/models"
)

// ReconnectOutlookCredentials changes only an existing delegated OAuth row.
// Locking the ciphertext lets us compare its plaintext without storing a token hash.
func (r *emailRepository) ReconnectOutlookCredentials(ctx context.Context, id uuid.UUID, userID string, orgID uuid.UUID, email, previousRefreshToken, accessToken, refreshToken string, expiresAt time.Time) *errx.Error {
	if id == uuid.Nil || orgID == uuid.Nil || userID == "" || email == "" || previousRefreshToken == "" || previousRefreshToken == models.GraphAppOnlyRefreshToken || accessToken == "" || refreshToken == "" || expiresAt.IsZero() {
		return errx.ErrInvalid
	}
	if r.Encrypt == nil {
		return errx.InternalError()
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		db.CaptureError(err, "", nil, "begin")
		return errx.InternalError()
	}
	defer tx.Rollback(ctx)

	const selectQuery = `SELECT o.refresh_token FROM email_accounts_oauth o
		JOIN email_accounts a ON a.id = o.email_account_id
		WHERE a.id = $1 AND a.user_id = $2 AND a.organization_id = $3
		  AND lower(a.email) = lower($4) AND a.provider = 'outlook'
		  AND a.status = 'active' FOR UPDATE OF o, a`
	var storedRefresh string
	if err := tx.QueryRow(ctx, selectQuery, id, userID, orgID, email).Scan(&storedRefresh); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errx.New(errx.BadRequest, "mailbox identity or prior credentials changed; reconnect aborted")
		}
		db.CaptureError(err, selectQuery, nil, "queryrow")
		return errx.InternalError()
	}
	oldRefresh, err := r.Encrypt.Decrypt(storedRefresh)
	if err != nil {
		return errx.InternalError()
	}
	if subtle.ConstantTimeCompare([]byte(oldRefresh), []byte(previousRefreshToken)) != 1 || oldRefresh == models.GraphAppOnlyRefreshToken {
		return errx.New(errx.BadRequest, "mailbox identity or prior credentials changed; reconnect aborted")
	}
	sealedAccess, err := r.Encrypt.Encrypt(accessToken)
	if err != nil {
		return errx.InternalError()
	}
	sealedRefresh, err := r.Encrypt.Encrypt(refreshToken)
	if err != nil {
		return errx.InternalError()
	}
	const updateQuery = `UPDATE email_accounts_oauth SET access_token = $1, refresh_token = $2, expires_at = $3
		WHERE email_account_id = $4 AND refresh_token = $5`
	result, err := tx.Exec(ctx, updateQuery, sealedAccess, sealedRefresh, expiresAt, id, storedRefresh)
	if err != nil {
		// Do not capture parameters: they contain credential ciphertext.
		db.CaptureError(err, updateQuery, nil, "exec")
		return errx.InternalError()
	}
	if result.RowsAffected() != 1 {
		return errx.New(errx.BadRequest, "mailbox identity or prior credentials changed; reconnect aborted")
	}
	if err := tx.Commit(ctx); err != nil {
		db.CaptureError(err, "", nil, "commit")
		return errx.InternalError()
	}
	return nil
}
