-- One-time backfill for existing image-runtime conversations.
-- Run after migration 202607170900000000_add_chat_runtime_conversation_type.

UPDATE chat_runtime_conversations AS c
SET conversation_type = 'image'
WHERE c.deleted_at IS NULL
  AND c.conversation_type <> 'image'
  AND EXISTS (
    SELECT 1
    FROM chat_runtime_messages AS m
    WHERE m.conversation_id = c.id
      AND m.deleted_at IS NULL
      AND m.metadata ? 'image_generation'
  );
