--
-- PostgreSQL database dump
--

\restrict OammFOhMfJ77tJrtSGBSTnMOZKJdviy5nbhfAVzDHdDQpXfVWoFzbdduU9e7M74

-- Dumped from database version 17.6
-- Dumped by pg_dump version 17.6

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: bank_transfer_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.bank_transfer_requests (
    id character varying(255) NOT NULL,
    request_no character varying(50) NOT NULL,
    account_id uuid NOT NULL,
    group_id uuid NOT NULL,
    amount numeric(12,2) NOT NULL,
    currency character varying(10) DEFAULT 'CNY'::character varying NOT NULL,
    voucher_key character varying(255) NOT NULL,
    remark text,
    status character varying(30) DEFAULT 'pending'::character varying NOT NULL,
    reviewed_by uuid,
    reviewed_at timestamp with time zone,
    reject_reason text,
    completed_at timestamp with time zone,
    canceled_at timestamp with time zone,
    cancel_reason text,
    client_ip character varying(45),
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT chk_btr_amount CHECK ((amount > (0)::numeric))
);


--
-- Name: TABLE bank_transfer_requests; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.bank_transfer_requests IS 'Bank transfer recharge requests table';


--
-- Name: COLUMN bank_transfer_requests.request_no; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.bank_transfer_requests.request_no IS 'User-visible request number';


--
-- Name: COLUMN bank_transfer_requests.voucher_key; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.bank_transfer_requests.voucher_key IS 'Voucher storage key (object storage path)';


--
-- Name: COLUMN bank_transfer_requests.status; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.bank_transfer_requests.status IS 'Request status: pending/approved/rejected/canceled';


--
-- Name: COLUMN bank_transfer_requests.reviewed_by; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.bank_transfer_requests.reviewed_by IS 'Reviewer account ID';


--
-- Name: COLUMN bank_transfer_requests.reviewed_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.bank_transfer_requests.reviewed_at IS 'Review timestamp';


--
-- Name: COLUMN bank_transfer_requests.completed_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.bank_transfer_requests.completed_at IS 'Completion timestamp (when balance is credited)';


