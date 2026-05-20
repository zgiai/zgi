--
-- PostgreSQL database dump
--

\restrict HfCAsdBKLrQAIpPKsvNWGpJyu9Hd66KWNyPA9e0AP6Uiwz8QM9PUewIk7WkQwWT

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
-- Name: batch_hit_testing_tasks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.batch_hit_testing_tasks (
    task_id character varying(36) NOT NULL,
    dataset_id uuid NOT NULL,
    account_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    status character varying(20) DEFAULT 'pending'::character varying NOT NULL,
    progress integer DEFAULT 0 NOT NULL,
    total integer NOT NULL,
    completed integer DEFAULT 0 NOT NULL,
    failed integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    queries jsonb
);


--
-- Name: child_chunks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.child_chunks (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    organization_id uuid NOT NULL,
    dataset_id uuid NOT NULL,
    document_id uuid NOT NULL,
    segment_id uuid NOT NULL,
    "position" integer NOT NULL,
    content text NOT NULL,
    word_count integer NOT NULL,
    index_node_id character varying(255),
    index_node_hash character varying(255),
    type character varying(255) DEFAULT 'automatic'::character varying NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_by uuid,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    indexing_at timestamp with time zone,
    completed_at timestamp with time zone,
    error text
);


--
-- Name: dataset_collection_bindings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.dataset_collection_bindings (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    provider_name character varying(255) NOT NULL,
    model_name character varying(255) NOT NULL,
    collection_name character varying(64) NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    type character varying(40) DEFAULT 'dataset'::character varying NOT NULL
);


