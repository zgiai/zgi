--
-- PostgreSQL database dump
--

\restrict hOssKoUSrwAJFHRTqjrO3rYwmIDxgFpaNXft0weuZ25rtebr4kN5EsBxlsw0NW9

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
-- Name: account_plugin_installations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.account_plugin_installations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    marketplace_plugin_id uuid NOT NULL,
    marketplace_version_id uuid NOT NULL,
    installed_by uuid NOT NULL,
    installed_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    status character varying(20) DEFAULT 'active'::character varying
);


--
-- Name: TABLE account_plugin_installations; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.account_plugin_installations IS 'Tenant to plugin installation relationships';


--
-- Name: COLUMN account_plugin_installations.tenant_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.account_plugin_installations.tenant_id IS 'Organization/Tenant ID where the plugin is installed';


--
-- Name: COLUMN account_plugin_installations.marketplace_plugin_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.account_plugin_installations.marketplace_plugin_id IS 'Marketplace plugin ID for redundancy and easy querying';


--
-- Name: COLUMN account_plugin_installations.marketplace_version_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.account_plugin_installations.marketplace_version_id IS 'Marketplace version ID - lookup declaration via plugin_declarations table';


--
-- Name: COLUMN account_plugin_installations.installed_by; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.account_plugin_installations.installed_by IS 'User ID who installed this plugin';


--
-- Name: COLUMN account_plugin_installations.status; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.account_plugin_installations.status IS 'Installation status: active, disabled';


