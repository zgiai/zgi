-- Apply token-based image pricing to gpt-image-2 (provider=openai).
-- Pre mirrors the console authority; the local resolver keeps the same
-- exclusive token shape. NOTE: platform models are re-synced from the console
-- published catalog, so this row is durable only after the catalog is
-- republished.
-- Generated 2026-09-18. See image-token-pricing-rollback.sql for the inverse.

BEGIN;
UPDATE llm_models
SET pricing = '{"rules":[{"id":"chat-input","unit":"per_1m_tokens","meter":"input_token","price":"8","operation":"chat","configured":false},{"id":"chat-output","unit":"per_1m_tokens","meter":"output_token","price":"30","operation":"chat","configured":false},{"id":"image-token-text-input","unit":"per_1m_tokens","meter":"input_text_token","price":"5","operation":"image_generation","configured":true},{"id":"image-token-image-input","unit":"per_1m_tokens","meter":"input_image_token","price":"8","operation":"image_generation","configured":true},{"id":"image-token-output","unit":"per_1m_tokens","meter":"output_token","price":"30","operation":"image_generation","configured":true}],"version":2,"token_tiers":[{"min_input_tokens":0,"input_price_per_million":8,"output_price_per_million":30}],"image_token_pricing":{"currency":"USD","text_input_price_per_million":5,"image_input_price_per_million":8,"output_price_per_million":30},"deployment_scope":"global"}'::jsonb,
    updated_at = NOW()
WHERE name = 'gpt-image-2'
  AND provider = 'openai'
  AND deleted_at IS NULL;
COMMIT;
