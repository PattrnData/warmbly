package repository

import (
	"context"
	"errors"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/infrastructure/db"
	"github.com/warmbly/warmbly/internal/models"
)

func (r *emailRepository) GetSMTPIMAP(ctx context.Context, userID, emailAccountID string) (*models.SmtpImap, *errx.Error) {
	var imap models.Service
	var smtp models.Service
	var ts time.Time

	query := `
		SELECT
    	 smtp.smtp_host,
    	 smtp.smtp_port,
    	 smtp.smtp_user,
    	 smtp.smtp_password,
   		 smtp.imap_host,
    	 smtp.imap_port,
    	 smtp.imap_user,
    	 smtp.imap_password,
    	 smtp.updated_at
	 	FROM 
    	 email_accounts ea
		JOIN 
    	 email_accounts_smtp_imap smtp ON ea.id = smtp.email_account_id
		WHERE 
     	 ea.user_id = $1
    	 AND ea.id = $2
	`

	params := []any{
		userID,
		emailAccountID,
	}

	err := r.DB.QueryRow(
		ctx,
		query,
		params...,
	).Scan(
		&smtp.Host, &smtp.Port, &smtp.Username, &smtp.Password,
		&imap.Host, &imap.Port, &imap.Username, &imap.Password,
		&ts,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		db.CaptureError(err, query, params, "queryrow")
		return nil, errx.InternalError()
	}
	imap.Username, err = r.Encrypt.Decrypt(imap.Username)
	if err != nil {
		sentry.CaptureException(err)
		return nil, errx.InternalError()
	}
	imap.Password, err = r.Encrypt.Decrypt(imap.Password)
	if err != nil {
		sentry.CaptureException(err)
		return nil, errx.InternalError()
	}
	imap.Host, err = r.Encrypt.Decrypt(imap.Host)
	if err != nil {
		sentry.CaptureException(err)
		return nil, errx.InternalError()
	}
	smtp.Username, err = r.Encrypt.Decrypt(smtp.Username)
	if err != nil {
		sentry.CaptureException(err)
		return nil, errx.InternalError()
	}
	smtp.Password, err = r.Encrypt.Decrypt(smtp.Password)
	if err != nil {
		sentry.CaptureException(err)
		return nil, errx.InternalError()
	}
	smtp.Host, err = r.Encrypt.Decrypt(smtp.Host)
	if err != nil {
		sentry.CaptureException(err)
		return nil, errx.InternalError()
	}

	return &models.SmtpImap{
		SMTP: &smtp,
		IMAP: &imap,
	}, nil
}

func (r *emailRepository) RevokeOauth(ctx context.Context, id string) *errx.Error {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		db.CaptureError(err, "", nil, "begin")
		return errx.InternalError()
	}
	defer tx.Rollback(ctx)

	query := `
		UPDATE email_accounts 
		SET status = 'revoked' WHERE id = $1
	`

	params := []any{
		id,
	}
	_, err = tx.Exec(
		ctx,
		query,
		params...,
	)
	if err != nil {
		db.CaptureError(err, query, params, "exec")
		return errx.InternalError()
	}

	query = `
		DELETE FROM email_accounts_oauth
		WHERE email_account_id = $1
	`

	params = []any{
		id,
	}

	_, err = tx.Exec(
		ctx,
		query,
		params...,
	)
	if err != nil {
		db.CaptureError(err, query, params, "exec")
		return errx.InternalError()
	}

	if err := tx.Commit(ctx); err != nil {
		db.CaptureError(err, "", nil, "commit")
		return errx.InternalError()
	}

	return nil
}

func (r *emailRepository) RefreshBoxToken(ctx context.Context, id uuid.UUID, accessToken, refreshToken string, expiresAt time.Time) error {
	sealedAccess, sealedRefresh, err := r.sealOAuthTokens(accessToken, refreshToken)
	if err != nil {
		return err
	}
	const query = `UPDATE email_accounts_oauth SET access_token = $1, refresh_token = $2, expires_at = $3 WHERE email_account_id = $4`
	_, err = r.DB.Exec(ctx, query, sealedAccess, sealedRefresh, expiresAt, id)
	if err != nil {
		// Never log token-bearing parameters.
		db.CaptureError(err, query, nil, "exec")
	}
	return err
}

// sealOAuthTokens mirrors GetOAuthCredentials: app-only sentinel rows retain
// their legacy shape; delegated credentials must always be encrypted.
func (r *emailRepository) sealOAuthTokens(accessToken, refreshToken string) (string, string, error) {
	if accessToken == "" && refreshToken == models.GraphAppOnlyRefreshToken {
		return "", refreshToken, nil
	}
	if r.Encrypt == nil {
		return "", "", errNoCredentialEncrypter
	}
	if accessToken == "" || refreshToken == "" || refreshToken == models.GraphAppOnlyRefreshToken {
		return "", "", errors.New("invalid delegated OAuth credential shape")
	}
	sealedAccess, err := r.Encrypt.Encrypt(accessToken)
	if err != nil {
		return "", "", err
	}
	sealedRefresh, err := r.Encrypt.Encrypt(refreshToken)
	if err != nil {
		return "", "", err
	}
	return sealedAccess, sealedRefresh, nil
}

// IsDelegatedOutlookAccount checks shape without reading or decrypting any stored token.
func (r *emailRepository) IsDelegatedOutlookAccount(ctx context.Context, userID string, orgID, id uuid.UUID) (bool, *errx.Error) {
	const query = `SELECT EXISTS (
		SELECT 1 FROM email_accounts ea JOIN email_accounts_oauth o ON o.email_account_id = ea.id
		WHERE ea.id = $1 AND ea.user_id = $2 AND ea.organization_id = $3
		  AND ea.provider = 'outlook' AND ea.status = 'inactive'
		  AND o.refresh_token <> $4
	)`
	var ok bool
	if err := r.DB.QueryRow(ctx, query, id, userID, orgID, models.GraphAppOnlyRefreshToken).Scan(&ok); err != nil {
		db.CaptureError(err, query, nil, "queryrow")
		return false, errx.InternalError()
	}
	return ok, nil
}

// ReconnectOutlookToken updates credentials only for the exact inactive,
// owner/org-scoped delegated Outlook mailbox. It does not activate it.
func (r *emailRepository) ReconnectOutlookToken(ctx context.Context, userID string, orgID, id uuid.UUID, email, accessToken, refreshToken string, expiresAt time.Time) *errx.Error {
	sealedAccess, sealedRefresh, err := r.sealOAuthTokens(accessToken, refreshToken)
	if err != nil {
		sentry.CaptureException(err)
		return errx.InternalError()
	}
	const query = `
		UPDATE email_accounts_oauth o
		SET access_token = $1, refresh_token = $2, expires_at = $3
		FROM email_accounts ea
		WHERE o.email_account_id = ea.id AND ea.id = $4
		  AND ea.user_id = $5 AND ea.organization_id = $6
		  AND lower(ea.email) = lower($7) AND ea.provider = 'outlook'
		  AND ea.status = 'inactive'
		  AND o.refresh_token <> $8
	`
	tag, err := r.DB.Exec(ctx, query, sealedAccess, sealedRefresh, expiresAt, id, userID, orgID, email, models.GraphAppOnlyRefreshToken)
	if err != nil {
		db.CaptureError(err, query, nil, "exec")
		return errx.InternalError()
	}
	if tag.RowsAffected() != 1 {
		return errx.ErrEmailOnboardState
	}
	return nil
}
