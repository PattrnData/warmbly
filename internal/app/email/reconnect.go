package email

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
)

// ReconnectOutlook replaces only the OAuth credentials of one existing,
// explicitly targeted delegated Outlook account. It is deliberately not wired
// to a public route: an operator must approve the target and exchange code.
// Shared and app-only mailboxes require separate tenant/delegation evidence.
func (s *emailService) ReconnectOutlook(ctx context.Context, userID string, orgID, accountID uuid.UUID, code string) (*models.Email, *errx.Error) {
	if userID == "" || orgID == uuid.Nil || accountID == uuid.Nil || strings.TrimSpace(code) == "" {
		return nil, errx.ErrInvalid
	}
	account, xerr := s.emailRepository.Get(ctx, userID, accountID.String())
	if xerr != nil {
		return nil, xerr
	}
	if account == nil || account.ID != accountID || account.UserID != userID || account.OrganizationID == nil || *account.OrganizationID != orgID || account.Provider != string(models.InboxProviderOutlook) || account.Status != "active" {
		return nil, errx.ErrInvalid
	}
	creds, xerr := s.emailRepository.GetOAuthCredentials(ctx, accountID)
	if xerr != nil {
		return nil, xerr
	}
	if creds == nil || creds.AppOnly || creds.RefreshToken == "" || creds.RefreshToken == models.GraphAppOnlyRefreshToken {
		return nil, errx.ErrInvalid
	}
	cfg, xerr := s.oauthConfigFor(models.InboxProviderOutlook)
	if xerr != nil {
		return nil, xerr
	}
	tok, err := cfg.Exchange(ctx, strings.TrimSpace(code))
	if err != nil || tok == nil || tok.AccessToken == "" || tok.RefreshToken == "" || !tok.Expiry.After(time.Now()) {
		return nil, errx.ErrEmailOnboardExchange
	}
	owner, xerr := fetchOutlookOwner(ctx, tok.AccessToken)
	if xerr != nil {
		return nil, xerr
	}
	if owner == nil || !strings.EqualFold(strings.TrimSpace(owner.Email), strings.TrimSpace(account.Email)) {
		return nil, errx.New(errx.BadRequest, "OAuth identity does not match the existing Outlook mailbox")
	}
	if xerr := s.emailRepository.ReconnectOutlookCredentials(ctx, accountID, userID, orgID, account.Email, creds.RefreshToken, tok.AccessToken, tok.RefreshToken, tok.Expiry); xerr != nil {
		return nil, xerr
	}
	return account, nil
}
