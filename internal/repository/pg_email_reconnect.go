package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/infrastructure/db"
	"github.com/warmbly/warmbly/internal/models"
)

// ReconnectOutlookCredentials is a compare-and-swap: it cannot insert an
// account, alter the sender graph or overwrite a refresh that raced the operator.
func (r *emailRepository) ReconnectOutlookCredentials(ctx context.Context, id uuid.UUID, userID string, orgID uuid.UUID, email, previousRefreshToken, accessToken, refreshToken string, expiresAt time.Time) *errx.Error {
	if id == uuid.Nil || orgID == uuid.Nil || userID == "" || email == "" || previousRefreshToken == "" || previousRefreshToken == models.GraphAppOnlyRefreshToken || accessToken == "" || refreshToken == "" || expiresAt.IsZero() {
		return errx.ErrInvalid
	}
	const query = `UPDATE email_accounts_oauth o
		SET access_token = $6, refresh_token = $7, expires_at = $8
		FROM email_accounts a
		WHERE o.email_account_id = a.id AND a.id = $1 AND a.user_id = $2
		  AND a.organization_id = $3 AND lower(a.email) = lower($4)
		  AND a.provider = 'outlook' AND a.status = 'active'
		  AND o.refresh_token = $5`
	result, err := r.DB.Exec(ctx, query, id, userID, orgID, email, previousRefreshToken, accessToken, refreshToken, expiresAt)
	if err != nil {
		// Never capture query parameters: these include raw OAuth credentials.
		db.CaptureError(err, query, nil, "exec")
		return errx.InternalError()
	}
	if result.RowsAffected() != 1 {
		return errx.New(errx.BadRequest, "mailbox identity or prior credentials changed; reconnect aborted")
	}
	return nil
}
