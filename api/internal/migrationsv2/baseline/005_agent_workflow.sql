--
-- PostgreSQL database dump
--

\restrict Tq9EtGPOa5CuQYbXIQbHOzNhijdHTQUItnQew4vnVPtUDFTA1i8M0BnZUKTHEhj

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
-- Name: agent_api_key_usage_logs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.agent_api_key_usage_logs (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    api_key_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    operation_log_id uuid,
    request_path character varying(500) NOT NULL,
    request_ip character varying(45) NOT NULL,
    user_agent text,
    request_headers json,
    request_body_size bigint DEFAULT 0,
    response_status_code integer NOT NULL,
    response_body_size bigint DEFAULT 0,
    response_time_ms integer NOT NULL,
    tokens_used integer DEFAULT 0,
    cost_amount numeric(10,6) DEFAULT 0,
    currency character varying(3) DEFAULT 'USD'::character varying,
    error_message text,
    metadata json,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    tenant_id uuid
);


--
-- Name: agent_api_keys; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.agent_api_keys (
    id character varying(36) NOT NULL,
    agent_id character varying(36) NOT NULL,
    tenant_id character varying(36) NOT NULL,
    key_hash character varying(64) NOT NULL,
    key_prefix character varying(12) NOT NULL,
    name character varying(255) NOT NULL,
    status character varying(20) DEFAULT 'active'::character varying,
    expires_at timestamp with time zone,
    usage_count bigint DEFAULT 0,
    last_used_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    CONSTRAINT agent_api_keys_status_check CHECK (((status)::text = ANY (ARRAY[('active'::character varying)::text, ('inactive'::character varying)::text, ('revoked'::character varying)::text])))
);


