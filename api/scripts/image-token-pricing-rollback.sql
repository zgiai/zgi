-- Rollback token-based image pricing for gpt-image-2 (provider=openai).
-- Restores the exact pre-change pricing envelope.
-- Generated 2026-09-18. See image-token-pricing-rollback.sql for the inverse.

BEGIN;
UPDATE llm_models
SET pricing = '{"token_tiers": [{"min_input_tokens": 0, "input_price_per_million": 8, "output_price_per_million": 30}], "price_per_image": 0.1, "deployment_scope": "global"}'::jsonb,
    updated_at = NOW()
WHERE name = 'gpt-image-2'
  AND provider = 'openai'
  AND deleted_at IS NULL;
COMMIT;