--
-- Name: dataset_folder_joins; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.dataset_folder_joins (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    dataset_id uuid NOT NULL,
    folder_id uuid NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: dataset_folders; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.dataset_folders (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    workspace_id uuid,
    name character varying(255) NOT NULL,
    description text,
    parent_id uuid,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_by uuid,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    icon_type character varying(255),
    icon character varying(255),
    "position" integer DEFAULT 0 NOT NULL,
    permission character varying(255) DEFAULT 'only_me'::character varying NOT NULL,
    icon_background character varying(255),
    organization_id uuid
);


--
-- Name: dataset_metadata_bindings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.dataset_metadata_bindings (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    tenant_id uuid NOT NULL,
    dataset_id uuid NOT NULL,
    metadata_id uuid NOT NULL,
    document_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    created_by uuid NOT NULL
);


--
-- Name: dataset_metadatas; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.dataset_metadatas (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    tenant_id uuid NOT NULL,
    dataset_id uuid NOT NULL,
    type character varying(255) NOT NULL,
    name character varying(255) NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    created_by uuid NOT NULL,
    updated_by uuid
);


--
-- Name: dataset_permissions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.dataset_permissions (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    dataset_id uuid NOT NULL,
    account_id uuid NOT NULL,
    has_permission boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    tenant_id uuid NOT NULL
);


--
-- Name: dataset_process_rules; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.dataset_process_rules (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    dataset_id uuid NOT NULL,
    mode character varying(255) DEFAULT 'automatic'::character varying NOT NULL,
    rules text,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: dataset_queries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.dataset_queries (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    dataset_id uuid NOT NULL,
    content text NOT NULL,
    source character varying(255) NOT NULL,
    source_app_id uuid,
    created_by_role character varying NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    results jsonb,
    elapsed_time numeric,
    hit_count integer,
    query_type character varying(50) DEFAULT 'single'::character varying NOT NULL,
    batch_task_id uuid,
    batch_name character varying(255)
);


--
-- Name: datasets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.datasets (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    workspace_id uuid,
    name character varying(255) NOT NULL,
    description text,
    provider character varying(255) DEFAULT 'vendor'::character varying NOT NULL,
    permission character varying(255) DEFAULT 'only_me'::character varying NOT NULL,
    data_source_type character varying(255),
    indexing_technique character varying(255),
    index_struct text,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_by uuid,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    embedding_model character varying(255) DEFAULT 'text-embedding-ada-002'::character varying,
    embedding_model_provider character varying(255) DEFAULT 'openai'::character varying,
    collection_binding_id uuid,
    retrieval_config jsonb,
    owner uuid,
    icon text,
    icon_background character varying(255),
    built_in_field_enabled boolean DEFAULT false NOT NULL,
    icon_type character varying(255),
    enable_graph_flow boolean DEFAULT false,
    organization_id uuid,
    extraction_strategy character varying(20) DEFAULT 'openie'::character varying,
    segmentation_method character varying(50) DEFAULT 'parent_child'::character varying,
    process_rule jsonb
);


--
-- Name: document_segment_questions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.document_segment_questions (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    organization_id uuid NOT NULL,
    dataset_id uuid NOT NULL,
    document_id uuid NOT NULL,
    segment_id uuid NOT NULL,
    question text NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_by uuid,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    status character varying(255) DEFAULT 'waiting'::character varying NOT NULL,
    indexing_at timestamp with time zone,
    completed_at timestamp with time zone,
    error text
);


--
-- Name: document_segments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.document_segments (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    organization_id uuid NOT NULL,
    dataset_id uuid NOT NULL,
    document_id uuid NOT NULL,
    "position" integer NOT NULL,
    content text NOT NULL,
    word_count integer NOT NULL,
    tokens integer NOT NULL,
    keywords json,
    index_node_id character varying(255),
    index_node_hash character varying(255),
    hit_count integer NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    disabled_at timestamp with time zone,
    disabled_by uuid,
    status character varying(255) DEFAULT 'waiting'::character varying NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    indexing_at timestamp with time zone,
    completed_at timestamp with time zone,
    error text,
    stopped_at timestamp with time zone,
    answer text,
    updated_by uuid,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    graph_indexing_status character varying(50) DEFAULT 'pending'::character varying,
    is_deleted boolean DEFAULT false,
    deleted_at timestamp with time zone
);


--
-- Name: documents; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.documents (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    organization_id uuid NOT NULL,
    dataset_id uuid NOT NULL,
    "position" integer NOT NULL,
    data_source_type character varying(255) NOT NULL,
    data_source_info text,
    dataset_process_rule_id uuid,
    batch character varying(255) NOT NULL,
    name character varying(255) NOT NULL,
    created_from character varying(255) NOT NULL,
    created_by uuid NOT NULL,
    created_api_request_id uuid,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    processing_started_at timestamp with time zone,
    file_id text,
    word_count integer,
    parsing_completed_at timestamp with time zone,
    cleaning_completed_at timestamp with time zone,
    splitting_completed_at timestamp with time zone,
    tokens integer,
    indexing_latency double precision,
    completed_at timestamp with time zone,
    is_paused boolean DEFAULT false,
    paused_by uuid,
    paused_at timestamp with time zone,
    error text,
    stopped_at timestamp with time zone,
    indexing_status character varying(255) DEFAULT 'waiting'::character varying NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    disabled_at timestamp with time zone,
    disabled_by uuid,
    archived boolean DEFAULT false NOT NULL,
    archived_reason character varying(255),
    archived_by uuid,
    archived_at timestamp with time zone,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    doc_type character varying(40),
    doc_metadata jsonb,
    doc_form character varying(255) DEFAULT 'text_model'::character varying NOT NULL,
    doc_language character varying(255)
);

--
-- Name: file_favorites; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.file_favorites (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    file_id uuid NOT NULL,
    account_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: file_folder_joins; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.file_folder_joins (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    file_id uuid NOT NULL,
    folder_id uuid NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: file_folder_permissions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.file_folder_permissions (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    folder_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: file_folders; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.file_folders (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    organization_id uuid NOT NULL,
    name character varying(255) NOT NULL,
    description text,
    parent_id uuid,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_by uuid,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    icon_type character varying(255),
    icon character varying(255),
    icon_background character varying(255),
    "position" integer DEFAULT 0 NOT NULL,
    permission character varying(255) DEFAULT 'only_me'::character varying NOT NULL,
    workspace_id uuid
);


--
-- Name: graphflow_tasks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.graphflow_tasks (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    kb_id uuid NOT NULL,
    document_id uuid NOT NULL,
    segment_id uuid,
    extraction_strategy character varying(20) DEFAULT 'llm'::character varying,
    task_type character varying(50) NOT NULL,
    status character varying(50) DEFAULT 'pending'::character varying NOT NULL,
    progress integer DEFAULT 0,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    error_message text,
    retry_count integer DEFAULT 0,
    metadata jsonb DEFAULT '{}'::jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: kb_entities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.kb_entities (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    kb_id uuid NOT NULL,
    tenant_id uuid NOT NULL,
    name character varying(255) NOT NULL,
    canonical_name character varying(255) NOT NULL,
    type character varying(100) NOT NULL,
    description text,
    source_count integer DEFAULT 1,
    merged_ids jsonb DEFAULT '[]'::jsonb,
    embedding_id character varying(255),
    graph_node_id character varying(255),
    vector_state character varying(20) DEFAULT 'pending'::character varying,
    graph_state character varying(20) DEFAULT 'pending'::character varying,
    sync_error_log text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    is_deleted boolean DEFAULT false,
    deleted_at timestamp with time zone
);


--
-- Name: kb_entity_mentions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.kb_entity_mentions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    kb_id uuid NOT NULL,
    tenant_id uuid NOT NULL,
    segment_id uuid NOT NULL,
    raw_name character varying(255) NOT NULL,
    raw_type character varying(100),
    confidence double precision DEFAULT 1.0,
    entity_id uuid,
    status character varying(20) DEFAULT 'pending'::character varying,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    is_deleted boolean DEFAULT false,
    deleted_at timestamp with time zone
);


--
-- Name: kb_relationships; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.kb_relationships (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    kb_id uuid NOT NULL,
    tenant_id uuid NOT NULL,
    head_entity_id uuid NOT NULL,
    tail_entity_id uuid NOT NULL,
    relation_type character varying(100) NOT NULL,
    weight integer DEFAULT 1,
    graph_state character varying(20) DEFAULT 'pending'::character varying,
    last_synced_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    is_deleted boolean DEFAULT false,
    deleted_at timestamp with time zone
);


--
-- Name: kb_triple_mentions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.kb_triple_mentions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    kb_id uuid NOT NULL,
    tenant_id uuid NOT NULL,
    segment_id uuid NOT NULL,
    raw_subject character varying(255) NOT NULL,
    raw_predicate character varying(255) NOT NULL,
    raw_object character varying(255) NOT NULL,
    head_entity_id uuid,
    tail_entity_id uuid,
    status character varying(20) DEFAULT 'pending'::character varying,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    is_deleted boolean DEFAULT false,
    deleted_at timestamp with time zone
);


--
-- Name: kb_type_definitions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.kb_type_definitions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    dataset_id uuid NOT NULL,
    type_key character varying(100) NOT NULL,
    label_zh character varying(100),
    label_en character varying(100),
    style_config jsonb DEFAULT '{}'::jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: upload_files; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.upload_files (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    organization_id uuid NOT NULL,
    storage_type character varying(255) NOT NULL,
    key character varying(255) NOT NULL,
    name character varying(255) NOT NULL,
    size integer NOT NULL,
    extension character varying(255) NOT NULL,
    mime_type character varying(255),
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    used boolean DEFAULT false NOT NULL,
    used_by uuid,
    used_at timestamp with time zone,
    hash character varying(255),
    created_by_role character varying(255) DEFAULT 'account'::character varying NOT NULL,
    source_url text DEFAULT ''::text NOT NULL,
    content_text text,
    is_archived boolean DEFAULT false,
    archived_at timestamp with time zone,
    archived_by uuid,
    workspace_id uuid,
    is_temporary boolean DEFAULT false NOT NULL
);


--
-- Name: batch_hit_testing_tasks batch_hit_testing_tasks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.batch_hit_testing_tasks
    ADD CONSTRAINT batch_hit_testing_tasks_pkey PRIMARY KEY (task_id);


--
-- Name: child_chunks child_chunks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.child_chunks
    ADD CONSTRAINT child_chunks_pkey PRIMARY KEY (id);


--
-- Name: dataset_collection_bindings dataset_collection_bindings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dataset_collection_bindings
    ADD CONSTRAINT dataset_collection_bindings_pkey PRIMARY KEY (id);


--
-- Name: dataset_folder_joins dataset_folder_joins_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dataset_folder_joins
    ADD CONSTRAINT dataset_folder_joins_pkey PRIMARY KEY (id);


--
-- Name: dataset_folders dataset_folders_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dataset_folders
    ADD CONSTRAINT dataset_folders_pkey PRIMARY KEY (id);


--
-- Name: dataset_metadata_bindings dataset_metadata_bindings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dataset_metadata_bindings
    ADD CONSTRAINT dataset_metadata_bindings_pkey PRIMARY KEY (id);


--
-- Name: dataset_metadatas dataset_metadatas_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dataset_metadatas
    ADD CONSTRAINT dataset_metadatas_pkey PRIMARY KEY (id);


--
-- Name: dataset_permissions dataset_permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dataset_permissions
    ADD CONSTRAINT dataset_permissions_pkey PRIMARY KEY (id);


--
-- Name: dataset_process_rules dataset_process_rules_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dataset_process_rules
    ADD CONSTRAINT dataset_process_rules_pkey PRIMARY KEY (id);


--
-- Name: dataset_queries dataset_queries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dataset_queries
    ADD CONSTRAINT dataset_queries_pkey PRIMARY KEY (id);


--
-- Name: datasets datasets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.datasets
    ADD CONSTRAINT datasets_pkey PRIMARY KEY (id);


--
-- Name: document_segment_questions document_segment_questions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_segment_questions
    ADD CONSTRAINT document_segment_questions_pkey PRIMARY KEY (id);


--
-- Name: document_segments document_segments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_segments
    ADD CONSTRAINT document_segments_pkey PRIMARY KEY (id);


--
-- Name: documents documents_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documents
    ADD CONSTRAINT documents_pkey PRIMARY KEY (id);

--
-- Name: file_favorites file_favorites_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.file_favorites
    ADD CONSTRAINT file_favorites_pkey PRIMARY KEY (id);


--
-- Name: file_folder_joins file_folder_joins_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.file_folder_joins
    ADD CONSTRAINT file_folder_joins_pkey PRIMARY KEY (id);


--
-- Name: file_folder_permissions file_folder_permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.file_folder_permissions
    ADD CONSTRAINT file_folder_permissions_pkey PRIMARY KEY (id);


--
-- Name: file_folders file_folders_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.file_folders
    ADD CONSTRAINT file_folders_pkey PRIMARY KEY (id);


--
-- Name: graphflow_tasks graphflow_tasks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.graphflow_tasks
    ADD CONSTRAINT graphflow_tasks_pkey PRIMARY KEY (id);


--
-- Name: kb_entities kb_entities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kb_entities
    ADD CONSTRAINT kb_entities_pkey PRIMARY KEY (id);


--
-- Name: kb_entity_mentions kb_entity_mentions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kb_entity_mentions
    ADD CONSTRAINT kb_entity_mentions_pkey PRIMARY KEY (id);


--
-- Name: kb_relationships kb_relationships_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kb_relationships
    ADD CONSTRAINT kb_relationships_pkey PRIMARY KEY (id);


--
-- Name: kb_triple_mentions kb_triple_mentions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kb_triple_mentions
    ADD CONSTRAINT kb_triple_mentions_pkey PRIMARY KEY (id);


--
-- Name: kb_type_definitions kb_type_definitions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kb_type_definitions
    ADD CONSTRAINT kb_type_definitions_pkey PRIMARY KEY (id);


--
-- Name: dataset_collection_bindings unique_collection_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dataset_collection_bindings
    ADD CONSTRAINT unique_collection_name UNIQUE (collection_name);


--
-- Name: upload_files upload_files_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.upload_files
    ADD CONSTRAINT upload_files_pkey PRIMARY KEY (id);


--
-- Name: kb_type_definitions uq_type_per_dataset; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kb_type_definitions
    ADD CONSTRAINT uq_type_per_dataset UNIQUE (dataset_id, type_key);


--
-- Name: batch_hit_testing_tasks_dataset_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX batch_hit_testing_tasks_dataset_id_idx ON public.batch_hit_testing_tasks USING btree (dataset_id);


--
-- Name: child_chunk_organization_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX child_chunk_organization_id_idx ON public.child_chunks USING btree (organization_id, dataset_id);


--
-- Name: child_chunks_document_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX child_chunks_document_id_idx ON public.child_chunks USING btree (document_id);


--
-- Name: child_chunks_segment_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX child_chunks_segment_id_idx ON public.child_chunks USING btree (segment_id);


--
-- Name: dataset_folder_joins_dataset_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX dataset_folder_joins_dataset_id_idx ON public.dataset_folder_joins USING btree (dataset_id);


--
-- Name: dataset_folder_joins_folder_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX dataset_folder_joins_folder_id_idx ON public.dataset_folder_joins USING btree (folder_id);


--
-- Name: dataset_folder_organization_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX dataset_folder_organization_id_idx ON public.dataset_folders USING btree (organization_id);


--
-- Name: dataset_folders_tenant_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX dataset_folders_tenant_id_idx ON public.dataset_folders USING btree (workspace_id);


--
-- Name: dataset_metadata_bindings_dataset_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX dataset_metadata_bindings_dataset_id_idx ON public.dataset_metadata_bindings USING btree (dataset_id);


--
-- Name: dataset_metadata_bindings_metadata_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX dataset_metadata_bindings_metadata_id_idx ON public.dataset_metadata_bindings USING btree (metadata_id);


--
-- Name: dataset_metadatas_dataset_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX dataset_metadatas_dataset_id_idx ON public.dataset_metadatas USING btree (dataset_id);


--
-- Name: dataset_metadatas_tenant_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX dataset_metadatas_tenant_id_idx ON public.dataset_metadatas USING btree (tenant_id);


--
-- Name: dataset_organization_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX dataset_organization_id_idx ON public.datasets USING btree (organization_id);


--
-- Name: dataset_permissions_account_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX dataset_permissions_account_id_idx ON public.dataset_permissions USING btree (account_id);


--
-- Name: dataset_permissions_dataset_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX dataset_permissions_dataset_id_idx ON public.dataset_permissions USING btree (dataset_id);


--
-- Name: dataset_process_rules_dataset_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX dataset_process_rules_dataset_id_idx ON public.dataset_process_rules USING btree (dataset_id);


--
-- Name: dataset_queries_batch_task_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX dataset_queries_batch_task_id_idx ON public.dataset_queries USING btree (batch_task_id);


--
-- Name: dataset_queries_dataset_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX dataset_queries_dataset_id_idx ON public.dataset_queries USING btree (dataset_id);


--
-- Name: dataset_tenant_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX dataset_tenant_id_idx ON public.datasets USING btree (workspace_id);


--
-- Name: document_organization_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX document_organization_id_idx ON public.documents USING btree (organization_id);


--
-- Name: document_segment_organization_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX document_segment_organization_id_idx ON public.document_segments USING btree (organization_id);


--
-- Name: document_segment_question_organization_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX document_segment_question_organization_id_idx ON public.document_segment_questions USING btree (organization_id);


--
-- Name: document_segment_questions_dataset_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX document_segment_questions_dataset_id_idx ON public.document_segment_questions USING btree (dataset_id);


--
-- Name: document_segment_questions_document_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX document_segment_questions_document_id_idx ON public.document_segment_questions USING btree (document_id);


--
-- Name: document_segment_questions_segment_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX document_segment_questions_segment_id_idx ON public.document_segment_questions USING btree (segment_id);


--
-- Name: document_segment_questions_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX document_segment_questions_status_idx ON public.document_segment_questions USING btree (status);


--
-- Name: document_segments_dataset_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX document_segments_dataset_id_idx ON public.document_segments USING btree (dataset_id);


--
-- Name: document_segments_document_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX document_segments_document_id_idx ON public.document_segments USING btree (document_id);


--
-- Name: documents_dataset_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX documents_dataset_id_idx ON public.documents USING btree (dataset_id);

--
-- Name: file_favorites_account_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX file_favorites_account_id_idx ON public.file_favorites USING btree (account_id);


--
-- Name: file_favorites_file_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX file_favorites_file_id_idx ON public.file_favorites USING btree (file_id);


--
-- Name: file_folder_assoc_file_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX file_folder_assoc_file_idx ON public.file_folder_joins USING btree (file_id);


--
-- Name: file_folder_assoc_folder_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX file_folder_assoc_folder_idx ON public.file_folder_joins USING btree (folder_id);


--
-- Name: file_folder_joins_file_id_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX file_folder_joins_file_id_unique ON public.file_folder_joins USING btree (file_id);


--
-- Name: file_folder_organization_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX file_folder_organization_idx ON public.file_folders USING btree (organization_id);


--
-- Name: file_folder_parent_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX file_folder_parent_idx ON public.file_folders USING btree (parent_id);


--
-- Name: file_folder_permission_folder_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX file_folder_permission_folder_idx ON public.file_folder_permissions USING btree (folder_id);


--
-- Name: file_folder_permission_workspace_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX file_folder_permission_workspace_idx ON public.file_folder_permissions USING btree (workspace_id);


--
-- Name: file_folders_team_tenant_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX file_folders_team_tenant_id_idx ON public.file_folders USING btree (workspace_id);


--
-- Name: idx_batch_hit_testing_tasks_organization_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_batch_hit_testing_tasks_organization_id ON public.batch_hit_testing_tasks USING btree (organization_id);


--
-- Name: idx_datasets_segmentation_method; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_datasets_segmentation_method ON public.datasets USING btree (segmentation_method);


--
-- Name: idx_document_segments_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_document_segments_deleted ON public.document_segments USING btree (is_deleted) WHERE (is_deleted = false);


--
-- Name: idx_document_segments_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_document_segments_deleted_at ON public.document_segments USING btree (deleted_at);


--
-- Name: idx_document_segments_graph_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_document_segments_graph_status ON public.document_segments USING btree (graph_indexing_status);


--
-- Name: idx_document_segments_is_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_document_segments_is_deleted ON public.document_segments USING btree (is_deleted) WHERE (is_deleted = false);


--
-- Name: idx_entities_sync_check; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_entities_sync_check ON public.kb_entities USING btree (kb_id, vector_state, graph_state);


--
-- Name: idx_entities_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_entities_tenant ON public.kb_entities USING btree (tenant_id);


--
-- Name: idx_entities_unique_identity; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_entities_unique_identity ON public.kb_entities USING btree (kb_id, canonical_name) WHERE (is_deleted = false);


--
-- Name: idx_entity_mentions_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_entity_mentions_deleted ON public.kb_entity_mentions USING btree (is_deleted) WHERE (is_deleted = false);


--
-- Name: idx_graphflow_tasks_document; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_graphflow_tasks_document ON public.graphflow_tasks USING btree (document_id, task_type);


--
-- Name: idx_graphflow_tasks_pending; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_graphflow_tasks_pending ON public.graphflow_tasks USING btree (kb_id, status) WHERE ((status)::text = ANY (ARRAY[('pending'::character varying)::text, ('processing'::character varying)::text]));


--
-- Name: idx_graphflow_tasks_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_graphflow_tasks_tenant ON public.graphflow_tasks USING btree (tenant_id);


--
-- Name: idx_kb_graph_flow; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_kb_graph_flow ON public.datasets USING btree (enable_graph_flow) WHERE (enable_graph_flow = true);


--
-- Name: idx_mentions_entity_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mentions_entity_id ON public.kb_entity_mentions USING btree (entity_id);


--
-- Name: idx_mentions_pending_task; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mentions_pending_task ON public.kb_entity_mentions USING btree (kb_id, status) WHERE ((status)::text = 'pending'::text);


--
-- Name: idx_relationships_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_relationships_deleted ON public.kb_relationships USING btree (is_deleted) WHERE (is_deleted = false);


--
-- Name: idx_triple_mentions_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_triple_mentions_deleted ON public.kb_triple_mentions USING btree (is_deleted) WHERE (is_deleted = false);


--
-- Name: idx_triple_mentions_task; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_triple_mentions_task ON public.kb_triple_mentions USING btree (kb_id, status) WHERE ((status)::text = 'pending'::text);


--
-- Name: idx_type_definitions_dataset; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_type_definitions_dataset ON public.kb_type_definitions USING btree (dataset_id);


--
-- Name: idx_unique_relationship_fact; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_unique_relationship_fact ON public.kb_relationships USING btree (kb_id, head_entity_id, tail_entity_id, relation_type);


--
-- Name: idx_upload_files_organization_archived; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_upload_files_organization_archived ON public.upload_files USING btree (organization_id, is_archived);


--
-- Name: idx_upload_files_organization_archived_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_upload_files_organization_archived_created ON public.upload_files USING btree (organization_id, is_archived, created_at);


--
-- Name: upload_file_organization_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX upload_file_organization_idx ON public.upload_files USING btree (organization_id);


--
-- Name: upload_files_created_by_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX upload_files_created_by_idx ON public.upload_files USING btree (created_by);


--
-- Name: upload_files_organization_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX upload_files_organization_id_idx ON public.upload_files USING btree (organization_id);


--
-- Name: upload_files_workspace_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX upload_files_workspace_id_idx ON public.upload_files USING btree (workspace_id);


--
-- Name: batch_hit_testing_tasks fk_batch_hit_testing_task_dataset; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.batch_hit_testing_tasks
    ADD CONSTRAINT fk_batch_hit_testing_task_dataset FOREIGN KEY (dataset_id) REFERENCES public.datasets(id) ON DELETE CASCADE;


--
-- Name: batch_hit_testing_tasks fk_batch_hit_testing_task_tenant; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.batch_hit_testing_tasks
    ADD CONSTRAINT fk_batch_hit_testing_task_tenant FOREIGN KEY (organization_id) REFERENCES public.workspaces(id) ON DELETE CASCADE;


--
-- Name: dataset_folder_joins fk_dataset_folder_join_dataset; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dataset_folder_joins
    ADD CONSTRAINT fk_dataset_folder_join_dataset FOREIGN KEY (dataset_id) REFERENCES public.datasets(id) ON DELETE CASCADE;


--
-- Name: dataset_folder_joins fk_dataset_folder_join_folder; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dataset_folder_joins
    ADD CONSTRAINT fk_dataset_folder_join_folder FOREIGN KEY (folder_id) REFERENCES public.dataset_folders(id) ON DELETE CASCADE;


--
-- Name: dataset_folders fk_dataset_folder_tenant; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dataset_folders
    ADD CONSTRAINT fk_dataset_folder_tenant FOREIGN KEY (workspace_id) REFERENCES public.workspaces(id) ON DELETE CASCADE;


--
-- Name: document_segment_questions fk_document_segment_question_dataset; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_segment_questions
    ADD CONSTRAINT fk_document_segment_question_dataset FOREIGN KEY (dataset_id) REFERENCES public.datasets(id) ON DELETE CASCADE;


--
-- Name: document_segment_questions fk_document_segment_question_document; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_segment_questions
    ADD CONSTRAINT fk_document_segment_question_document FOREIGN KEY (document_id) REFERENCES public.documents(id) ON DELETE CASCADE;


--
-- Name: document_segment_questions fk_document_segment_question_segment; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_segment_questions
    ADD CONSTRAINT fk_document_segment_question_segment FOREIGN KEY (segment_id) REFERENCES public.document_segments(id) ON DELETE CASCADE;


--
-- Name: document_segment_questions fk_document_segment_question_tenant; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_segment_questions
    ADD CONSTRAINT fk_document_segment_question_tenant FOREIGN KEY (organization_id) REFERENCES public.workspaces(id) ON DELETE CASCADE;


--
-- Name: kb_entities fk_entities_kb; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kb_entities
    ADD CONSTRAINT fk_entities_kb FOREIGN KEY (kb_id) REFERENCES public.datasets(id) ON DELETE CASCADE;


--
-- Name: graphflow_tasks fk_graphflow_tasks_document; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.graphflow_tasks
    ADD CONSTRAINT fk_graphflow_tasks_document FOREIGN KEY (document_id) REFERENCES public.documents(id) ON DELETE CASCADE;


--
-- Name: graphflow_tasks fk_graphflow_tasks_kb; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.graphflow_tasks
    ADD CONSTRAINT fk_graphflow_tasks_kb FOREIGN KEY (kb_id) REFERENCES public.datasets(id) ON DELETE CASCADE;


--
-- Name: kb_entity_mentions fk_mentions_entity; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kb_entity_mentions
    ADD CONSTRAINT fk_mentions_entity FOREIGN KEY (entity_id) REFERENCES public.kb_entities(id) ON DELETE SET NULL;


--
-- Name: kb_entity_mentions fk_mentions_kb; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kb_entity_mentions
    ADD CONSTRAINT fk_mentions_kb FOREIGN KEY (kb_id) REFERENCES public.datasets(id) ON DELETE CASCADE;


--
-- Name: kb_entity_mentions fk_mentions_segment; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kb_entity_mentions
    ADD CONSTRAINT fk_mentions_segment FOREIGN KEY (segment_id) REFERENCES public.document_segments(id) ON DELETE CASCADE;


--
-- Name: kb_relationships fk_rels_head; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kb_relationships
    ADD CONSTRAINT fk_rels_head FOREIGN KEY (head_entity_id) REFERENCES public.kb_entities(id) ON DELETE CASCADE;


--
-- Name: kb_relationships fk_rels_kb; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kb_relationships
    ADD CONSTRAINT fk_rels_kb FOREIGN KEY (kb_id) REFERENCES public.datasets(id) ON DELETE CASCADE;


--
-- Name: kb_relationships fk_rels_tail; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kb_relationships
    ADD CONSTRAINT fk_rels_tail FOREIGN KEY (tail_entity_id) REFERENCES public.kb_entities(id) ON DELETE CASCADE;


--
-- Name: kb_triple_mentions fk_triple_mentions_head; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kb_triple_mentions
    ADD CONSTRAINT fk_triple_mentions_head FOREIGN KEY (head_entity_id) REFERENCES public.kb_entities(id) ON DELETE SET NULL;


--
-- Name: kb_triple_mentions fk_triple_mentions_kb; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kb_triple_mentions
    ADD CONSTRAINT fk_triple_mentions_kb FOREIGN KEY (kb_id) REFERENCES public.datasets(id) ON DELETE CASCADE;


--
-- Name: kb_triple_mentions fk_triple_mentions_segment; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kb_triple_mentions
    ADD CONSTRAINT fk_triple_mentions_segment FOREIGN KEY (segment_id) REFERENCES public.document_segments(id) ON DELETE CASCADE;


--
-- Name: kb_triple_mentions fk_triple_mentions_tail; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kb_triple_mentions
    ADD CONSTRAINT fk_triple_mentions_tail FOREIGN KEY (tail_entity_id) REFERENCES public.kb_entities(id) ON DELETE SET NULL;


--
-- Name: kb_type_definitions fk_type_definitions_dataset; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kb_type_definitions
    ADD CONSTRAINT fk_type_definitions_dataset FOREIGN KEY (dataset_id) REFERENCES public.datasets(id) ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--

\unrestrict HfCAsdBKLrQAIpPKsvNWGpJyu9Hd66KWNyPA9e0AP6Uiwz8QM9PUewIk7WkQwWT

