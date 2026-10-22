package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// A DigitalHuman task approval in an interactive conversation now holds its
// run, like a workflow approval: the run waits with the 'task' origin until
// the approval is answered, instead of staying running until it is reaped.
const digitalHumanTaskApprovalOriginSQL = `
ALTER TABLE public.digital_human_runs DROP CONSTRAINT IF EXISTS digital_human_runs_waiting_check;
ALTER TABLE public.digital_human_runs ADD CONSTRAINT digital_human_runs_waiting_check CHECK (
 waiting_approval_origin IN ('', 'platform_cli', 'workflow', 'task') AND (
  (status = 'waiting' AND (
   (waiting_reason = 'waiting_approval' AND (
    (waiting_receipt IS NOT NULL AND waiting_approval_origin = '') OR
    (waiting_receipt IS NULL AND waiting_approval_origin IN ('platform_cli', 'workflow', 'task')
     AND surface IN ('digital_human_console', 'digital_human_webapp', 'digital_human_api')
     AND conversation_id IS NOT NULL AND message_id IS NOT NULL)
   )) OR
   (waiting_reason IN ('waiting_question', 'waiting_client_action', 'waiting_budget')
    AND waiting_receipt IS NULL AND waiting_approval_origin = '')
  )) OR
  (status <> 'waiting' AND waiting_reason = '' AND waiting_receipt IS NULL AND waiting_approval_origin = '')
 )
);`

func init() {
	// Additive only: rollback closes admission; it must not invalidate pending approvals.
	registerSchemaMigration("202610221100000000_digital_human_task_approval_origin", upDigitalHumanTaskApprovalOrigin, nil)
}

func upDigitalHumanTaskApprovalOrigin(schema *mschema.Builder) error {
	return schema.Raw(digitalHumanTaskApprovalOriginSQL)
}
