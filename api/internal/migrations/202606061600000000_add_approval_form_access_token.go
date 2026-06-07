package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration202606061600000000ID = "202606061600000000_add_approval_form_access_token"

const addApprovalFormAccessTokenColumnSQL = `
ALTER TABLE public.workflow_approval_forms
ADD COLUMN IF NOT EXISTS access_token varchar(64)
`

const allowNullableApprovalRecipientAccessTokenSQL = `
ALTER TABLE public.workflow_approval_recipients
ALTER COLUMN access_token DROP NOT NULL
`

const uniqueApprovalFormAccessTokenSQL = `
CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_approval_forms_access_token
ON public.workflow_approval_forms (access_token)
`

const dropApprovalFormAccessTokenIndexSQL = `
DROP INDEX IF EXISTS public.idx_workflow_approval_forms_access_token
`

func init() {
	registerSchemaMigration(migration202606061600000000ID, up202606061600000000, down202606061600000000)
}

func up202606061600000000(schema *mschema.Builder) error {
	if err := schema.Raw(addApprovalFormAccessTokenColumnSQL); err != nil {
		return err
	}
	if err := schema.Raw(allowNullableApprovalRecipientAccessTokenSQL); err != nil {
		return err
	}
	return schema.Raw(uniqueApprovalFormAccessTokenSQL)
}

func down202606061600000000(schema *mschema.Builder) error {
	return schema.Raw(dropApprovalFormAccessTokenIndexSQL)
}
