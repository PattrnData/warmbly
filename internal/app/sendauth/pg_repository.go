package sendauth

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BindingSQL claims a single provider attempt for a currently valid send tuple.
// The unique task_id is the replay boundary, including after a queued task was
// marked completed by the publisher. A timeout after claim must never retry.
const BindingSQL = `WITH eligible AS (
 SELECT t.id, t.message_id, ea.provider::text, ea.email, t.task_type::text
 FROM tasks t
 JOIN email_accounts ea ON ea.id = t.email_account_id
 JOIN workers w ON w.id = ea.worker_id AND w.active IS TRUE
 LEFT JOIN email_accounts_smtp_imap sm ON sm.email_account_id = ea.id AND ea.provider = 'smtp_imap'
 LEFT JOIN email_accounts_oauth oa ON oa.email_account_id = ea.id AND ea.provider IN ('gmail', 'outlook')
 WHERE t.id = $1 AND t.email_account_id = $2 AND ea.organization_id = $3
   AND ea.worker_id = $4 AND t.message_id = $5 AND t.message_id <> ''
   AND t.send_payload_hash = $9 AND t.send_payload_hash IS NOT NULL AND t.send_payload_hash <> ''
   AND lower(ea.email) = lower($6) AND ea.provider::text = $7
   AND (t.task_type = 'warmup') = $8
   AND t.status IN ('active','completed') AND ea.status = 'active'
   AND ((ea.provider = 'smtp_imap' AND sm.email_account_id IS NOT NULL)
     OR (ea.provider IN ('gmail','outlook') AND oa.email_account_id IS NOT NULL))
   AND ( (t.task_type = 'campaign' AND EXISTS (
       SELECT 1 FROM campaign_tasks ct JOIN campaigns c ON c.id = ct.campaign_id
       WHERE ct.task_id = t.id AND c.organization_id = ea.organization_id AND c.status = 'active'))
     OR (t.task_type = 'warmup' AND ea.warmup IS NOT NULL AND ea.warmup_paused_at IS NULL AND NOT ea.warmup_denied
       AND EXISTS (SELECT 1 FROM warmup_tasks wt WHERE wt.task_id = t.id))
     OR (t.task_type = 'email' AND EXISTS (SELECT 1 FROM email_tasks et WHERE et.task_id = t.id)) )
), claimed AS (
 INSERT INTO send_attempt_claims (task_id, message_id)
 SELECT id, message_id FROM eligible WHERE TRUE
 ON CONFLICT (task_id) DO NOTHING
 RETURNING task_id
)
SELECT eligible.provider, eligible.email, eligible.task_type
FROM eligible JOIN claimed ON claimed.task_id = eligible.id`

type PGRepository struct{ Pool *pgxpool.Pool }

func (r PGRepository) Claim(ctx context.Context, req Request) (Snapshot, error) {
	var snap Snapshot
	if r.Pool == nil {
		return snap, errors.New("no database")
	}
	err := r.Pool.QueryRow(ctx, BindingSQL, req.TaskID, req.EmailAccountID, req.OrganizationID, req.WorkerID, req.MessageID, req.From, req.Provider, req.IsWarmup, req.PayloadHash).Scan(
		&snap.Provider, &snap.From, &snap.TaskType)
	if errors.Is(err, pgx.ErrNoRows) {
		return snap, ErrNotFound
	}
	return snap, err
}