--
-- Name: data_retention_policies; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.data_retention_policies (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    data_type character varying(50) NOT NULL,
    description text,
    retention_days integer NOT NULL,
    anonymize_after_days integer,
    hard_delete_after_days integer,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: data_source_sql_operations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.data_source_sql_operations (
    id uuid NOT NULL,
    organization_id uuid NOT NULL,
    data_source_id uuid NOT NULL,
    table_id uuid,
    data_source_name character varying(255),
    table_name character varying(255),
    sql_statement text NOT NULL,
    operation_type character varying(20) NOT NULL,
    start_time timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    end_time timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    status character varying(10) NOT NULL,
    created_by character varying(36) NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: data_source_table_prompts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.data_source_table_prompts (
    id uuid NOT NULL,
    table_id uuid NOT NULL,
    prompt text NOT NULL,
    created_by character varying(36) NOT NULL,
    updated_by character varying(36) NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: data_source_tables; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.data_source_tables (
    id uuid NOT NULL,
    organization_id uuid NOT NULL,
    data_source_id uuid NOT NULL,
    name character varying(255) NOT NULL,
    table_id integer NOT NULL,
    table_name character varying(255) NOT NULL,
    description text,
    created_by character varying(36) NOT NULL,
    updated_by character varying(36) NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: data_sources; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.data_sources (
    id uuid NOT NULL,
    organization_id uuid NOT NULL,
    workspace_id uuid,
    name character varying(255) NOT NULL,
    schema_id integer DEFAULT 0 NOT NULL,
    schema_name character varying(255) NOT NULL,
    description text,
    permission character varying(50) DEFAULT 'only_me'::character varying NOT NULL,
    status character varying(50) DEFAULT 'active'::character varying NOT NULL,
    icon_type character varying(255),
    icon text,
    icon_background character varying(255),
    created_by character varying(36) NOT NULL,
    updated_by character varying(36) NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: enterprise_group_plugin_subscriptions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.enterprise_group_plugin_subscriptions (
    id integer NOT NULL,
    group_id uuid NOT NULL,
    plugin_id character varying(255) NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    config text,
    subscribed_by uuid,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    account_id uuid,
    installation_id uuid
);


--
-- Name: TABLE enterprise_group_plugin_subscriptions; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.enterprise_group_plugin_subscriptions IS 'Plugin subscriptions at organization level';


--
-- Name: COLUMN enterprise_group_plugin_subscriptions.account_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.enterprise_group_plugin_subscriptions.account_id IS 'Member account ID who subscribed to the plugin';


--
-- Name: COLUMN enterprise_group_plugin_subscriptions.installation_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.enterprise_group_plugin_subscriptions.installation_id IS 'Reference to account_plugin_installations.id';


--
-- Name: enterprise_group_plugin_subscriptions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.enterprise_group_plugin_subscriptions_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: enterprise_group_plugin_subscriptions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.enterprise_group_plugin_subscriptions_id_seq OWNED BY public.enterprise_group_plugin_subscriptions.id;


--
-- Name: gdpr_audit_logs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gdpr_audit_logs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    action_type character varying(50) NOT NULL,
    actor_id uuid,
    actor_email character varying(255),
    subject_id uuid NOT NULL,
    subject_email character varying(255),
    tenant_id uuid,
    details jsonb DEFAULT '{}'::jsonb,
    ip_address character varying(45),
    user_agent text,
    status character varying(20) DEFAULT 'completed'::character varying NOT NULL,
    error_message text,
    created_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: installed_plugin_info; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.installed_plugin_info (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    marketplace_plugin_id uuid,
    marketplace_version_id uuid NOT NULL,
    plugin_name character varying(100) NOT NULL,
    plugin_version character varying(50) NOT NULL,
    plugin_author character varying(100),
    declaration jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: TABLE installed_plugin_info; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.installed_plugin_info IS 'Installed plugin info, one record per marketplace version';


--
-- Name: COLUMN installed_plugin_info.marketplace_plugin_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.installed_plugin_info.marketplace_plugin_id IS 'Marketplace plugin ID for redundancy';


--
-- Name: COLUMN installed_plugin_info.declaration; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.installed_plugin_info.declaration IS 'JSONB containing provider and tools definition';


--
-- Name: plugin_installations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.plugin_installations (
    id character varying(255) NOT NULL,
    tenant_id character varying(255) NOT NULL,
    plugin_unique_identifier character varying(255) NOT NULL,
    version character varying(100),
    source character varying(50) NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    installed_at timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);

--
-- Name: seed_executions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.seed_executions (
    name character varying(100) NOT NULL,
    version character varying(50) NOT NULL,
    executed_at timestamp with time zone DEFAULT now() NOT NULL,
    executed_by character varying(50) DEFAULT 'manual'::character varying NOT NULL,
    status character varying(20) DEFAULT 'success'::character varying NOT NULL,
    PRIMARY KEY (name, version)
);


--
-- Name: user_consents; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_consents (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    account_id uuid NOT NULL,
    consent_type character varying(50) NOT NULL,
    is_granted boolean NOT NULL,
    granted_at timestamp without time zone,
    revoked_at timestamp without time zone,
    ip_address character varying(45),
    user_agent text,
    version character varying(20) DEFAULT '1.0'::character varying,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: zgi_setups; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.zgi_setups (
    version character varying(255) NOT NULL,
    setup_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: enterprise_group_plugin_subscriptions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.enterprise_group_plugin_subscriptions ALTER COLUMN id SET DEFAULT nextval('public.enterprise_group_plugin_subscriptions_id_seq'::regclass);


--
-- Name: account_plugin_installations account_plugin_installations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_plugin_installations
    ADD CONSTRAINT account_plugin_installations_pkey PRIMARY KEY (id);


--
-- Name: data_retention_policies data_retention_policies_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.data_retention_policies
    ADD CONSTRAINT data_retention_policies_pkey PRIMARY KEY (id);


--
-- Name: data_source_sql_operations data_source_sql_operations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.data_source_sql_operations
    ADD CONSTRAINT data_source_sql_operations_pkey PRIMARY KEY (id);


--
-- Name: data_source_table_prompts data_source_table_prompts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.data_source_table_prompts
    ADD CONSTRAINT data_source_table_prompts_pkey PRIMARY KEY (id);


--
-- Name: data_source_tables data_source_tables_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.data_source_tables
    ADD CONSTRAINT data_source_tables_pkey PRIMARY KEY (id);


--
-- Name: data_sources data_sources_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.data_sources
    ADD CONSTRAINT data_sources_pkey PRIMARY KEY (id);


--
-- Name: enterprise_group_plugin_subscriptions enterprise_group_plugin_subscriptions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.enterprise_group_plugin_subscriptions
    ADD CONSTRAINT enterprise_group_plugin_subscriptions_pkey PRIMARY KEY (id);


--
-- Name: gdpr_audit_logs gdpr_audit_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gdpr_audit_logs
    ADD CONSTRAINT gdpr_audit_logs_pkey PRIMARY KEY (id);


--
-- Name: enterprise_group_plugin_subscriptions idx_group_account_installation_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.enterprise_group_plugin_subscriptions
    ADD CONSTRAINT idx_group_account_installation_unique UNIQUE (group_id, account_id, installation_id);


--
-- Name: installed_plugin_info installed_plugin_info_marketplace_version_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.installed_plugin_info
    ADD CONSTRAINT installed_plugin_info_marketplace_version_id_key UNIQUE (marketplace_version_id);


--
-- Name: installed_plugin_info installed_plugin_info_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.installed_plugin_info
    ADD CONSTRAINT installed_plugin_info_pkey PRIMARY KEY (id);


--
-- Name: plugin_installations plugin_installations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plugin_installations
    ADD CONSTRAINT plugin_installations_pkey PRIMARY KEY (id);

--
-- Name: account_plugin_installations uq_account_plugin_installations_tenant_version; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_plugin_installations
    ADD CONSTRAINT uq_account_plugin_installations_tenant_version UNIQUE (tenant_id, marketplace_version_id);


--
-- Name: data_retention_policies uq_data_retention_policies_data_type; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.data_retention_policies
    ADD CONSTRAINT uq_data_retention_policies_data_type UNIQUE (data_type);


--
-- Name: user_consents user_consents_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_consents
    ADD CONSTRAINT user_consents_pkey PRIMARY KEY (id);


--
-- Name: zgi_setups zgi_setups_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.zgi_setups
    ADD CONSTRAINT zgi_setups_pkey PRIMARY KEY (version);


--
-- Name: idx_account_plugin_installations_plugin; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_account_plugin_installations_plugin ON public.account_plugin_installations USING btree (tenant_id, marketplace_plugin_id);


--
-- Name: idx_account_plugin_installations_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_account_plugin_installations_tenant ON public.account_plugin_installations USING btree (tenant_id);


--
-- Name: idx_account_plugin_installations_version; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_account_plugin_installations_version ON public.account_plugin_installations USING btree (marketplace_version_id);


--
-- Name: idx_data_source_sql_operations_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_data_source_sql_operations_created_at ON public.data_source_sql_operations USING btree (created_at);


--
-- Name: idx_data_source_sql_operations_data_source_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_data_source_sql_operations_data_source_id ON public.data_source_sql_operations USING btree (data_source_id);


--
-- Name: idx_data_source_sql_operations_organization_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_data_source_sql_operations_organization_id ON public.data_source_sql_operations USING btree (organization_id);


--
-- Name: idx_data_source_sql_operations_table_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_data_source_sql_operations_table_id ON public.data_source_sql_operations USING btree (table_id);


--
-- Name: idx_data_source_tables_data_source_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_data_source_tables_data_source_id ON public.data_source_tables USING btree (data_source_id);


--
-- Name: idx_data_source_tables_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_data_source_tables_name ON public.data_source_tables USING btree (name);


--
-- Name: idx_data_source_tables_organization_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_data_source_tables_organization_id ON public.data_source_tables USING btree (organization_id);


--
-- Name: idx_data_sources_organization_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_data_sources_organization_id ON public.data_sources USING btree (organization_id);


--
-- Name: idx_egps_account_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_egps_account_id ON public.enterprise_group_plugin_subscriptions USING btree (account_id);


--
-- Name: idx_egps_enabled; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_egps_enabled ON public.enterprise_group_plugin_subscriptions USING btree (group_id, enabled) WHERE (enabled = true);


--
-- Name: idx_egps_group_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_egps_group_id ON public.enterprise_group_plugin_subscriptions USING btree (group_id);


--
-- Name: idx_egps_installation_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_egps_installation_id ON public.enterprise_group_plugin_subscriptions USING btree (installation_id);


--
-- Name: idx_egps_plugin_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_egps_plugin_id ON public.enterprise_group_plugin_subscriptions USING btree (plugin_id);


--
-- Name: idx_gdpr_audit_logs_action_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_gdpr_audit_logs_action_type ON public.gdpr_audit_logs USING btree (action_type);


--
-- Name: idx_gdpr_audit_logs_actor_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_gdpr_audit_logs_actor_id ON public.gdpr_audit_logs USING btree (actor_id);


--
-- Name: idx_gdpr_audit_logs_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_gdpr_audit_logs_created_at ON public.gdpr_audit_logs USING btree (created_at);


--
-- Name: idx_gdpr_audit_logs_subject_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_gdpr_audit_logs_subject_id ON public.gdpr_audit_logs USING btree (subject_id);


--
-- Name: idx_gdpr_audit_logs_tenant_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_gdpr_audit_logs_tenant_id ON public.gdpr_audit_logs USING btree (tenant_id);


--
-- Name: idx_installed_plugin_info_marketplace_plugin; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_installed_plugin_info_marketplace_plugin ON public.installed_plugin_info USING btree (marketplace_plugin_id);


--
-- Name: idx_installed_plugin_info_plugin_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_installed_plugin_info_plugin_name ON public.installed_plugin_info USING btree (plugin_name);


--
-- Name: idx_plugin_installations_tenant_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_plugin_installations_tenant_id ON public.plugin_installations USING btree (tenant_id);


--
-- Name: idx_plugin_installations_tenant_plugin; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_plugin_installations_tenant_plugin ON public.plugin_installations USING btree (tenant_id, plugin_unique_identifier);

--
-- Name: idx_table_prompts_table_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_table_prompts_table_id ON public.data_source_table_prompts USING btree (table_id);


--
-- Name: idx_user_consents_account_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_consents_account_id ON public.user_consents USING btree (account_id);


--
-- Name: idx_user_consents_account_type; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_user_consents_account_type ON public.user_consents USING btree (account_id, consent_type);


--
-- Name: idx_user_consents_consent_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_consents_consent_type ON public.user_consents USING btree (consent_type);


--
-- PostgreSQL database dump complete
--

\unrestrict hOssKoUSrwAJFHRTqjrO3rYwmIDxgFpaNXft0weuZ25rtebr4kN5EsBxlsw0NW9

