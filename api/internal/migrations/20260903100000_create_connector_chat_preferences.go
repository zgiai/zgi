package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const createConnectorChatPreferencesSQL = `
CREATE FUNCTION public.connector_chat_defaults_valid(value jsonb) RETURNS boolean
LANGUAGE sql IMMUTABLE AS $$
 SELECT CASE WHEN jsonb_typeof(value) <> 'object' THEN false ELSE (
  SELECT count(*) <= 64 AND COALESCE(bool_and(COALESCE(
   key ~ '^[a-z0-9][a-z0-9._-]{0,127}$' AND jsonb_typeof(entry) = 'object' AND (
    (entry = '{"mode":"none"}'::jsonb) OR
    (entry->>'mode' = 'selected' AND jsonb_typeof(entry->'connection_id') = 'string'
     AND (entry - 'mode' - 'connection_id') = '{}'::jsonb
     AND entry->>'connection_id' ~ '^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$'
     AND entry->>'connection_id' <> '00000000-0000-0000-0000-000000000000')
   )
  , false)), true) FROM jsonb_each(value) AS selection(key,entry)
 ) END
$$;
CREATE TABLE public.connector_chat_preferences (
 organization_id uuid NOT NULL, owner_account_id uuid NOT NULL,
 revision bigint NOT NULL CHECK (revision > 0), defaults jsonb NOT NULL DEFAULT '{}'::jsonb,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (organization_id, owner_account_id),
 CHECK (public.connector_chat_defaults_valid(defaults) AND octet_length(defaults::text) <= 32768)
);
CREATE TABLE public.connector_chat_preference_mutations (
 organization_id uuid NOT NULL, owner_account_id uuid NOT NULL,
 key_hash varchar(64) NOT NULL CHECK (key_hash ~ '^[a-f0-9]{64}$'),
 fingerprint varchar(64) NOT NULL CHECK (fingerprint ~ '^[a-f0-9]{64}$'),
 result_revision bigint NOT NULL CHECK (result_revision > 0),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (organization_id, owner_account_id, key_hash),
 FOREIGN KEY (organization_id, owner_account_id)
  REFERENCES public.connector_chat_preferences(organization_id, owner_account_id)
);
`

func init() {
	registerSchemaMigration("20260903100000_create_connector_chat_preferences", upCreateConnectorChatPreferences, downCreateConnectorChatPreferences)
}
func upCreateConnectorChatPreferences(schema *mschema.Builder) error {
	return schema.Raw(createConnectorChatPreferencesSQL)
}
func downCreateConnectorChatPreferences(schema *mschema.Builder) error {
	return schema.Raw("DROP TABLE public.connector_chat_preference_mutations; DROP TABLE public.connector_chat_preferences; DROP FUNCTION public.connector_chat_defaults_valid(jsonb);")
}
