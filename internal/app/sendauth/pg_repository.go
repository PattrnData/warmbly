package sendauth

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BindingSQL returns no row unless the complete send tuple is still valid.
// No worker-supplied identity or DNS result is projected as outbound evidence.
const BindingSQL = `SELECT ea.provider::text, ea.email, t.task_type::text
 FROM tasks t
 JOIN email_accounts ea ON ea.id = t.email_account_id
 JOIN workers w ON w.id = ea.worker_id AND w.active IS TRUE
 LEFT JOIN email_accounts_smtp_imap sm ON sm.email_account_id = ea.id AND ea.provider = 'smtp_imap'
 LEFT JOIN email_accounts_oauth oa ON oa.email_account_id = ea.id AND ea.provider IN ('gmail', 'outlook')
 WHERE t.id = $1 AND t.email_account_id = $2 AND ea.organization_id = $3
   AND ea.worker_id = $4 AND t.message_id = $5 AND t.message_id <> ''
   AND t.status IN ('active','completed') AND ea.status = 'active'
   AND ((ea.provider = 'smtp_imap' AND sm.email_account_id IS NOT NULL)
     OR (ea.provider IN ('gmail','outlook') AND oa.email_account_id IS NOT NULL))
   AND ( (t.task_type = 'campaign' AND EXISTS (
       SELECT 1 FROM campaign_tasks ct JOIN campaigns c ON c.id = ct.campaign_id
       WHERE ct.task_id = t.id AND c.organization_id = ea.organization_id AND c.status = 'active'))
     OR (t.task_type = 'warmup' AND ea.warmup IS NOT NULL AND ea.warmup_paused_at IS NULL
       AND EXISTS (SELECT 1 FROM warmup_tasks wt WHERE wt.task_id = t.id))
     OR (t.task_type = 'email' AND EXISTS (SELECT 1 FROM email_tasks et WHERE et.task_id = t.id)) )`

type PGRepository struct{ Pool *pgxpool.Pool }

func (r PGRepository) Lookup(ctx context.Context, req Request) (Snapshot, error) {
	var snap Snapshot
	if r.Pool == nil {
		return snap, errors.New("no database")
	}
	err := r.Pool.QueryRow(ctx, BindingSQL, req.TaskID, req.EmailAccountID, req.OrganizationID, req.WorkerID, req.MessageID).Scan(
		&snap.Provider, &snap.From, &snap.TaskType)
	if errors.Is(err, pgx.ErrNoRows) {
		return snap, ErrNotFound
	}
	return snap, err
}