--
-- Name: agent_extensions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.agent_extensions (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    agent_id uuid NOT NULL,
    permission character varying(32),
    extended_properties jsonb,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: agents; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.agents (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    tenant_id uuid NOT NULL,
    name character varying(255) NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    agent_type character varying(255) NOT NULL,
    icon_type character varying(255),
    icon text,
    agents_model_config_id uuid,
    workflow_id uuid,
    enable_api boolean NOT NULL,
    is_public boolean DEFAULT false NOT NULL,
    is_universal boolean DEFAULT false NOT NULL,
    created_by uuid,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_by uuid,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    deleted_by uuid,
    deleted_at timestamp with time zone,
    workflow_config character varying,
    internal boolean DEFAULT false,
    web_app_id uuid DEFAULT public.uuid_generate_v4() NOT NULL
);


--
-- Name: agents_configs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.agents_configs (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    agents_id uuid NOT NULL,
    model_provider character varying(255),
    model_version_id character varying(255),
    configs json,
    created_by uuid,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_by uuid,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    deleted_by uuid,
    deleted_at timestamp with time zone,
    greeting_message text,
    user_input_form text,
    dataset_query_variable character varying(255),
    pre_prompt text,
    agent_mode text,
    sensitive_word_avoidance text,
    retriever_resource text,
    prompt_type character varying(255) DEFAULT 'simple'::character varying NOT NULL,
    chat_prompt_config text,
    completion_prompt_config text,
    dataset_configs text,
    external_data_tools text,
    file_upload text
);


--
-- Name: agents_conversations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.agents_conversations (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    agent_id uuid NOT NULL,
    agent_config_id uuid,
    model_provider character varying(255),
    override_model_configs text,
    model_version_id character varying(255),
    mode character varying(255) NOT NULL,
    name character varying(255) NOT NULL,
    summary text,
    inputs json NOT NULL,
    introduction text,
    system_instruction text,
    system_instruction_tokens integer DEFAULT 0 NOT NULL,
    status character varying(255) NOT NULL,
    invoke_from character varying(255),
    from_source character varying(255) NOT NULL,
    from_end_user_id uuid,
    from_account_id uuid,
    read_at timestamp with time zone,
    read_account_id uuid,
    dialogue_count integer DEFAULT 0 NOT NULL,
    created_by uuid,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_by uuid,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    deleted_by uuid,
    deleted_at timestamp with time zone,
    workflow_version_uuid uuid,
    web_app_id uuid
);


--
-- Name: agents_messages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.agents_messages (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    agent_id uuid NOT NULL,
    model_provider character varying(255),
    model_version_id character varying(255),
    override_model_configs text,
    conversation_id uuid NOT NULL,
    inputs json NOT NULL,
    query text NOT NULL,
    message json NOT NULL,
    message_tokens integer DEFAULT 0 NOT NULL,
    message_unit_price numeric(10,4) NOT NULL,
    message_price_unit numeric(10,7) DEFAULT 0.001 NOT NULL,
    answer text NOT NULL,
    answer_tokens integer DEFAULT 0 NOT NULL,
    answer_unit_price numeric(10,4) NOT NULL,
    answer_price_unit numeric(10,7) DEFAULT 0.001 NOT NULL,
    parent_message_id uuid,
    provider_response_latency double precision DEFAULT 0 NOT NULL,
    total_price numeric(10,7),
    currency character varying(255) NOT NULL,
    status character varying(255) DEFAULT 'normal'::character varying NOT NULL,
    error text,
    message_metadata text,
    invoke_from character varying(255),
    from_source character varying(255) NOT NULL,
    from_end_user_id uuid,
    from_account_id uuid,
    created_by uuid,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_by uuid,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    deleted_by uuid,
    deleted_at timestamp with time zone,
    agent_based boolean DEFAULT false NOT NULL,
    workflow_run_id uuid,
    workflow_version_uuid uuid,
    web_app_id uuid
);

--
-- Name: conversation_group; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.conversation_group (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    app_id character varying(36) NOT NULL,
    group_id character varying(36) NOT NULL,
    conversation_id character varying(36),
    from_account_id character varying(36) NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    name character varying(255) DEFAULT ''::character varying NOT NULL
);


--
-- Name: conversations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.conversations (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    app_id uuid NOT NULL,
    model_provider character varying(255),
    override_model_configs text,
    model_id character varying(255),
    mode character varying(255) NOT NULL,
    name character varying(255) NOT NULL,
    summary text,
    inputs json NOT NULL,
    introduction text,
    system_instruction text,
    system_instruction_tokens integer DEFAULT 0 NOT NULL,
    status character varying(255) NOT NULL,
    from_source character varying(255) NOT NULL,
    from_end_user_id uuid,
    from_account_id uuid,
    read_at timestamp with time zone,
    read_account_id uuid,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    is_deleted boolean DEFAULT false NOT NULL,
    invoke_from character varying(255),
    dialogue_count integer DEFAULT 0 NOT NULL
);


--
-- Name: installed_agents; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.installed_agents (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    tenant_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    agent_owner_tenant_id uuid NOT NULL,
    "position" integer DEFAULT 0 NOT NULL,
    is_pinned boolean DEFAULT false NOT NULL,
    last_used_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);

--
-- Name: message_annotations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.message_annotations (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    app_id uuid NOT NULL,
    conversation_id uuid,
    message_id uuid,
    content text NOT NULL,
    account_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    question text,
    hit_count integer DEFAULT 0 NOT NULL
);


--
-- Name: message_feedbacks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.message_feedbacks (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    app_id uuid NOT NULL,
    conversation_id uuid NOT NULL,
    message_id uuid NOT NULL,
    rating character varying(255) NOT NULL,
    content text,
    from_source character varying(255) NOT NULL,
    from_end_user_id uuid,
    from_account_id uuid,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: message_files; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.message_files (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    message_id uuid NOT NULL,
    type character varying(255) NOT NULL,
    transfer_method character varying(255) NOT NULL,
    url text,
    upload_file_id uuid,
    created_by_role character varying(255) NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    belongs_to character varying(255)
);


--
-- Name: messages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.messages (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    app_id uuid NOT NULL,
    model_provider character varying(255),
    model_id character varying(255),
    override_model_configs text,
    conversation_id uuid NOT NULL,
    inputs json NOT NULL,
    query text NOT NULL,
    message json NOT NULL,
    message_tokens integer DEFAULT 0 NOT NULL,
    message_unit_price numeric(10,4) NOT NULL,
    answer text NOT NULL,
    answer_tokens integer DEFAULT 0 NOT NULL,
    answer_unit_price numeric(10,4) NOT NULL,
    provider_response_latency double precision DEFAULT 0 NOT NULL,
    total_price numeric(10,7),
    currency character varying(255) NOT NULL,
    from_source character varying(255) NOT NULL,
    from_end_user_id uuid,
    from_account_id uuid,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    agent_based boolean DEFAULT false NOT NULL,
    message_price_unit numeric(10,7) DEFAULT 0.001 NOT NULL,
    answer_price_unit numeric(10,7) DEFAULT 0.001 NOT NULL,
    workflow_run_id uuid,
    status character varying(255) DEFAULT 'normal'::character varying NOT NULL,
    error text,
    message_metadata text,
    invoke_from character varying(255),
    parent_message_id uuid
);


--
-- Name: tool_files; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tool_files (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    user_id uuid NOT NULL,
    tenant_id uuid NOT NULL,
    conversation_id uuid,
    file_key character varying(255) NOT NULL,
    mimetype character varying(255) NOT NULL,
    original_url character varying(2048),
    name character varying NOT NULL,
    size integer NOT NULL,
    lifecycle character varying(32) DEFAULT 'persistent'::character varying NOT NULL,
    expires_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    deleted_at timestamp with time zone
);


--
-- Name: workflow_app_logs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.workflow_app_logs (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    tenant_id uuid NOT NULL,
    app_id uuid NOT NULL,
    workflow_id uuid NOT NULL,
    workflow_run_id uuid NOT NULL,
    created_from character varying(255) NOT NULL,
    created_by_role character varying(255) NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    agent_id uuid
);


--
-- Name: workflow_conversation_variables; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.workflow_conversation_variables (
    id uuid NOT NULL,
    conversation_id uuid NOT NULL,
    app_id uuid NOT NULL,
    data text NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: workflow_node_executions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.workflow_node_executions (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    tenant_id uuid NOT NULL,
    app_id uuid NOT NULL,
    workflow_id uuid NOT NULL,
    triggered_from character varying(255) NOT NULL,
    workflow_run_id uuid,
    index integer NOT NULL,
    predecessor_node_id character varying(255),
    node_id character varying(255) NOT NULL,
    node_type character varying(255) NOT NULL,
    title character varying(255) NOT NULL,
    inputs text,
    process_data text,
    outputs text,
    status character varying(255) NOT NULL,
    error text,
    elapsed_time double precision DEFAULT 0 NOT NULL,
    execution_metadata text,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    created_by_role character varying(255) NOT NULL,
    created_by uuid NOT NULL,
    finished_at timestamp with time zone,
    node_execution_id character varying(255),
    agent_id uuid
);


--
-- Name: workflow_node_runtime_logs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.workflow_node_runtime_logs (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    tenant_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    workflow_id uuid NOT NULL,
    triggered_from character varying(255) NOT NULL,
    workflow_run_id uuid,
    index integer NOT NULL,
    predecessor_node_id character varying(255),
    node_execution_id character varying(255),
    node_id character varying(255) NOT NULL,
    node_type character varying(255) NOT NULL,
    title character varying(255) NOT NULL,
    inputs text,
    process_data text,
    outputs text,
    status character varying(255) NOT NULL,
    error text,
    elapsed_time double precision DEFAULT 0 NOT NULL,
    execution_metadata text,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    created_by_role character varying(255) NOT NULL,
    created_by uuid NOT NULL,
    deleted_at timestamp with time zone,
    deleted_by uuid,
    finished_at timestamp with time zone,
    graph jsonb,
    features jsonb,
    workflow_version_uuid uuid,
    web_app_id uuid
);


--
-- Name: workflow_run_logs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.workflow_run_logs (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    tenant_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    sequence_number integer NOT NULL,
    workflow_id uuid NOT NULL,
    type character varying(255) NOT NULL,
    triggered_from character varying(255) NOT NULL,
    version character varying(255) NOT NULL,
    graph text,
    inputs text,
    status character varying(255) NOT NULL,
    outputs text DEFAULT '{}'::text,
    error text,
    elapsed_time double precision DEFAULT 0 NOT NULL,
    total_tokens bigint DEFAULT 0 NOT NULL,
    total_steps integer DEFAULT 0,
    created_by_role character varying(255) NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    finished_at timestamp with time zone,
    deleted_at timestamp with time zone,
    deleted_by uuid,
    exceptions_count integer DEFAULT 0,
    features jsonb,
    workflow_version_uuid uuid,
    web_app_id uuid
);


--
-- Name: workflow_runs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.workflow_runs (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    tenant_id uuid NOT NULL,
    app_id uuid NOT NULL,
    sequence_number integer NOT NULL,
    workflow_id uuid NOT NULL,
    type character varying(255) NOT NULL,
    triggered_from character varying(255) NOT NULL,
    version character varying(255) NOT NULL,
    graph text,
    inputs text,
    status character varying(255) NOT NULL,
    outputs text,
    error text,
    elapsed_time double precision DEFAULT 0 NOT NULL,
    total_tokens bigint DEFAULT 0 NOT NULL,
    total_steps integer DEFAULT 0,
    created_by_role character varying(255) NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    finished_at timestamp with time zone,
    exceptions_count integer DEFAULT 0
);


--
-- Name: workflows; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.workflows (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    tenant_id uuid NOT NULL,
    app_id uuid NOT NULL,
    type character varying(255) NOT NULL,
    version character varying(255) NOT NULL,
    graph text NOT NULL,
    features text NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_by uuid,
    updated_at timestamp with time zone NOT NULL,
    environment_variables text DEFAULT '{}'::text NOT NULL,
    conversation_variables text DEFAULT '{}'::text NOT NULL,
    marked_name character varying DEFAULT ''::character varying NOT NULL,
    marked_comment character varying DEFAULT ''::character varying NOT NULL,
    deleted_at timestamp with time zone,
    deleted_by uuid,
    agent_id uuid DEFAULT '00000000-0000-0000-0000-000000000000'::uuid NOT NULL,
    version_uuid uuid,
    internal boolean DEFAULT false
);


--
-- Name: agent_api_key_usage_logs agent_api_key_usage_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_api_key_usage_logs
    ADD CONSTRAINT agent_api_key_usage_logs_pkey PRIMARY KEY (id);


--
-- Name: agent_api_keys agent_api_keys_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_api_keys
    ADD CONSTRAINT agent_api_keys_pkey PRIMARY KEY (id);


--
-- Name: agent_extensions agent_extensions_agent_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_extensions
    ADD CONSTRAINT agent_extensions_agent_id_key UNIQUE (agent_id);


--
-- Name: agent_extensions agent_extensions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_extensions
    ADD CONSTRAINT agent_extensions_pkey PRIMARY KEY (id);


--
-- Name: agents_configs agents_configs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agents_configs
    ADD CONSTRAINT agents_configs_pkey PRIMARY KEY (id);


--
-- Name: agents_conversations agents_conversations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agents_conversations
    ADD CONSTRAINT agents_conversations_pkey PRIMARY KEY (id);


--
-- Name: agents_messages agents_messages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agents_messages
    ADD CONSTRAINT agents_messages_pkey PRIMARY KEY (id);


--
-- Name: agents agents_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT agents_pkey PRIMARY KEY (id);

--
-- Name: conversation_group conversation_group_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversation_group
    ADD CONSTRAINT conversation_group_pkey PRIMARY KEY (id);


--
-- Name: conversations conversations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversations
    ADD CONSTRAINT conversations_pkey PRIMARY KEY (id);


--
-- Name: installed_agents installed_agents_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.installed_agents
    ADD CONSTRAINT installed_agents_pkey PRIMARY KEY (id);

--
-- Name: message_annotations message_annotations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.message_annotations
    ADD CONSTRAINT message_annotations_pkey PRIMARY KEY (id);


--
-- Name: message_feedbacks message_feedbacks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.message_feedbacks
    ADD CONSTRAINT message_feedbacks_pkey PRIMARY KEY (id);


--
-- Name: message_files message_files_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.message_files
    ADD CONSTRAINT message_files_pkey PRIMARY KEY (id);


--
-- Name: messages messages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.messages
    ADD CONSTRAINT messages_pkey PRIMARY KEY (id);


--
-- Name: tool_files tool_files_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tool_files
    ADD CONSTRAINT tool_files_pkey PRIMARY KEY (id);


--
-- Name: workflow_app_logs workflow_app_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.workflow_app_logs
    ADD CONSTRAINT workflow_app_logs_pkey PRIMARY KEY (id);


--
-- Name: workflow_conversation_variables workflow_conversation_variables_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.workflow_conversation_variables
    ADD CONSTRAINT workflow_conversation_variables_pkey PRIMARY KEY (id);


--
-- Name: workflow_node_executions workflow_node_executions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.workflow_node_executions
    ADD CONSTRAINT workflow_node_executions_pkey PRIMARY KEY (id);


--
-- Name: workflow_node_runtime_logs workflow_node_runtime_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.workflow_node_runtime_logs
    ADD CONSTRAINT workflow_node_runtime_logs_pkey PRIMARY KEY (id);


--
-- Name: workflow_run_logs workflow_run_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.workflow_run_logs
    ADD CONSTRAINT workflow_run_logs_pkey PRIMARY KEY (id);


--
-- Name: workflow_runs workflow_runs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.workflow_runs
    ADD CONSTRAINT workflow_runs_pkey PRIMARY KEY (id);


--
-- Name: workflows workflows_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.workflows
    ADD CONSTRAINT workflows_pkey PRIMARY KEY (id);


--
-- Name: agents_agents_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX agents_agents_id_idx ON public.agents_configs USING btree (agents_id);


--
-- Name: agents_conversations_agent_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX agents_conversations_agent_id_idx ON public.agents_conversations USING btree (agent_id);


--
-- Name: agents_message_agents_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX agents_message_agents_id_idx ON public.agents_messages USING btree (agent_id, created_at);


--
-- Name: agents_message_conversation_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX agents_message_conversation_id_idx ON public.agents_messages USING btree (conversation_id, workflow_run_id);


--
-- Name: agents_message_created_at_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX agents_message_created_at_idx ON public.agents_messages USING btree (created_at);


--
-- Name: agents_message_end_user_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX agents_message_end_user_idx ON public.agents_messages USING btree (agent_id, from_source, from_end_user_id);


--
-- Name: agents_message_workflow_run_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX agents_message_workflow_run_id_idx ON public.agents_messages USING btree (conversation_id, workflow_run_id);


--
-- Name: agents_messages_conversation_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX agents_messages_conversation_id_idx ON public.agents_messages USING btree (conversation_id);


--
-- Name: agents_tenant_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX agents_tenant_id_idx ON public.agents USING btree (tenant_id);

--
-- Name: conversation_app_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX conversation_app_id_idx ON public.conversations USING btree (app_id);


--
-- Name: conversation_from_user_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX conversation_from_user_idx ON public.agents_conversations USING btree (agent_id, from_source, from_end_user_id);


--
-- Name: conversation_group_app_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX conversation_group_app_id_idx ON public.conversation_group USING btree (app_id);


--
-- Name: idx_ae_agent_permission; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ae_agent_permission ON public.agent_extensions USING btree (agent_id, permission);


--
-- Name: idx_agent_api_key_usage_logs_agent_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agent_api_key_usage_logs_agent_id ON public.agent_api_key_usage_logs USING btree (agent_id);


--
-- Name: idx_agent_api_key_usage_logs_api_key_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agent_api_key_usage_logs_api_key_created ON public.agent_api_key_usage_logs USING btree (api_key_id, created_at);


--
-- Name: idx_agent_api_key_usage_logs_api_key_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agent_api_key_usage_logs_api_key_id ON public.agent_api_key_usage_logs USING btree (api_key_id);


--
-- Name: idx_agent_api_key_usage_logs_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agent_api_key_usage_logs_created_at ON public.agent_api_key_usage_logs USING btree (created_at);


--
-- Name: idx_agent_api_keys_agent_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agent_api_keys_agent_id ON public.agent_api_keys USING btree (agent_id);


--
-- Name: idx_agent_api_keys_agent_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agent_api_keys_agent_tenant ON public.agent_api_keys USING btree (agent_id, tenant_id);


--
-- Name: idx_agent_api_keys_expires_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agent_api_keys_expires_at ON public.agent_api_keys USING btree (expires_at);


--
-- Name: idx_agent_api_keys_key_hash; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agent_api_keys_key_hash ON public.agent_api_keys USING btree (key_hash);


--
-- Name: idx_agent_api_keys_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agent_api_keys_status ON public.agent_api_keys USING btree (status);


--
-- Name: idx_agent_api_keys_tenant_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agent_api_keys_tenant_id ON public.agent_api_keys USING btree (tenant_id);


--
-- Name: idx_agent_extensions_permission; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agent_extensions_permission ON public.agent_extensions USING btree (permission);


--
-- Name: idx_agents_conversations_web_app_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agents_conversations_web_app_id ON public.agents_conversations USING btree (web_app_id);


--
-- Name: idx_agents_created_at_desc; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agents_created_at_desc ON public.agents USING btree (created_at DESC) WHERE (deleted_at IS NULL);


--
-- Name: idx_agents_created_by; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agents_created_by ON public.agents USING btree (created_by) WHERE (deleted_at IS NULL);


--
-- Name: idx_agents_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agents_deleted_at ON public.agents USING btree (deleted_at);


--
-- Name: idx_agents_internal; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agents_internal ON public.agents USING btree (internal) WHERE (internal = true);


--
-- Name: idx_agents_internal_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agents_internal_tenant ON public.agents USING btree (internal, tenant_id) WHERE (internal = true);


--
-- Name: idx_agents_messages_web_app_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agents_messages_web_app_id ON public.agents_messages USING btree (web_app_id);


--
-- Name: idx_agents_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agents_tenant ON public.agents USING btree (tenant_id) WHERE (deleted_at IS NULL);


--
-- Name: idx_agents_tenant_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agents_tenant_created_at ON public.agents USING btree (tenant_id, created_at DESC) WHERE (deleted_at IS NULL);


--
-- Name: idx_agents_web_app_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_agents_web_app_id ON public.agents USING btree (web_app_id);


--
-- Name: idx_tool_files_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tool_files_deleted_at ON public.tool_files USING btree (deleted_at);


--
-- Name: idx_tool_files_lifecycle_expires_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tool_files_lifecycle_expires_at ON public.tool_files USING btree (lifecycle, expires_at);


--
-- Name: idx_workflows_internal; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_workflows_internal ON public.workflows USING btree (internal) WHERE (internal = true);


--
-- Name: idx_workflows_internal_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_workflows_internal_tenant ON public.workflows USING btree (internal, tenant_id) WHERE (internal = true);


--
-- Name: installed_agents_tenant_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX installed_agents_tenant_id_idx ON public.installed_agents USING btree (tenant_id);

--
-- Name: message_annotations_app_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX message_annotations_app_id_idx ON public.message_annotations USING btree (app_id);


--
-- Name: message_annotations_conversation_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX message_annotations_conversation_id_idx ON public.message_annotations USING btree (conversation_id);


--
-- Name: message_annotations_message_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX message_annotations_message_id_idx ON public.message_annotations USING btree (message_id);


--
-- Name: message_feedbacks_app_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX message_feedbacks_app_id_idx ON public.message_feedbacks USING btree (app_id);


--
-- Name: message_feedbacks_conversation_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX message_feedbacks_conversation_id_idx ON public.message_feedbacks USING btree (conversation_id);


--
-- Name: message_feedbacks_message_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX message_feedbacks_message_id_idx ON public.message_feedbacks USING btree (message_id);


--
-- Name: message_files_created_by_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX message_files_created_by_idx ON public.message_files USING btree (created_by);


--
-- Name: message_files_message_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX message_files_message_id_idx ON public.message_files USING btree (message_id);


--
-- Name: messages_app_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX messages_app_id_idx ON public.messages USING btree (app_id);


--
-- Name: messages_conversation_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX messages_conversation_id_idx ON public.messages USING btree (conversation_id);


--
-- Name: messages_created_at_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX messages_created_at_idx ON public.messages USING btree (created_at);


--
-- Name: messages_end_user_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX messages_end_user_idx ON public.messages USING btree (app_id, from_source, from_end_user_id);


--
-- Name: messages_workflow_run_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX messages_workflow_run_id_idx ON public.messages USING btree (conversation_id, workflow_run_id);


--
-- Name: tool_file_conversation_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX tool_file_conversation_id_idx ON public.tool_files USING btree (conversation_id);


--
-- Name: workflow_node_executions_workflow_run_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX workflow_node_executions_workflow_run_id_idx ON public.workflow_node_executions USING btree (workflow_run_id);


--
-- Name: workflow_node_runtime_logs_workflow_run_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX workflow_node_runtime_logs_workflow_run_id_idx ON public.workflow_node_runtime_logs USING btree (workflow_run_id);


--
-- Name: workflow_run_logs_agent_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX workflow_run_logs_agent_id_idx ON public.workflow_run_logs USING btree (agent_id);


--
-- Name: workflow_runs_app_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX workflow_runs_app_id_idx ON public.workflow_runs USING btree (app_id);


--
-- Name: workflows_app_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX workflows_app_id_idx ON public.workflows USING btree (app_id);


--
-- Name: workflows_tenant_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX workflows_tenant_id_idx ON public.workflows USING btree (tenant_id);


--
-- PostgreSQL database dump complete
--

\unrestrict Tq9EtGPOa5CuQYbXIQbHOzNhijdHTQUItnQew4vnVPtUDFTA1i8M0BnZUKTHEhj

