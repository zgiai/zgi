-- Sync test data from zgi_test to local database
-- Purpose: Sync providers and provider_models for testing Dashboard API

-- Target tenant in local database
\set LOCAL_TENANT_ID 'ecd02342-43a7-4c8b-9b2a-6a217b0d3fc9'

-- Source tenant from test database (has good data)
\set SOURCE_TENANT_ID '0a0bb64b-29c3-45b7-bb42-3287cb5a4759'

BEGIN;

-- Clean existing data for the target tenant
DELETE FROM provider_models WHERE tenant_id = :'LOCAL_TENANT_ID';
DELETE FROM providers WHERE tenant_id = :'LOCAL_TENANT_ID';

-- Insert providers from test environment
-- Note: We copy structure but remove encrypted_config for security
INSERT INTO providers (tenant_id, provider_name, provider_type, is_valid, quota_type, created_at, updated_at)
VALUES
  -- OpenAI provider
  (:'LOCAL_TENANT_ID', 'openai', 'custom', true, '', NOW(), NOW()),
  -- Anthropic provider
  (:'LOCAL_TENANT_ID', 'anthropic', 'custom', true, '', NOW(), NOW()),
  -- Zhipu provider
  (:'LOCAL_TENANT_ID', 'zhipu', 'custom', true, '', NOW(), NOW()),
  -- JingZhi Community provider
  (:'LOCAL_TENANT_ID', 'jingzhicommunity', 'custom', true, '', NOW(), NOW()),
  -- System providers
  (:'LOCAL_TENANT_ID', 'zgi/openai/openai', 'custom', true, '', NOW(), NOW()),
  (:'LOCAL_TENANT_ID', 'zgi/agicto/agicto', 'custom', true, '', NOW(), NOW()),
  (:'LOCAL_TENANT_ID', 'zgi/tongyi/tongyi', 'custom', true, '', NOW(), NOW())
ON CONFLICT (tenant_id, provider_name, provider_type, quota_type) DO NOTHING;

-- Insert provider_models from test environment
INSERT INTO provider_models (tenant_id, provider_name, model_name, model_type, is_valid, created_at, updated_at)
VALUES
  -- OpenAI LLM models
  (:'LOCAL_TENANT_ID', 'openai', 'gpt-4', 'llm', true, NOW(), NOW()),
  (:'LOCAL_TENANT_ID', 'openai', 'gpt-4-turbo', 'llm', true, NOW(), NOW()),
  (:'LOCAL_TENANT_ID', 'openai', 'gpt-3.5-turbo', 'llm', true, NOW(), NOW()),
  (:'LOCAL_TENANT_ID', 'openai', 'gpt-4o', 'llm', true, NOW(), NOW()),
  (:'LOCAL_TENANT_ID', 'openai', 'gpt-4o-mini', 'llm', true, NOW(), NOW()),

  -- OpenAI Embedding models
  (:'LOCAL_TENANT_ID', 'openai', 'text-embedding-3-small', 'text-embedding', true, NOW(), NOW()),
  (:'LOCAL_TENANT_ID', 'openai', 'text-embedding-3-large', 'text-embedding', true, NOW(), NOW()),
  (:'LOCAL_TENANT_ID', 'openai', 'text-embedding-ada-002', 'embeddings', true, NOW(), NOW()),

  -- Anthropic LLM models
  (:'LOCAL_TENANT_ID', 'anthropic', 'claude-3-opus-20240229', 'llm', true, NOW(), NOW()),
  (:'LOCAL_TENANT_ID', 'anthropic', 'claude-3-sonnet-20240229', 'llm', true, NOW(), NOW()),
  (:'LOCAL_TENANT_ID', 'anthropic', 'claude-3-haiku-20240307', 'llm', true, NOW(), NOW()),

  -- Zhipu LLM models
  (:'LOCAL_TENANT_ID', 'zhipu', 'glm-4', 'llm', true, NOW(), NOW()),
  (:'LOCAL_TENANT_ID', 'zhipu', 'glm-3-turbo', 'llm', true, NOW(), NOW()),

  -- JingZhi Community models
  (:'LOCAL_TENANT_ID', 'jingzhicommunity', 'gte-large-zh', 'text-embedding', true, NOW(), NOW()),
  (:'LOCAL_TENANT_ID', 'jingzhicommunity', 'gte-base-zh', 'text-embedding', true, NOW(), NOW()),
  (:'LOCAL_TENANT_ID', 'jingzhicommunity', 'gte-rerank-v2', 'rerank', true, NOW(), NOW()),
  (:'LOCAL_TENANT_ID', 'jingzhicommunity', 'bge-reranker-large', 'rerank', true, NOW(), NOW()),

  -- System provider models
  (:'LOCAL_TENANT_ID', 'zgi/openai/openai', 'text-embedding-3-large', 'text-embedding', true, NOW(), NOW()),
  (:'LOCAL_TENANT_ID', 'zgi/agicto/agicto', 'gpt-4', 'llm', true, NOW(), NOW()),
  (:'LOCAL_TENANT_ID', 'zgi/tongyi/tongyi', 'qwen-turbo', 'llm', true, NOW(), NOW()),
  (:'LOCAL_TENANT_ID', 'zgi/tongyi/tongyi', 'qwen-plus', 'llm', true, NOW(), NOW()),
  (:'LOCAL_TENANT_ID', 'zgi/tongyi/tongyi', 'qwen-max', 'llm', true, NOW(), NOW())
ON CONFLICT (tenant_id, provider_name, model_name, model_type) DO NOTHING;

COMMIT;

-- Verification queries
\echo ''
\echo '========== Sync Completed =========='
\echo ''
\echo 'Provider Summary:'
SELECT provider_name, COUNT(*) as model_count
FROM provider_models
WHERE tenant_id = :'LOCAL_TENANT_ID'
GROUP BY provider_name
ORDER BY model_count DESC;

\echo ''
\echo 'Model Type Summary:'
SELECT model_type, COUNT(*) as count
FROM provider_models
WHERE tenant_id = :'LOCAL_TENANT_ID'
GROUP BY model_type
ORDER BY count DESC;

\echo ''
\echo 'Total Statistics:'
SELECT
  (SELECT COUNT(DISTINCT provider_name) FROM providers WHERE tenant_id = :'LOCAL_TENANT_ID') as total_providers,
  (SELECT COUNT(DISTINCT provider_name) FROM providers WHERE tenant_id = :'LOCAL_TENANT_ID' AND is_valid = true) as active_providers,
  (SELECT COUNT(*) FROM provider_models WHERE tenant_id = :'LOCAL_TENANT_ID' AND model_type IN ('llm', 'text-generation')) as llm_models,
  (SELECT COUNT(*) FROM provider_models WHERE tenant_id = :'LOCAL_TENANT_ID' AND model_type IN ('text-embedding', 'embeddings')) as embedding_models,
  (SELECT COUNT(*) FROM provider_models WHERE tenant_id = :'LOCAL_TENANT_ID' AND model_type IN ('rerank', 'reranking')) as rerank_models;