--
-- Name: billing_attempt_entries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.billing_attempt_entries (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    attempt_id character varying(120) NOT NULL,
    entry_type character varying(20) NOT NULL,
    ledger_type character varying(30) NOT NULL,
    ledger_ref_id character varying(120) NOT NULL,
    reserved_amount bigint DEFAULT 0 NOT NULL,
    actual_amount bigint DEFAULT 0 NOT NULL,
    refunded_amount bigint DEFAULT 0 NOT NULL,
    status character varying(20) NOT NULL,
    error_code character varying(100),
    error_message text,
    idempotency_key character varying(160),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: billing_attempts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.billing_attempts (
    attempt_id character varying(120) NOT NULL,
    request_id character varying(100) NOT NULL,
    organization_id uuid NOT NULL,
    lane character varying(20) NOT NULL,
    route_id uuid,
    provider_id uuid,
    model_id uuid,
    quota_subject_type character varying(20) NOT NULL,
    quota_subject_id character varying(64) NOT NULL,
    status character varying(30) NOT NULL,
    invocation_result character varying(20),
    error_code character varying(100),
    error_message text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    reconcile_attempts integer DEFAULT 0 NOT NULL,
    next_reconcile_at timestamp with time zone,
    last_reconcile_at timestamp with time zone
);


--
-- Name: channel_wallet_transactions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.channel_wallet_transactions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    channel_id uuid NOT NULL,
    attempt_id character varying(120),
    type character varying(40) NOT NULL,
    amount bigint NOT NULL,
    balance_before bigint NOT NULL,
    balance_after bigint NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: channel_wallets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.channel_wallets (
    channel_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    balance bigint DEFAULT 0 NOT NULL,
    status character varying(20) DEFAULT 'ACTIVE'::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: group_ai_credit_accounts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.group_ai_credit_accounts (
    id character varying(255) NOT NULL,
    account_id uuid NOT NULL,
    group_id uuid NOT NULL,
    purchased_credits bigint DEFAULT 0 NOT NULL,
    total_earned bigint DEFAULT 0 NOT NULL,
    total_spent bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: TABLE group_ai_credit_accounts; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.group_ai_credit_accounts IS 'Group AI credit accounts table';


--
-- Name: COLUMN group_ai_credit_accounts.account_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.group_ai_credit_accounts.account_id IS 'Account ID (redundant for query convenience)';


--
-- Name: COLUMN group_ai_credit_accounts.group_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.group_ai_credit_accounts.group_id IS 'Group ID, credit account is for a specific group';


--
-- Name: COLUMN group_ai_credit_accounts.purchased_credits; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.group_ai_credit_accounts.purchased_credits IS 'Purchased credits balance (no reset)';


--
-- Name: COLUMN group_ai_credit_accounts.total_earned; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.group_ai_credit_accounts.total_earned IS 'Total credits earned (cumulative)';


--
-- Name: COLUMN group_ai_credit_accounts.total_spent; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.group_ai_credit_accounts.total_spent IS 'Total credits spent (cumulative)';


--
-- Name: group_wallets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.group_wallets (
    id character varying(255) NOT NULL,
    account_id uuid NOT NULL,
    group_id uuid NOT NULL,
    currency character varying(10) DEFAULT 'CNY'::character varying NOT NULL,
    balance numeric(12,2) DEFAULT 0 NOT NULL,
    frozen_balance numeric(12,2) DEFAULT 0 NOT NULL,
    total_recharged numeric(12,2) DEFAULT 0 NOT NULL,
    total_consumed numeric(12,2) DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: TABLE group_wallets; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.group_wallets IS 'Group wallets table for cash balance management';


--
-- Name: COLUMN group_wallets.account_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.group_wallets.account_id IS 'Account ID (redundant for query convenience)';


--
-- Name: COLUMN group_wallets.currency; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.group_wallets.currency IS 'Currency: CNY, USD';


--
-- Name: COLUMN group_wallets.balance; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.group_wallets.balance IS 'Available balance';


--
-- Name: COLUMN group_wallets.frozen_balance; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.group_wallets.frozen_balance IS 'Frozen balance (pending review, refund processing, etc.)';


--
-- Name: COLUMN group_wallets.total_recharged; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.group_wallets.total_recharged IS 'Total recharged amount (cumulative)';


--
-- Name: COLUMN group_wallets.total_consumed; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.group_wallets.total_consumed IS 'Total consumed amount (cumulative)';


--
-- Name: llm_catalog_sync_states; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.llm_catalog_sync_states (
    sync_key character varying(50) NOT NULL,
    last_applied_version bigint DEFAULT 0 NOT NULL,
    last_applied_at timestamp with time zone,
    last_error text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: llm_credentials; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.llm_credentials (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    organization_id uuid,
    name character varying(100),
    provider character varying(50) NOT NULL,
    api_key_ciphertext text NOT NULL,
    api_key_hash character varying(64),
    api_base_url character varying(500),
    is_active boolean DEFAULT true,
    last_used_at timestamp with time zone,
    expires_at timestamp with time zone,
    metadata jsonb DEFAULT '{}'::jsonb,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    deleted_at timestamp with time zone,
    protocol character varying(50)
);


--
-- Name: TABLE llm_credentials; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.llm_credentials IS 'Centralized credential storage for LLM API keys';


--
-- Name: COLUMN llm_credentials.organization_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.llm_credentials.organization_id IS 'NULL = system credential, otherwise user credential';


--
-- Name: COLUMN llm_credentials.api_key_ciphertext; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.llm_credentials.api_key_ciphertext IS 'AES-256-GCM encrypted API key';


--
-- Name: llm_custom_models; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.llm_custom_models (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    organization_id uuid NOT NULL,
    provider_id uuid NOT NULL,
    name character varying(100) NOT NULL,
    display_name character varying(200) NOT NULL,
    context_window integer,
    max_output_tokens integer,
    input_price numeric(10,4) DEFAULT 0,
    output_price numeric(10,4) DEFAULT 0,
    supports_vision boolean DEFAULT false,
    supports_tool_call boolean DEFAULT false,
    supports_streaming boolean DEFAULT true,
    supports_reasoning boolean DEFAULT false,
    knowledge_cutoff character varying(20),
    description text,
    is_active boolean DEFAULT true,
    sort_order integer DEFAULT 0,
    metadata jsonb DEFAULT '{}'::jsonb,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    deleted_at timestamp with time zone,
    use_cases text[] DEFAULT '{}'::text[] NOT NULL,
    max_input_tokens integer DEFAULT 0,
    slug character varying(100),
    family character varying(100),
    family_slug character varying(100),
    status character varying(20) DEFAULT 'active'::character varying,
    tagline text,
    is_flagship boolean DEFAULT false,
    is_featured boolean DEFAULT false,
    is_new boolean DEFAULT false,
    access_type character varying(20) DEFAULT 'closed'::character varying,
    currency character varying(10) DEFAULT 'USD'::character varying,
    chat_completions boolean DEFAULT true,
    embeddings boolean DEFAULT false,
    streaming boolean DEFAULT true,
    structured_output boolean DEFAULT false,
    json_mode boolean DEFAULT false,
    input_modalities jsonb DEFAULT '["text"]'::jsonb,
    output_modalities jsonb DEFAULT '["text"]'::jsonb,
    supported_parameters jsonb DEFAULT '[]'::jsonb,
    legacy_function_call boolean DEFAULT false NOT NULL,
    provider character varying(100) DEFAULT ''::character varying,
    vision boolean DEFAULT false,
    function_calling boolean DEFAULT false,
    reasoning boolean DEFAULT false,
    audio boolean DEFAULT false,
    image_generation boolean DEFAULT false,
    speech_generation boolean DEFAULT false,
    transcription boolean DEFAULT false,
    translation boolean DEFAULT false,
    moderation boolean DEFAULT false,
    realtime boolean DEFAULT false,
    batch boolean DEFAULT false,
    fine_tuning boolean DEFAULT false,
    assistants boolean DEFAULT false,
    responses boolean DEFAULT false,
    distillation boolean DEFAULT false,
    system_prompt boolean DEFAULT true,
    logprobs boolean DEFAULT false,
    web_search boolean DEFAULT false,
    file_search boolean DEFAULT false,
    code_interpreter boolean DEFAULT false,
    computer_use boolean DEFAULT false,
    mcp boolean DEFAULT false,
    reasoning_effort boolean DEFAULT false,
    parallel_tool_calls boolean DEFAULT false,
    temperature boolean DEFAULT true,
    top_p boolean DEFAULT true,
    presence_penalty boolean DEFAULT false,
    frequency_penalty boolean DEFAULT false,
    logit_bias boolean DEFAULT false,
    seed boolean DEFAULT false,
    stop boolean DEFAULT true,
    max_stop_sequences integer DEFAULT 4,
    default_parameters jsonb DEFAULT '{}'::jsonb
);


--
-- Name: COLUMN llm_custom_models.use_cases; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.llm_custom_models.use_cases IS 'Usage scenarios for tenant custom models';


--
-- Name: llm_custom_providers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.llm_custom_providers (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    organization_id uuid NOT NULL,
    provider character varying(50) NOT NULL,
    provider_name character varying(100) NOT NULL,
    api_base_url character varying(255),
    protocol character varying(50) DEFAULT 'openai'::character varying,
    logo_url character varying(255),
    documentation_url character varying(255),
    description text,
    is_active boolean DEFAULT true,
    sort_order integer DEFAULT 0,
    metadata jsonb DEFAULT '{}'::jsonb,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    deleted_at timestamp with time zone
);


--
-- Name: llm_model_configs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.llm_model_configs (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    organization_id uuid NOT NULL,
    model_id uuid NOT NULL,
    is_enabled boolean DEFAULT true,
    custom_display_name character varying(200),
    input_price_override numeric(10,4),
    output_price_override numeric(10,4),
    access_scope character varying(20) DEFAULT 'all'::character varying,
    visible_groups jsonb DEFAULT '[]'::jsonb,
    visible_users jsonb DEFAULT '[]'::jsonb,
    sort_order integer DEFAULT 0,
    metadata jsonb DEFAULT '{}'::jsonb,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    deleted_at timestamp without time zone,
    CONSTRAINT chk_access_scope CHECK (((access_scope)::text = ANY (ARRAY[('all'::character varying)::text, ('group'::character varying)::text, ('user'::character varying)::text])))
);


--
-- Name: llm_models; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.llm_models (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    provider character varying(100) NOT NULL,
    name character varying(100) NOT NULL,
    display_name character varying(200) NOT NULL,
    attachment boolean DEFAULT false,
    reasoning boolean DEFAULT false,
    function_calling boolean DEFAULT false,
    structured_output boolean DEFAULT false,
    temperature boolean DEFAULT true,
    knowledge_cutoff character varying(20),
    release_date date,
    last_updated date,
    input_modalities jsonb,
    output_modalities jsonb,
    open_weights boolean DEFAULT false,
    input_price numeric(10,4),
    output_price numeric(10,4),
    cost_cache_read numeric(10,4),
    cost_cache_write numeric(10,4),
    cost_context_over_200k jsonb,
    context_window integer,
    max_output_tokens integer,
    is_active boolean DEFAULT true,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    deleted_at timestamp with time zone,
    description text,
    tokenizer character varying(100),
    instruct_type character varying(100),
    cost_image numeric(10,4),
    cost_audio numeric(10,4),
    supported_parameters jsonb,
    default_parameters jsonb,
    is_moderated boolean DEFAULT false,
    is_finetuned boolean DEFAULT false,
    cost_rate jsonb DEFAULT '{"audio": 1, "image": 1, "input": 1, "output": 1}'::jsonb,
    is_system_enabled boolean DEFAULT true NOT NULL,
    sort_order integer DEFAULT 0,
    vision boolean DEFAULT false,
    model_tier character varying(20) DEFAULT NULL::character varying,
    is_recommended boolean DEFAULT false,
    temperature_min numeric(4,2) DEFAULT 0,
    temperature_max numeric(4,2) DEFAULT 2,
    temperature_default numeric(4,2) DEFAULT 1,
    audio boolean DEFAULT false NOT NULL,
    legacy_function_call boolean DEFAULT false NOT NULL,
    json_mode boolean DEFAULT false NOT NULL,
    streaming boolean DEFAULT true NOT NULL,
    use_cases text[] DEFAULT '{}'::text[] NOT NULL,
    max_input_tokens integer DEFAULT 0,
    slug character varying(100),
    family character varying(100),
    family_slug character varying(100),
    status character varying(20) DEFAULT 'active'::character varying,
    tagline text,
    is_flagship boolean DEFAULT false,
    is_featured boolean DEFAULT false,
    is_new boolean DEFAULT false,
    access_type character varying(20) DEFAULT 'closed'::character varying,
    currency character varying(10) DEFAULT 'USD'::character varying,
    chat_completions boolean DEFAULT true,
    embeddings boolean DEFAULT false,
    image_generation boolean DEFAULT false,
    speech_generation boolean DEFAULT false,
    transcription boolean DEFAULT false,
    moderation boolean DEFAULT false,
    realtime boolean DEFAULT false,
    batch boolean DEFAULT false,
    fine_tuning boolean DEFAULT false,
    assistants boolean DEFAULT false,
    responses boolean DEFAULT false,
    distillation boolean DEFAULT false,
    system_prompt boolean DEFAULT true,
    logprobs boolean DEFAULT false,
    web_search boolean DEFAULT false,
    file_search boolean DEFAULT false,
    code_interpreter boolean DEFAULT false,
    computer_use boolean DEFAULT false,
    mcp boolean DEFAULT false,
    seed boolean DEFAULT false,
    stop boolean DEFAULT true,
    max_stop_sequences integer DEFAULT 4,
    top_p boolean DEFAULT true,
    presence_penalty boolean DEFAULT false,
    frequency_penalty boolean DEFAULT false,
    logit_bias boolean DEFAULT false,
    parallel_tool_calls boolean DEFAULT true,
    reasoning_effort boolean DEFAULT false,
    protocol_config jsonb DEFAULT '[]'::jsonb,
    family_name character varying(200),
    parent_id uuid,
    family_default boolean DEFAULT false,
    cached_input_price numeric(10,4),
    videos boolean DEFAULT false,
    image_edit boolean DEFAULT false,
    translation boolean DEFAULT false,
    is_configured boolean DEFAULT false,
    image_prices jsonb DEFAULT '[]'::jsonb
);


--
-- Name: COLUMN llm_models.reasoning; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.llm_models.reasoning IS 'Has reasoning/thinking capabilities';


--
-- Name: COLUMN llm_models.function_calling; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.llm_models.function_calling IS 'Supports function/tool calling';


--
-- Name: COLUMN llm_models.input_price; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.llm_models.input_price IS 'Input token price per million tokens (OpenAI naming)';


--
-- Name: COLUMN llm_models.output_price; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.llm_models.output_price IS 'Output token price per million tokens (OpenAI naming)';


--
-- Name: COLUMN llm_models.context_window; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.llm_models.context_window IS 'Maximum context window size in tokens (OpenAI naming)';


--
-- Name: COLUMN llm_models.max_output_tokens; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.llm_models.max_output_tokens IS 'Maximum output tokens (OpenAI naming)';


--
-- Name: COLUMN llm_models.is_system_enabled; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.llm_models.is_system_enabled IS 'System-level control: whether this model is available for tenants to use';


--
-- Name: COLUMN llm_models.vision; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.llm_models.vision IS 'Supports image/vision input';


--
-- Name: COLUMN llm_models.use_cases; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.llm_models.use_cases IS 'Usage scenarios: text-chat, vision, image-gen, embedding, rerank, speech-to-text, text-to-speech, realtime-audio, video-gen, moderation, reasoning, function-calling';


--
-- Name: COLUMN llm_models.max_input_tokens; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.llm_models.max_input_tokens IS 'Maximum input tokens (OpenAI naming)';


--
-- Name: COLUMN llm_models.slug; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.llm_models.slug IS 'URL-friendly model identifier';


--
-- Name: COLUMN llm_models.family; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.llm_models.family IS 'Model family (e.g., GPT-4, Claude)';


--
-- Name: COLUMN llm_models.status; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.llm_models.status IS 'Model status: active, deprecated';


--
-- Name: COLUMN llm_models.chat_completions; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.llm_models.chat_completions IS 'Supports chat completions endpoint';


--
-- Name: COLUMN llm_models.protocol_config; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.llm_models.protocol_config IS 'Protocol configuration array in JSONB format';


--
-- Name: llm_official_model_snapshots; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.llm_official_model_snapshots (
    source_key character varying(50) NOT NULL,
    effective_models jsonb DEFAULT '[]'::jsonb NOT NULL,
    latest_models jsonb DEFAULT '[]'::jsonb NOT NULL,
    previous_models jsonb DEFAULT '[]'::jsonb NOT NULL,
    latest_event_version bigint DEFAULT 0 NOT NULL,
    latest_synced_at timestamp with time zone,
    effective_updated_at timestamp with time zone,
    last_check_status character varying(20) DEFAULT 'accepted'::character varying NOT NULL,
    last_reject_reason text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: llm_organization_api_keys; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.llm_organization_api_keys (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    organization_id uuid NOT NULL,
    key character varying(255) NOT NULL,
    name character varying(255) NOT NULL,
    status character varying(20) DEFAULT 'active'::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    accessed_at timestamp with time zone,
    expires_at timestamp with time zone,
    deleted_at timestamp with time zone,
    used_quota bigint DEFAULT 0 NOT NULL,
    remain_quota bigint DEFAULT 0 NOT NULL,
    quota_limit bigint,
    model_limits_enabled boolean DEFAULT false NOT NULL,
    model_limits jsonb,
    allow_ips text DEFAULT ''::text NOT NULL,
    key_hash character varying(64),
    is_internal boolean DEFAULT false NOT NULL,
    CONSTRAINT chk_llm_tenant_api_keys_quota CHECK (((used_quota >= 0) AND (remain_quota >= 0))),
    CONSTRAINT chk_llm_tenant_api_keys_quota_limit CHECK (((quota_limit IS NULL) OR (quota_limit > 0))),
    CONSTRAINT chk_llm_tenant_api_keys_status CHECK (((status)::text = ANY (ARRAY[('active'::character varying)::text, ('inactive'::character varying)::text, ('revoked'::character varying)::text])))
);


--
-- Name: llm_providers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.llm_providers (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    provider character varying(50) NOT NULL,
    provider_name character varying(100) NOT NULL,
    api_base_url character varying(255),
    env_keys jsonb,
    npm_package character varying(100),
    documentation_url character varying(255),
    logo_url character varying(255),
    api_key text,
    balance numeric(15,4) DEFAULT 0.0000,
    currency character varying(10) DEFAULT 'USD'::character varying,
    is_active boolean DEFAULT true,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    deleted_at timestamp with time zone,
    description text,
    openai_compatible boolean DEFAULT false,
    is_system_enabled boolean DEFAULT true NOT NULL,
    sort_order integer DEFAULT 0,
    metadata jsonb DEFAULT '{}'::jsonb,
    protocol character varying(50),
    fallback_protocol character varying(50),
    website character varying(255),
    pricing_url character varying(255),
    tagline character varying(500),
    country_code character varying(10),
    founded_year integer DEFAULT 0
);


--
-- Name: COLUMN llm_providers.is_system_enabled; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.llm_providers.is_system_enabled IS 'System-level control: whether this provider is available for tenants to use';


--
-- Name: llm_routes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.llm_routes (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    organization_id uuid NOT NULL,
    type character varying(20) NOT NULL,
    user_credential_id uuid,
    name character varying(255),
    provider character varying(100),
    protocol character varying(50),
    models jsonb DEFAULT '[]'::jsonb,
    api_base_url character varying(500),
    model_maps jsonb DEFAULT '{}'::jsonb,
    param_override jsonb DEFAULT '{}'::jsonb,
    header_override jsonb DEFAULT '{}'::jsonb,
    priority integer DEFAULT 0 NOT NULL,
    weight integer DEFAULT 1 NOT NULL,
    is_enabled boolean DEFAULT true NOT NULL,
    auto_ban boolean DEFAULT false,
    balance numeric(15,4) DEFAULT 0,
    currency character varying(10) DEFAULT 'USD'::character varying,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    deleted_at timestamp with time zone,
    credential_id uuid,
    tags jsonb DEFAULT '[]'::jsonb,
    description text,
    match_models text[] DEFAULT '{}'::text[],
    route_type character varying(20) DEFAULT 'PRIVATE'::character varying,
    credential_ids uuid[] DEFAULT '{}'::uuid[],
    load_balance_strategy character varying(20) DEFAULT 'round_robin'::character varying,
    model_rewrite_map jsonb DEFAULT '{}'::jsonb,
    base_url character varying(255),
    is_official boolean DEFAULT false,
    status character varying(20) DEFAULT 'active'::character varying,
    status_reason text,
    status_changed_at timestamp with time zone,
    circuit_breaker_threshold integer DEFAULT 5,
    circuit_breaker_window_seconds integer DEFAULT 60,
    circuit_breaker_cooldown_seconds integer DEFAULT 30,
    rate_limit_rpm integer,
    rate_limit_tpm bigint,
    timeout_seconds integer DEFAULT 60,
    retry_count integer DEFAULT 3,
    retry_delay_ms integer DEFAULT 1000,
    created_by uuid,
    updated_by uuid,
    supported_protocols jsonb DEFAULT '[]'::jsonb,
    validation_report jsonb DEFAULT '{}'::jsonb,
    sync_mode character varying(20) DEFAULT 'snapshot'::character varying,
    last_synced_at timestamp without time zone,
    fallback_protocol character varying(50),
    protocol_source character varying(20) DEFAULT 'default'::character varying,
    success_count integer DEFAULT 0,
    failure_count integer DEFAULT 0,
    avg_latency_ms integer DEFAULT 0,
    last_health_check_at timestamp without time zone,
    CONSTRAINT chk_load_balance CHECK (((load_balance_strategy IS NULL) OR ((load_balance_strategy)::text = ANY (ARRAY[('round_robin'::character varying)::text, ('random'::character varying)::text, ('weighted'::character varying)::text])))),
    CONSTRAINT chk_route_status CHECK (((status IS NULL) OR ((status)::text = ANY (ARRAY[('active'::character varying)::text, ('disabled'::character varying)::text, ('banned'::character varying)::text, ('maintenance'::character varying)::text])))),
    CONSTRAINT chk_route_type CHECK (((type)::text = ANY (ARRAY[('ZGI_CLOUD'::character varying)::text, ('PRIVATE'::character varying)::text]))),
    CONSTRAINT chk_route_type_new CHECK (((route_type IS NULL) OR ((route_type)::text = ANY (ARRAY[('PRIVATE'::character varying)::text, ('ZGI_CLOUD'::character varying)::text])))),
    CONSTRAINT chk_system_ref CHECK (((((type)::text = 'ZGI_CLOUD'::text) AND (is_official = true)) OR (((type)::text = 'PRIVATE'::text) AND (user_credential_id IS NOT NULL))))
);


--
-- Name: TABLE llm_routes; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.llm_routes IS 'Tenant routing configuration for load balancing';


--
-- Name: COLUMN llm_routes.type; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.llm_routes.type IS 'ZGI_CLOUD = ZGI official cloud service channel, PRIVATE = user private channel';
--
-- Name: llm_tenant_models; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.llm_tenant_models (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    tenant_id uuid NOT NULL,
    provider character varying(100) NOT NULL,
    model character varying(100) NOT NULL,
    is_enabled boolean DEFAULT true,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    deleted_at timestamp with time zone
);


--
-- Name: TABLE llm_tenant_models; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.llm_tenant_models IS 'DEPRECATED: Use llm_tenant_model_configs instead. This table will be removed in a future version.';


--
-- Name: llm_workspace_quotas; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.llm_workspace_quotas (
    workspace_id character varying(255) NOT NULL,
    organization_id uuid NOT NULL,
    used_quota bigint DEFAULT 0 NOT NULL,
    remain_quota bigint DEFAULT 0 NOT NULL,
    quota_limit bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT chk_llm_workspace_quotas_limit_positive CHECK (((quota_limit IS NULL) OR (quota_limit > 0))),
    CONSTRAINT chk_llm_workspace_quotas_used_non_negative CHECK ((used_quota >= 0))
);


--
-- Name: orders; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.orders (
    id character varying(255) NOT NULL,
    order_no character varying(50) NOT NULL,
    account_id uuid NOT NULL,
    group_id uuid NOT NULL,
    order_type character varying(30) NOT NULL,
    product_code character varying(50) NOT NULL,
    product_type character varying(30) NOT NULL,
    product_snapshot jsonb NOT NULL,
    original_amount numeric(10,2) NOT NULL,
    discount_amount numeric(10,2) DEFAULT 0 NOT NULL,
    final_amount numeric(10,2) NOT NULL,
    currency character varying(10) DEFAULT 'CNY'::character varying NOT NULL,
    discount_details jsonb,
    status character varying(30) DEFAULT 'pending'::character varying NOT NULL,
    paid_at timestamp with time zone,
    completed_at timestamp with time zone,
    failed_at timestamp with time zone,
    failure_reason text,
    canceled_at timestamp with time zone,
    cancel_reason text,
    refunded_at timestamp with time zone,
    refund_reason text,
    subscription_id character varying(255),
    client_ip character varying(45),
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: TABLE orders; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.orders IS 'Orders table';


--
-- Name: COLUMN orders.group_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.orders.group_id IS 'Group ID, order is for a specific group under the user';


--
-- Name: COLUMN orders.order_type; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.orders.order_type IS 'Order type: subscription_new, subscription_renew, subscription_upgrade, credit_purchase';


--
-- Name: COLUMN orders.status; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.orders.status IS 'Order status: pending, paid, completed, failed, canceled, refunded';


--
-- Name: payment_callbacks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.payment_callbacks (
    id character varying(255) NOT NULL,
    transaction_id character varying(255) NOT NULL,
    payment_method character varying(30) NOT NULL,
    callback_type character varying(20) NOT NULL,
    request_headers jsonb,
    request_body text,
    response_status integer,
    response_body text,
    is_verified boolean DEFAULT false NOT NULL,
    verification_error text,
    processed boolean DEFAULT false NOT NULL,
    processed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: TABLE payment_callbacks; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.payment_callbacks IS 'Payment callbacks log table';


--
-- Name: COLUMN payment_callbacks.callback_type; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.payment_callbacks.callback_type IS 'Callback type: notify, return';


--
-- Name: payment_transactions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.payment_transactions (
    id character varying(255) NOT NULL,
    transaction_no character varying(100) NOT NULL,
    order_id character varying(255) NOT NULL,
    account_id uuid NOT NULL,
    payment_method character varying(30) NOT NULL,
    amount numeric(10,2) NOT NULL,
    currency character varying(10) DEFAULT 'CNY'::character varying NOT NULL,
    status character varying(20) DEFAULT 'pending'::character varying NOT NULL,
    provider_transaction_id character varying(255),
    provider_response jsonb,
    paid_at timestamp with time zone,
    failed_at timestamp with time zone,
    failure_reason text,
    client_ip character varying(45),
    user_agent text,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: TABLE payment_transactions; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.payment_transactions IS 'Payment transactions table';


--
-- Name: COLUMN payment_transactions.status; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.payment_transactions.status IS 'Status: pending, processing, success, failed, canceled';


--
-- Name: quota_usage_history; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.quota_usage_history (
    id character varying(255) NOT NULL,
    group_id uuid NOT NULL,
    account_id uuid NOT NULL,
    tenant_id uuid,
    resource_type character varying(50) NOT NULL,
    operation_type character varying(20) NOT NULL,
    delta bigint NOT NULL,
    value_before bigint NOT NULL,
    value_after bigint NOT NULL,
    resource_id character varying(255),
    resource_name character varying(500),
    metadata jsonb,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: TABLE quota_usage_history; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.quota_usage_history IS '配额使用历史记录表';


--
-- Name: COLUMN quota_usage_history.group_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.quota_usage_history.group_id IS '组织ID';


--
-- Name: COLUMN quota_usage_history.account_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.quota_usage_history.account_id IS '操作账号ID';


--
-- Name: COLUMN quota_usage_history.tenant_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.quota_usage_history.tenant_id IS '部门ID(可选)';


--
-- Name: COLUMN quota_usage_history.resource_type; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.quota_usage_history.resource_type IS '资源类型: seats, storage, db_rows, knowledge_bases, ai_agents, workflows, workflow_executions';


--
-- Name: COLUMN quota_usage_history.operation_type; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.quota_usage_history.operation_type IS '操作类型: increase(增加), decrease(减少)';


--
-- Name: COLUMN quota_usage_history.delta; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.quota_usage_history.delta IS '变化量,正数表示增加,负数表示减少';


--
-- Name: COLUMN quota_usage_history.value_before; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.quota_usage_history.value_before IS '变化前的值';


--
-- Name: COLUMN quota_usage_history.value_after; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.quota_usage_history.value_after IS '变化后的值';


--
-- Name: COLUMN quota_usage_history.resource_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.quota_usage_history.resource_id IS '关联的资源ID(如文件ID、知识库ID等)';


--
-- Name: COLUMN quota_usage_history.resource_name; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.quota_usage_history.resource_name IS '资源名称';


--
-- Name: COLUMN quota_usage_history.metadata; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.quota_usage_history.metadata IS '详细元数据,JSON格式,包含操作的详细信息';


--
-- Name: refund_records; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.refund_records (
    id character varying(255) NOT NULL,
    refund_no character varying(100) NOT NULL,
    order_id character varying(255) NOT NULL,
    transaction_id character varying(255) NOT NULL,
    account_id uuid NOT NULL,
    refund_amount numeric(10,2) NOT NULL,
    currency character varying(10) DEFAULT 'CNY'::character varying NOT NULL,
    refund_fee numeric(10,2) DEFAULT 0 NOT NULL,
    original_transaction_amount numeric(10,2) NOT NULL,
    refund_reason text NOT NULL,
    status character varying(20) DEFAULT 'pending'::character varying NOT NULL,
    provider_refund_id character varying(255),
    provider_response jsonb,
    processing_at timestamp with time zone,
    success_at timestamp with time zone,
    failed_at timestamp with time zone,
    failure_reason text,
    operator_id uuid,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: TABLE refund_records; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.refund_records IS 'Refund records table';


--
-- Name: COLUMN refund_records.status; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.refund_records.status IS 'Status: pending, processing, success, failed';


--
-- Name: sales_contact_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sales_contact_requests (
    id character varying(255) NOT NULL,
    account_id uuid,
    company_name character varying(255) NOT NULL,
    contact_name character varying(100) NOT NULL,
    phone character varying(50) NOT NULL,
    email character varying(255),
    extra_meta jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: transactions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.transactions (
    id character varying(255) NOT NULL,
    batch_id character varying(255) NOT NULL,
    group_id uuid NOT NULL,
    tenant_id uuid,
    currency_type character varying(20) NOT NULL,
    transaction_type character varying(30) NOT NULL,
    amount numeric(16,4) NOT NULL,
    balance_before numeric(16,4) NOT NULL,
    balance_after numeric(16,4) NOT NULL,
    currency character varying(10),
    reference_type character varying(50),
    reference_id character varying(255),
    description character varying(500),
    transaction_detail jsonb,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: TABLE transactions; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.transactions IS 'Unified transaction records table for cash and credits';


--
-- Name: COLUMN transactions.batch_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.transactions.batch_id IS 'Batch ID, multiple records from same business operation share this ID';


--
-- Name: COLUMN transactions.currency_type; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.transactions.currency_type IS 'Currency type: cash / subscription_credits / purchased_credits';


--
-- Name: COLUMN transactions.transaction_type; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.transactions.transaction_type IS 'Transaction type: recharge, subscription_payment, ai_usage, etc.';


--
-- Name: COLUMN transactions.amount; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.transactions.amount IS 'Amount change (positive for income, negative for expense)';


--
-- Name: COLUMN transactions.reference_type; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.transactions.reference_type IS 'Reference type: order / subscription / refund';


--
-- Name: bank_transfer_requests bank_transfer_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bank_transfer_requests
    ADD CONSTRAINT bank_transfer_requests_pkey PRIMARY KEY (id);


--
-- Name: billing_attempt_entries billing_attempt_entries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.billing_attempt_entries
    ADD CONSTRAINT billing_attempt_entries_pkey PRIMARY KEY (id);


--
-- Name: billing_attempts billing_attempts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.billing_attempts
    ADD CONSTRAINT billing_attempts_pkey PRIMARY KEY (attempt_id);


--
-- Name: channel_wallet_transactions channel_wallet_transactions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.channel_wallet_transactions
    ADD CONSTRAINT channel_wallet_transactions_pkey PRIMARY KEY (id);


--
-- Name: channel_wallets channel_wallets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.channel_wallets
    ADD CONSTRAINT channel_wallets_pkey PRIMARY KEY (channel_id);


--
-- Name: group_ai_credit_accounts group_ai_credit_accounts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.group_ai_credit_accounts
    ADD CONSTRAINT group_ai_credit_accounts_pkey PRIMARY KEY (id);


--
-- Name: group_wallets group_wallets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.group_wallets
    ADD CONSTRAINT group_wallets_pkey PRIMARY KEY (id);


--
-- Name: llm_catalog_sync_states llm_catalog_sync_states_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.llm_catalog_sync_states
    ADD CONSTRAINT llm_catalog_sync_states_pkey PRIMARY KEY (sync_key);

--
-- Name: llm_credentials llm_credentials_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.llm_credentials
    ADD CONSTRAINT llm_credentials_pkey PRIMARY KEY (id);


--
-- Name: llm_models llm_models_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.llm_models
    ADD CONSTRAINT llm_models_pkey PRIMARY KEY (id);


--
-- Name: llm_official_model_snapshots llm_official_model_snapshots_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.llm_official_model_snapshots
    ADD CONSTRAINT llm_official_model_snapshots_pkey PRIMARY KEY (source_key);


--
-- Name: llm_providers llm_providers_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.llm_providers
    ADD CONSTRAINT llm_providers_name_key UNIQUE (provider);


--
-- Name: llm_providers llm_providers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.llm_providers
    ADD CONSTRAINT llm_providers_pkey PRIMARY KEY (id);


-- Name: llm_organization_api_keys llm_tenant_api_keys_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.llm_organization_api_keys
    ADD CONSTRAINT llm_tenant_api_keys_pkey PRIMARY KEY (id);


--
-- Name: llm_custom_models llm_tenant_custom_models_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.llm_custom_models
    ADD CONSTRAINT llm_tenant_custom_models_pkey PRIMARY KEY (id);


--
-- Name: llm_custom_providers llm_tenant_custom_providers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.llm_custom_providers
    ADD CONSTRAINT llm_tenant_custom_providers_pkey PRIMARY KEY (id);


--
-- Name: llm_model_configs llm_tenant_model_configs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.llm_model_configs
    ADD CONSTRAINT llm_tenant_model_configs_pkey PRIMARY KEY (id);


--
-- Name: llm_tenant_models llm_tenant_models_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.llm_tenant_models
    ADD CONSTRAINT llm_tenant_models_pkey PRIMARY KEY (id);


--
-- Name: llm_routes llm_tenant_routes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.llm_routes
    ADD CONSTRAINT llm_tenant_routes_pkey PRIMARY KEY (id);


--
-- Name: llm_workspace_quotas llm_workspace_quotas_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.llm_workspace_quotas
    ADD CONSTRAINT llm_workspace_quotas_pkey PRIMARY KEY (workspace_id);


--
-- Name: orders orders_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orders
    ADD CONSTRAINT orders_pkey PRIMARY KEY (id);


--
-- Name: payment_callbacks payment_callbacks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payment_callbacks
    ADD CONSTRAINT payment_callbacks_pkey PRIMARY KEY (id);


--
-- Name: payment_transactions payment_transactions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payment_transactions
    ADD CONSTRAINT payment_transactions_pkey PRIMARY KEY (id);


--
-- Name: quota_usage_history quota_usage_history_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.quota_usage_history
    ADD CONSTRAINT quota_usage_history_pkey PRIMARY KEY (id);


--
-- Name: refund_records refund_records_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.refund_records
    ADD CONSTRAINT refund_records_pkey PRIMARY KEY (id);


--
-- Name: sales_contact_requests sales_contact_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sales_contact_requests
    ADD CONSTRAINT sales_contact_requests_pkey PRIMARY KEY (id);


--
-- Name: transactions transactions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.transactions
    ADD CONSTRAINT transactions_pkey PRIMARY KEY (id);


--
-- Name: billing_attempt_entries uq_billing_attempt_entry; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.billing_attempt_entries
    ADD CONSTRAINT uq_billing_attempt_entry UNIQUE (attempt_id, entry_type, ledger_type);


--
-- Name: llm_tenant_models uq_tenant_provider_model; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.llm_tenant_models
    ADD CONSTRAINT uq_tenant_provider_model UNIQUE (tenant_id, provider, model);


--
-- Name: idx_account_group_credit; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_account_group_credit ON public.group_ai_credit_accounts USING btree (account_id, group_id);


--
-- Name: idx_billing_attempt_entries_attempt; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_billing_attempt_entries_attempt ON public.billing_attempt_entries USING btree (attempt_id);


--
-- Name: idx_billing_attempt_entries_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_billing_attempt_entries_status ON public.billing_attempt_entries USING btree (status);


--
-- Name: idx_billing_attempts_org_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_billing_attempts_org_created ON public.billing_attempts USING btree (organization_id, created_at DESC);


--
-- Name: idx_billing_attempts_reconcile_queue; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_billing_attempts_reconcile_queue ON public.billing_attempts USING btree (status, lane, next_reconcile_at, updated_at);


--
-- Name: idx_billing_attempts_request_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_billing_attempts_request_id ON public.billing_attempts USING btree (request_id);


--
-- Name: idx_billing_attempts_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_billing_attempts_status ON public.billing_attempts USING btree (status);


--
-- Name: idx_btr_account; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_btr_account ON public.bank_transfer_requests USING btree (account_id);


--
-- Name: idx_btr_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_btr_created_at ON public.bank_transfer_requests USING btree (created_at);


--
-- Name: idx_btr_group; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_btr_group ON public.bank_transfer_requests USING btree (group_id);


--
-- Name: idx_btr_request_no; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_btr_request_no ON public.bank_transfer_requests USING btree (request_no);


--
-- Name: idx_btr_reviewed_by; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_btr_reviewed_by ON public.bank_transfer_requests USING btree (reviewed_by);


--
-- Name: idx_btr_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_btr_status ON public.bank_transfer_requests USING btree (status);


--
-- Name: idx_callback_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_callback_created_at ON public.payment_callbacks USING btree (created_at);


--
-- Name: idx_callback_method; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_callback_method ON public.payment_callbacks USING btree (payment_method);


--
-- Name: idx_callback_processed; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_callback_processed ON public.payment_callbacks USING btree (processed);


--
-- Name: idx_callback_transaction; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_callback_transaction ON public.payment_callbacks USING btree (transaction_id);


--
-- Name: idx_channel_wallet_transactions_attempt; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_channel_wallet_transactions_attempt ON public.channel_wallet_transactions USING btree (attempt_id, created_at DESC);


--
-- Name: idx_channel_wallet_transactions_channel; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_channel_wallet_transactions_channel ON public.channel_wallet_transactions USING btree (channel_id, created_at DESC);


--
-- Name: idx_channel_wallets_org_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_channel_wallets_org_status ON public.channel_wallets USING btree (organization_id, status);


--
-- Name: idx_credential_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_credential_active ON public.llm_credentials USING btree (is_active);


--
-- Name: idx_credential_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_credential_deleted ON public.llm_credentials USING btree (deleted_at);


--
-- Name: idx_credential_hash; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_credential_hash ON public.llm_credentials USING btree (api_key_hash);


--
-- Name: idx_credential_protocol; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_credential_protocol ON public.llm_credentials USING btree (protocol);


--
-- Name: idx_credential_provider; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_credential_provider ON public.llm_credentials USING btree (provider);


--
-- Name: idx_credential_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_credential_tenant ON public.llm_credentials USING btree (organization_id);


--
-- Name: idx_credit_account; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_credit_account ON public.group_ai_credit_accounts USING btree (account_id);


--
-- Name: idx_credit_group; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_credit_group ON public.group_ai_credit_accounts USING btree (group_id);

--
-- Name: idx_llm_credentials_org_hash; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_llm_credentials_org_hash ON public.llm_credentials USING btree (organization_id, api_key_hash) WHERE (deleted_at IS NULL);


--
-- Name: idx_llm_credentials_organization_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_llm_credentials_organization_id ON public.llm_credentials USING btree (organization_id);


--
-- Name: idx_llm_custom_models_organization_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_llm_custom_models_organization_id ON public.llm_custom_models USING btree (organization_id);


--
-- Name: idx_llm_custom_providers_organization_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_llm_custom_providers_organization_id ON public.llm_custom_providers USING btree (organization_id);


--
-- Name: idx_llm_model_configs_organization_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_llm_model_configs_organization_id ON public.llm_model_configs USING btree (organization_id);


--
-- Name: idx_llm_models_audio; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_llm_models_audio ON public.llm_models USING btree (audio) WHERE (audio = true);


--
-- Name: idx_llm_models_family; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_llm_models_family ON public.llm_models USING btree (family);


--
-- Name: idx_llm_models_finetuned; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_llm_models_finetuned ON public.llm_models USING btree (is_finetuned);


--
-- Name: idx_llm_models_function_calling; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_llm_models_function_calling ON public.llm_models USING btree (function_calling) WHERE (function_calling = true);


--
-- Name: idx_llm_models_is_configured; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_llm_models_is_configured ON public.llm_models USING btree (is_configured);


--
-- Name: idx_llm_models_is_flagship; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_llm_models_is_flagship ON public.llm_models USING btree (is_flagship);


--
-- Name: idx_llm_models_reasoning; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_llm_models_reasoning ON public.llm_models USING btree (reasoning) WHERE (reasoning = true);


--
-- Name: idx_llm_models_slug; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_llm_models_slug ON public.llm_models USING btree (slug);


--
-- Name: idx_llm_models_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_llm_models_status ON public.llm_models USING btree (status);


--
-- Name: idx_llm_models_streaming; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_llm_models_streaming ON public.llm_models USING btree (streaming) WHERE (streaming = true);


--
-- Name: idx_llm_models_system_enabled; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_llm_models_system_enabled ON public.llm_models USING btree (is_system_enabled);


--
-- Name: idx_llm_models_use_cases; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_llm_models_use_cases ON public.llm_models USING gin (use_cases);


--
-- Name: idx_llm_models_vision; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_llm_models_vision ON public.llm_models USING btree (vision) WHERE (vision = true);


--
-- Name: idx_llm_organization_api_keys_key_hash; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_llm_organization_api_keys_key_hash ON public.llm_organization_api_keys USING btree (key_hash) WHERE ((deleted_at IS NULL) AND (key_hash IS NOT NULL));


--
-- Name: idx_llm_organization_api_keys_organization_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_llm_organization_api_keys_organization_id ON public.llm_organization_api_keys USING btree (organization_id);


--
-- Name: idx_llm_providers_country_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_llm_providers_country_code ON public.llm_providers USING btree (country_code);


--
-- Name: idx_llm_providers_system_enabled; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_llm_providers_system_enabled ON public.llm_providers USING btree (is_system_enabled);


--
-- Name: idx_llm_routes_organization_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_llm_routes_organization_id ON public.llm_routes USING btree (organization_id);


--
-- Name: idx_llm_tenant_api_keys_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_llm_tenant_api_keys_deleted_at ON public.llm_organization_api_keys USING btree (deleted_at);


--
-- Name: idx_llm_tenant_api_keys_expires_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_llm_tenant_api_keys_expires_at ON public.llm_organization_api_keys USING btree (expires_at) WHERE (expires_at IS NOT NULL);


--
-- Name: idx_llm_tenant_api_keys_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_llm_tenant_api_keys_key ON public.llm_organization_api_keys USING btree (key) WHERE (deleted_at IS NULL);


--
-- Name: idx_llm_tenant_api_keys_key_hash; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_llm_tenant_api_keys_key_hash ON public.llm_organization_api_keys USING btree (key_hash);


--
-- Name: idx_llm_tenant_api_keys_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_llm_tenant_api_keys_status ON public.llm_organization_api_keys USING btree (status);


--
-- Name: idx_llm_tenant_api_keys_tenant_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_llm_tenant_api_keys_tenant_id ON public.llm_organization_api_keys USING btree (organization_id);


--
-- Name: idx_llm_tenant_custom_models_use_cases; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_llm_tenant_custom_models_use_cases ON public.llm_custom_models USING gin (use_cases);


--
-- Name: idx_llm_workspace_quotas_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_llm_workspace_quotas_org ON public.llm_workspace_quotas USING btree (organization_id);


--
-- Name: idx_model_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_model_active ON public.llm_models USING btree (is_active);


--
-- Name: idx_model_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_model_deleted_at ON public.llm_models USING btree (deleted_at);


--
-- Name: idx_model_display_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_model_display_name ON public.llm_models USING btree (display_name);


--
-- Name: idx_model_provider; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_model_provider ON public.llm_models USING btree (provider);


--
-- Name: idx_model_provider_name; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_model_provider_name ON public.llm_models USING btree (provider, name);


--
-- Name: idx_model_reasoning; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_model_reasoning ON public.llm_models USING btree (reasoning);


--
-- Name: idx_model_recommended; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_model_recommended ON public.llm_models USING btree (is_recommended) WHERE ((is_recommended = true) AND (deleted_at IS NULL));


--
-- Name: idx_model_release_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_model_release_date ON public.llm_models USING btree (release_date);


--
-- Name: idx_model_tier; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_model_tier ON public.llm_models USING btree (model_tier) WHERE ((model_tier IS NOT NULL) AND (deleted_at IS NULL));


--
-- Name: idx_model_tool_call; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_model_tool_call ON public.llm_models USING btree (function_calling);


--
-- Name: idx_models_parent_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_models_parent_id ON public.llm_models USING btree (parent_id);


--
-- Name: idx_order_account; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_order_account ON public.orders USING btree (account_id);


--
-- Name: idx_order_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_order_created_at ON public.orders USING btree (created_at);


--
-- Name: idx_order_group; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_order_group ON public.orders USING btree (group_id);


--
-- Name: idx_order_no; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_order_no ON public.orders USING btree (order_no);


--
-- Name: idx_order_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_order_status ON public.orders USING btree (status);


--
-- Name: idx_order_subscription; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_order_subscription ON public.orders USING btree (subscription_id);


--
-- Name: idx_order_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_order_type ON public.orders USING btree (order_type);


--
-- Name: idx_orders_pending_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_orders_pending_created_at ON public.orders USING btree (created_at) WHERE ((status)::text = 'pending'::text);


--
-- Name: idx_payment_transaction_account; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_transaction_account ON public.payment_transactions USING btree (account_id);


--
-- Name: idx_payment_transaction_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_transaction_created_at ON public.payment_transactions USING btree (created_at);


--
-- Name: idx_payment_transaction_method; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_transaction_method ON public.payment_transactions USING btree (payment_method);


--
-- Name: idx_payment_transaction_no; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_payment_transaction_no ON public.payment_transactions USING btree (transaction_no);


--
-- Name: idx_payment_transaction_order; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_transaction_order ON public.payment_transactions USING btree (order_id);


--
-- Name: idx_payment_transaction_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_transaction_status ON public.payment_transactions USING btree (status);


--
-- Name: idx_provider_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_provider_active ON public.llm_providers USING btree (is_active);


--
-- Name: idx_provider_currency; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_provider_currency ON public.llm_providers USING btree (currency);


--
-- Name: idx_provider_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_provider_deleted_at ON public.llm_providers USING btree (deleted_at);


--
-- Name: idx_provider_display_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_provider_display_name ON public.llm_providers USING btree (provider_name);


--
-- Name: idx_provider_name; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_provider_name ON public.llm_providers USING btree (provider);


--
-- Name: idx_quota_history_account; Type: INDEX; Schema: public; Owner: -
--
--

CREATE INDEX idx_quota_history_account ON public.quota_usage_history USING btree (account_id);


--
-- Name: idx_quota_history_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_quota_history_created_at ON public.quota_usage_history USING btree (created_at);


--
-- Name: idx_quota_history_group; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_quota_history_group ON public.quota_usage_history USING btree (group_id);


--
-- Name: idx_quota_history_group_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_quota_history_group_created ON public.quota_usage_history USING btree (group_id, created_at);


--
-- Name: idx_quota_history_group_resource; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_quota_history_group_resource ON public.quota_usage_history USING btree (group_id, resource_type);


--
-- Name: idx_quota_history_resource_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_quota_history_resource_type ON public.quota_usage_history USING btree (resource_type);


--
-- Name: idx_quota_history_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_quota_history_tenant ON public.quota_usage_history USING btree (tenant_id);


--
-- Name: idx_refund_account; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_refund_account ON public.refund_records USING btree (account_id);


--
-- Name: idx_refund_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_refund_created_at ON public.refund_records USING btree (created_at);


--
-- Name: idx_refund_no; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_refund_no ON public.refund_records USING btree (refund_no);


--
-- Name: idx_refund_order; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_refund_order ON public.refund_records USING btree (order_id);


--
-- Name: idx_refund_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_refund_status ON public.refund_records USING btree (status);


--
-- Name: idx_refund_transaction; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_refund_transaction ON public.refund_records USING btree (transaction_id);


--
-- Name: idx_route_credential; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_route_credential ON public.llm_routes USING btree (credential_id);


--
-- Name: idx_route_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_route_deleted ON public.llm_routes USING btree (deleted_at);


--
-- Name: idx_route_enabled; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_route_enabled ON public.llm_routes USING btree (is_enabled);

--
-- Name: idx_route_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_route_tenant ON public.llm_routes USING btree (organization_id);


--
-- Name: idx_route_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_route_type ON public.llm_routes USING btree (type);


--
-- Name: idx_route_usr_cred; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_route_usr_cred ON public.llm_routes USING btree (user_credential_id);


--
-- Name: idx_routes_models; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_routes_models ON public.llm_routes USING gin (match_models) WHERE (match_models IS NOT NULL);


--
-- Name: idx_routes_query; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_routes_query ON public.llm_routes USING btree (organization_id, is_enabled, priority DESC, weight DESC);


--
-- Name: idx_routes_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_routes_status ON public.llm_routes USING btree (status) WHERE (deleted_at IS NULL);


--
-- Name: idx_routes_tenant_new; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_routes_tenant_new ON public.llm_routes USING btree (organization_id) WHERE ((deleted_at IS NULL) AND (is_enabled = true));


--
-- Name: idx_routes_tenant_priority; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_routes_tenant_priority ON public.llm_routes USING btree (organization_id, priority DESC) WHERE ((deleted_at IS NULL) AND (is_enabled = true));


--
-- Name: idx_sales_contact_account; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sales_contact_account ON public.sales_contact_requests USING btree (account_id);


--
-- Name: idx_sales_contact_account_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_sales_contact_account_unique ON public.sales_contact_requests USING btree (account_id) WHERE (account_id IS NOT NULL);


--
-- Name: idx_sales_contact_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sales_contact_created_at ON public.sales_contact_requests USING btree (created_at);

--
-- Name: idx_tenant_custom_model_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tenant_custom_model_active ON public.llm_custom_models USING btree (is_active);


--
-- Name: idx_tenant_custom_model_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tenant_custom_model_deleted_at ON public.llm_custom_models USING btree (deleted_at);


--
-- Name: idx_tenant_custom_model_provider; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tenant_custom_model_provider ON public.llm_custom_models USING btree (provider_id);


--
-- Name: idx_tenant_custom_model_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tenant_custom_model_tenant ON public.llm_custom_models USING btree (organization_id);


--
-- Name: idx_tenant_custom_provider_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tenant_custom_provider_active ON public.llm_custom_providers USING btree (is_active);


--
-- Name: idx_tenant_custom_provider_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tenant_custom_provider_deleted_at ON public.llm_custom_providers USING btree (deleted_at);


--
-- Name: idx_tenant_custom_provider_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tenant_custom_provider_tenant ON public.llm_custom_providers USING btree (organization_id);


--
-- Name: idx_tenant_model_config_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tenant_model_config_deleted_at ON public.llm_model_configs USING btree (deleted_at);


--
-- Name: idx_tenant_model_config_enabled; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tenant_model_config_enabled ON public.llm_model_configs USING btree (is_enabled);


--
-- Name: idx_tenant_model_config_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tenant_model_config_tenant ON public.llm_model_configs USING btree (organization_id);


--
-- Name: idx_tenant_model_config_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_tenant_model_config_unique ON public.llm_model_configs USING btree (organization_id, model_id) WHERE (deleted_at IS NULL);


--
-- Name: idx_tenant_model_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tenant_model_deleted_at ON public.llm_tenant_models USING btree (deleted_at);


--
-- Name: idx_tenant_model_enabled; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tenant_model_enabled ON public.llm_tenant_models USING btree (is_enabled);


--
-- Name: idx_tenant_model_model; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tenant_model_model ON public.llm_tenant_models USING btree (model);


--
-- Name: idx_tenant_model_provider; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tenant_model_provider ON public.llm_tenant_models USING btree (provider);


--
-- Name: idx_tenant_model_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tenant_model_tenant ON public.llm_tenant_models USING btree (tenant_id);


--
-- Name: idx_tenant_model_tenant_provider; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tenant_model_tenant_provider ON public.llm_tenant_models USING btree (tenant_id, provider);

--
-- Name: idx_tenant_routes_health; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tenant_routes_health ON public.llm_routes USING btree (success_count, failure_count) WHERE ((success_count > 0) OR (failure_count > 0));


--
-- Name: idx_tenant_routes_protocol; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tenant_routes_protocol ON public.llm_routes USING btree (protocol) WHERE (protocol IS NOT NULL);


--
-- Name: idx_tx_batch; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tx_batch ON public.transactions USING btree (batch_id);


--
-- Name: idx_tx_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tx_created_at ON public.transactions USING btree (created_at);


--
-- Name: idx_tx_group; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tx_group ON public.transactions USING btree (group_id);


--
-- Name: idx_tx_group_currency; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tx_group_currency ON public.transactions USING btree (group_id, currency_type);


--
-- Name: idx_tx_reference; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tx_reference ON public.transactions USING btree (reference_type, reference_id);


--
-- Name: idx_tx_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tx_tenant ON public.transactions USING btree (tenant_id);


--
-- Name: idx_tx_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tx_type ON public.transactions USING btree (transaction_type);


--
-- Name: idx_wallet_account; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_wallet_account ON public.group_wallets USING btree (account_id);


--
-- Name: idx_wallet_account_group_currency; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_wallet_account_group_currency ON public.group_wallets USING btree (account_id, group_id, currency);


--
-- Name: idx_wallet_group; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_wallet_group ON public.group_wallets USING btree (group_id);


--
-- Name: billing_attempt_entries billing_attempt_entries_attempt_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.billing_attempt_entries
    ADD CONSTRAINT billing_attempt_entries_attempt_id_fkey FOREIGN KEY (attempt_id) REFERENCES public.billing_attempts(attempt_id) ON DELETE CASCADE;


--
-- Name: channel_wallet_transactions channel_wallet_transactions_channel_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.channel_wallet_transactions
    ADD CONSTRAINT channel_wallet_transactions_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES public.channel_wallets(channel_id) ON DELETE CASCADE;


--
-- Name: channel_wallets channel_wallets_channel_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.channel_wallets
    ADD CONSTRAINT channel_wallets_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES public.llm_routes(id) ON DELETE CASCADE;


--
-- Name: llm_custom_models fk_llm_custom_models_organization; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.llm_custom_models
    ADD CONSTRAINT fk_llm_custom_models_organization FOREIGN KEY (organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;


--
-- Name: llm_custom_providers fk_llm_custom_providers_organization; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.llm_custom_providers
    ADD CONSTRAINT fk_llm_custom_providers_organization FOREIGN KEY (organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;


--
-- Name: llm_routes fk_llm_routes_credential; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.llm_routes
    ADD CONSTRAINT fk_llm_routes_credential FOREIGN KEY (user_credential_id) REFERENCES public.llm_credentials(id) ON DELETE SET NULL;


-- Name: llm_model_configs fk_tenant_model_config_model; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.llm_model_configs
    ADD CONSTRAINT fk_tenant_model_config_model FOREIGN KEY (model_id) REFERENCES public.llm_models(id) ON DELETE CASCADE;


--
-- Name: llm_tenant_models fk_tenant_model_provider; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.llm_tenant_models
    ADD CONSTRAINT fk_tenant_model_provider FOREIGN KEY (provider) REFERENCES public.llm_providers(provider) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: llm_tenant_models fk_tenant_model_provider_model; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.llm_tenant_models
    ADD CONSTRAINT fk_tenant_model_provider_model FOREIGN KEY (provider, model) REFERENCES public.llm_models(provider, name) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: llm_tenant_models fk_tenant_model_tenant; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.llm_tenant_models
    ADD CONSTRAINT fk_tenant_model_tenant FOREIGN KEY (tenant_id) REFERENCES public.workspaces(id) ON UPDATE CASCADE ON DELETE CASCADE;

--
-- PostgreSQL database dump complete
--

\unrestrict OammFOhMfJ77tJrtSGBSTnMOZKJdviy5nbhfAVzDHdDQpXfVWoFzbdduU9e7M74

