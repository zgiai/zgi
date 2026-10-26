-- Per-Agent fingerprints of saved Agent memory, for checking a migration.
-- Run before and after migrating and compare the output row by row:
--   psql -At -F ',' -f checksums.sql > before.csv
-- Each row covers one Agent and one store. Values are hashed with their key,
-- user, revision, and source, so any lost, changed, or misattributed value
-- changes the fingerprint. Relationship rows also carry their state.

-- Relationship store.
SELECT 'relationship' AS store,
       r.consumer_id AS agent_id,
       count(DISTINCT r.id) AS relationships,
       count(i.id) AS values,
       md5(string_agg(
             r.subject_id::text || '|' || r.status || '|' || r.memory_enabled::text || '|' || r.auto_capture_enabled::text || '|' || r.memory_epoch::text
             || '|' || coalesce(i.memory_key, '') || '|' || coalesce(i.content_encoding, '') || '|' || coalesce(i.content_payload, '')
             || '|' || coalesce(i.revision::text, '') || '|' || coalesce(i.source_kind, ''),
             E'\n' ORDER BY r.subject_id, i.memory_key)) AS fingerprint
FROM user_memory_relationships r
LEFT JOIN user_memory_items i ON i.relationship_id = r.id
WHERE r.consumer_type = 'agent'
GROUP BY r.consumer_id

UNION ALL

-- Legacy slot store, read-only since ADR 0016; moved by schema migration.
SELECT 'legacy',
       v.agent_id,
       count(DISTINCT (v.user_scope, v.user_id)),
       count(*),
       md5(string_agg(
             v.user_scope || '|' || v.user_id::text || '|' || v.slot_key || '|' || v.content || '|' || v.revision::text || '|' || coalesce(v.source_kind, ''),
             E'\n' ORDER BY v.user_scope, v.user_id, v.slot_key))
FROM agent_memory_values v
GROUP BY v.agent_id

UNION ALL

-- Slot definitions, which become block definitions.
SELECT 'definitions',
       s.agent_id,
       NULL,
       count(*),
       md5(string_agg(
             s.key || '|' || s.name || '|' || s.description || '|' || s.max_chars::text || '|' || s.enabled::text || '|' || s.sort_order::text,
             E'\n' ORDER BY s.sort_order, s.key))
FROM agent_memory_slots s
GROUP BY s.agent_id

ORDER BY 1, 2;
