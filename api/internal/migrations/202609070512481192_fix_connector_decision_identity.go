package migrations

import (
	"fmt"

	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

const migration202609070512481192ID = "202609070512481192_fix_connector_decision_identity"

func init() {
	registerSchemaMigration(migration202609070512481192ID, up202609070512481192, down202609070512481192)
}

func up202609070512481192(schema *mschema.Builder) error {
	return schema.DataFix("reconcile Connector decision indexes and rejection projection", func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error { return mschema.New(tx).Raw(connectorDecisionIdentitySQL) })
	})
}

func down202609070512481192(schema *mschema.Builder) error {
	return fmt.Errorf("Connector decision identity repair is forward-only; retain the corrected indexes when rolling back application code")
}

const connectorDecisionIdentitySQL = `
SET LOCAL lock_timeout = '5s';
LOCK TABLE public.connector_lifecycle_decisions IN SHARE ROW EXCLUSIVE MODE;
DO $$
DECLARE
 item record;
 expected_keys text[];
 expected_predicate text;
 predicate text;
 old_indexes text[] := ARRAY[]::text[];
 index_name text;
 namespace_name text;
BEGIN
 SELECT n.nspname INTO namespace_name FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
 WHERE c.oid='public.connector_lifecycle_decisions'::regclass;
 IF EXISTS (SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
   LEFT JOIN pg_index i ON i.indexrelid=c.oid WHERE n.nspname=namespace_name
   AND c.relname IN ('idx_connector_decisions_operation_identity','idx_connector_decisions_lifecycle_identity','idx_connector_decisions_finalization_identity')
   AND (i.indrelid IS NULL OR i.indrelid<>'public.connector_lifecycle_decisions'::regclass)) THEN
   RAISE EXCEPTION 'Connector identity index name belongs to another relation';
 END IF;
 IF EXISTS (SELECT 1 FROM public.connector_lifecycle_decisions WHERE logical_operation_id IS NOT NULL
   AND logical_operation_id <> '' AND (btrim(logical_operation_id)='' OR logical_operation_id<>btrim(logical_operation_id))) THEN
   RAISE EXCEPTION 'Connector audit contains noncanonical logical identities; manual investigation required';
 END IF;
 IF EXISTS (SELECT 1 FROM public.connector_lifecycle_decisions WHERE logical_operation_id IS NOT NULL AND logical_operation_id<>''
   GROUP BY organization_id,logical_operation_id,decision,status HAVING count(*)>1)
 OR EXISTS (SELECT 1 FROM public.connector_lifecycle_decisions WHERE logical_operation_id IS NULL OR logical_operation_id=''
   GROUP BY organization_id,correlation_id,decision,status HAVING count(*)>1)
 OR EXISTS (SELECT 1 FROM public.connector_lifecycle_decisions WHERE decision='confirmation_finalization'
   GROUP BY organization_id,receipt_id HAVING count(*)>1)
 OR EXISTS (SELECT 1 FROM public.connector_lifecycle_decisions WHERE decision='confirmation_finalization'
   AND (receipt_id IS NULL OR btrim(receipt_id)='')) THEN
   RAISE EXCEPTION 'Connector audit identity preflight failed; immutable records were not modified';
 END IF;
 FOR item IN
   SELECT c.relname,i.indexrelid,i.indisunique,i.indisvalid,i.indisready,i.indnatts,i.indnkeyatts,
     i.indexprs IS NULL AS simple,am.amname,
     ARRAY(SELECT a.attname::text FROM unnest(i.indkey) WITH ORDINALITY k(attnum,position)
       JOIN pg_attribute a ON a.attrelid=i.indrelid AND a.attnum=k.attnum
       WHERE k.position<=i.indnkeyatts ORDER BY k.position) AS keys,
     pg_get_expr(i.indpred,i.indrelid) AS predicate,
     EXISTS(SELECT 1 FROM pg_constraint con WHERE con.conindid=i.indexrelid) AS constrained
   FROM pg_index i JOIN pg_class c ON c.oid=i.indexrelid JOIN pg_am am ON am.oid=c.relam
   WHERE i.indrelid='public.connector_lifecycle_decisions'::regclass
 LOOP
   predicate := lower(regexp_replace(replace(replace(coalesce(item.predicate,''),'::text',''),'::character varying',''),'[()[:space:]]','','g'));
   expected_keys := NULL;
   IF item.relname='idx_connector_decisions_operation_identity' THEN
     expected_keys:=ARRAY['organization_id','logical_operation_id','decision','status'];
     expected_predicate:='logical_operation_idisnotnullandlogical_operation_id<>''''';
   ELSIF item.relname='idx_connector_decisions_lifecycle_identity' THEN
     expected_keys:=ARRAY['organization_id','correlation_id','decision','status'];
     expected_predicate:='logical_operation_idisnullorlogical_operation_id=''''';
   ELSIF item.relname='idx_connector_decisions_finalization_identity' THEN
     expected_keys:=ARRAY['organization_id','receipt_id'];
     expected_predicate:='decision=''confirmation_finalization''';
   END IF;
   IF expected_keys IS NOT NULL AND (item.keys<>expected_keys OR predicate<>expected_predicate OR
     NOT item.indisunique OR NOT item.indisvalid OR NOT item.indisready OR NOT item.simple OR
     item.amname<>'btree' OR item.indnatts<>item.indnkeyatts OR item.constrained) THEN
     RAISE EXCEPTION 'Connector identity index has an unsupported definition: %',item.relname;
   END IF;
   IF item.indisunique AND item.keys=ARRAY['organization_id','correlation_id','decision','status'] AND item.predicate IS NULL THEN
     IF item.constrained OR NOT item.simple OR item.amname<>'btree' OR item.indnatts<>item.indnkeyatts THEN
       RAISE EXCEPTION 'Connector legacy index has unsupported dependencies: %',item.relname;
     END IF;
     old_indexes:=array_append(old_indexes,item.relname);
   ELSIF item.indisunique AND item.keys=ARRAY['organization_id','correlation_id','decision','status'] AND
     predicate<>'logical_operation_idisnullorlogical_operation_id=''''' THEN
     RAISE EXCEPTION 'Connector correlation index has an unsupported predicate: %',item.relname;
   END IF;
 END LOOP;
 -- Existing target names were checked above; IF NOT EXISTS is not validation.
 CREATE UNIQUE INDEX IF NOT EXISTS idx_connector_decisions_operation_identity ON public.connector_lifecycle_decisions
   (organization_id,logical_operation_id,decision,status) WHERE logical_operation_id IS NOT NULL AND logical_operation_id<>'';
 CREATE UNIQUE INDEX IF NOT EXISTS idx_connector_decisions_lifecycle_identity ON public.connector_lifecycle_decisions
   (organization_id,correlation_id,decision,status) WHERE logical_operation_id IS NULL OR logical_operation_id='';
 CREATE UNIQUE INDEX IF NOT EXISTS idx_connector_decisions_finalization_identity ON public.connector_lifecycle_decisions
   (organization_id,receipt_id) WHERE decision='confirmation_finalization';
 FOREACH index_name IN ARRAY old_indexes LOOP
   EXECUTE format('DROP INDEX %I.%I',namespace_name,index_name);
 END LOOP;
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='public.connector_lifecycle_decisions'::regclass
   AND conname='connector_finalization_receipt_required') THEN
   ALTER TABLE public.connector_lifecycle_decisions ADD CONSTRAINT connector_finalization_receipt_required
     CHECK (decision <> 'confirmation_finalization' OR (receipt_id IS NOT NULL AND btrim(receipt_id) <> ''));
 ELSE
   SELECT lower(regexp_replace(replace(pg_get_expr(conbin,conrelid),'::text',''),'[()[:space:]]','','g')) INTO predicate
   FROM pg_constraint WHERE conrelid='public.connector_lifecycle_decisions'::regclass AND conname='connector_finalization_receipt_required';
   IF predicate <> 'decision<>''confirmation_finalization''orreceipt_idisnotnullandbtrimreceipt_id<>''''' THEN
     RAISE EXCEPTION 'Connector finalization receipt constraint has an unsupported definition';
   END IF;
 END IF;
 ALTER TABLE public.connector_lifecycle_decisions VALIDATE CONSTRAINT connector_finalization_receipt_required;
END $$;
CREATE OR REPLACE FUNCTION public.advance_connector_invocation_state() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_TABLE_NAME='connector_operation_outcomes' THEN
   UPDATE public.connector_invocation_claims SET state=NEW.status
     WHERE organization_id=NEW.organization_id AND logical_operation_id=NEW.logical_operation_id AND state='submitted';
 ELSIF NEW.decision='authorization' AND NEW.status='denied' AND NEW.details->>'rejection_finalized'='true' THEN
   UPDATE public.connector_invocation_claims SET state='denied'
     WHERE organization_id=NEW.organization_id AND logical_operation_id=NEW.logical_operation_id
     AND owner_account_id=NEW.owner_account_id AND acting_principal_id=NEW.acting_principal_id
     AND receipt_id::text=NEW.receipt_id AND connection_id=NEW.connection_id AND connector_id=NEW.connector_id
     AND operation_id=NEW.operation_id AND target_type=NEW.target_type AND target_id=NEW.target_id
     AND target_scope=NEW.target_scope AND target_version_id=COALESCE(NEW.target_version_id,'')
     AND binding_instance_id=COALESCE(NEW.binding_instance_id,'') AND state IN ('claimed','awaiting_confirmation');
 ELSIF NEW.logical_operation_id IS NOT NULL AND NEW.decision IN ('dispatch','confirmation','execution') THEN
   UPDATE public.connector_invocation_claims SET state=NEW.status
     WHERE organization_id=NEW.organization_id AND logical_operation_id=NEW.logical_operation_id
     AND state IN ('claimed','awaiting_confirmation') AND NEW.status IN ('submitted','denied','blocked','cancelled');
 END IF;
 RETURN NEW;
END $$;
`
