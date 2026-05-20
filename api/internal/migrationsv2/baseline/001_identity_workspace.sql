--
-- PostgreSQL database dump
--

\restrict YgIGeFc1bLvKbhfNvKUGyTKeq6qTXSXWyBiYH4reFhP1IxmdgSA07AhbCJFY4mR

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
-- Name: account_contexts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.account_contexts (
    account_id uuid NOT NULL,
    current_organization_id uuid,
    current_workspace_id uuid,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: account_integrates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.account_integrates (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    account_id uuid NOT NULL,
    provider character varying(16) NOT NULL,
    open_id character varying(255) NOT NULL,
    encrypted_token character varying(255) NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: accounts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.accounts (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    name character varying(255) NOT NULL,
    email character varying(255) DEFAULT ''::character varying NOT NULL,
    password character varying(255),
    password_salt character varying(255),
    avatar character varying(255),
    interface_language character varying(255),
    interface_theme character varying(255),
    timezone character varying(255),
    last_login_at timestamp with time zone,
    last_login_ip character varying(255),
    status character varying(16) DEFAULT 'active'::character varying NOT NULL,
    initialized_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    last_active_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    extensions jsonb,
    mobile_e164 character varying(32),
    deleted_at timestamp with time zone
);


--
-- Name: department_members; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.department_members (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    department_id uuid NOT NULL,
    account_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: TABLE department_members; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.department_members IS 'Department member association table';


--
-- Name: COLUMN department_members.department_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.department_members.department_id IS 'Department ID';


--
-- Name: COLUMN department_members.account_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.department_members.account_id IS 'Account ID';


--
-- Name: departments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.departments (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    group_id uuid NOT NULL,
    parent_id uuid,
    name character varying(255) NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    status character varying(16) DEFAULT 'active'::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    created_by uuid
);


--
-- Name: TABLE departments; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.departments IS 'Department table for contacts module';


--
-- Name: COLUMN departments.group_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.departments.group_id IS 'Enterprise group ID';


--
-- Name: COLUMN departments.parent_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.departments.parent_id IS 'Parent department ID, NULL for root department';


--
-- Name: COLUMN departments.name; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.departments.name IS 'Department name';


--
-- Name: COLUMN departments.sort_order; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.departments.sort_order IS 'Sort order within same parent';


--
-- Name: COLUMN departments.status; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.departments.status IS 'Department status: active/archived';


--
-- Name: end_users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.end_users (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    tenant_id uuid NOT NULL,
    app_id uuid,
    type character varying(255) NOT NULL,
    external_user_id character varying(255),
    name character varying(255),
    is_anonymous boolean DEFAULT true NOT NULL,
    session_id character varying(255) NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: members; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.members (
    organization_id uuid NOT NULL,
    account_id uuid NOT NULL,
    role character varying(16) DEFAULT 'normal'::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    status character varying(16) DEFAULT 'active'::character varying NOT NULL,
    name character varying(255)
);


--
-- Name: COLUMN members.status; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.members.status IS 'Member status: active/inactive';


--
-- Name: roles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.roles (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    group_id uuid NOT NULL,
    name character varying(255) NOT NULL,
    description text,
    status character varying(16) DEFAULT 'active'::character varying NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    permissions jsonb DEFAULT '[]'::jsonb
);


--
-- Name: organizations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.organizations (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    name character varying(255) NOT NULL,
    status character varying(16) DEFAULT 'active'::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    short_name character varying(100)
);


--
-- Name: organization_invite_links; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.organization_invite_links (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    group_id uuid NOT NULL,
    department_id uuid,
    tenant_id uuid,
    token character varying(255) NOT NULL,
    status character varying(32) NOT NULL,
    require_approval boolean DEFAULT true NOT NULL,
    default_group_role character varying(32) DEFAULT 'normal'::character varying NOT NULL,
    default_tenant_role character varying(32) DEFAULT 'normal'::character varying NOT NULL,
    expires_at timestamp with time zone,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: TABLE organization_invite_links; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.organization_invite_links IS 'Enterprise invite links for departments and tenants';


--
-- Name: COLUMN organization_invite_links.group_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.organization_invite_links.group_id IS 'Enterprise group ID';


--
-- Name: COLUMN organization_invite_links.department_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.organization_invite_links.department_id IS 'Target department ID (current phase only department invites are used)';


--
-- Name: COLUMN organization_invite_links.tenant_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.organization_invite_links.tenant_id IS 'Target tenant ID (reserved for future use)';


--
-- Name: COLUMN organization_invite_links.token; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.organization_invite_links.token IS 'Invite token, high entropy random string';


--
-- Name: COLUMN organization_invite_links.status; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.organization_invite_links.status IS 'Invite link status: enabled/disabled/expired/reset';


--
-- Name: COLUMN organization_invite_links.require_approval; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.organization_invite_links.require_approval IS 'Whether join request requires admin approval';


--
-- Name: COLUMN organization_invite_links.default_group_role; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.organization_invite_links.default_group_role IS 'Default enterprise group role for joined member';


--
-- Name: COLUMN organization_invite_links.default_tenant_role; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.organization_invite_links.default_tenant_role IS 'Default tenant role for joined member (reserved)';


--
-- Name: COLUMN organization_invite_links.expires_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.organization_invite_links.expires_at IS 'Expire time of the invite link, null means never expires';


--
-- Name: organization_join_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.organization_join_requests (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    group_id uuid NOT NULL,
    invite_link_id uuid,
    account_id uuid NOT NULL,
    department_id uuid,
    tenant_id uuid,
    default_group_role character varying(32) NOT NULL,
    default_tenant_role character varying(32) NOT NULL,
    status character varying(32) NOT NULL,
    reason text,
    reviewer_id uuid,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    reviewed_at timestamp with time zone,
    name character varying(255)
);


--
-- Name: TABLE organization_join_requests; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.organization_join_requests IS 'Enterprise join requests created by invite links or admin actions';


--
-- Name: COLUMN organization_join_requests.group_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.organization_join_requests.group_id IS 'Enterprise group ID';


--
-- Name: COLUMN organization_join_requests.invite_link_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.organization_join_requests.invite_link_id IS 'Related invite link ID, null if created directly by admin';


--
-- Name: COLUMN organization_join_requests.account_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.organization_join_requests.account_id IS 'Applicant account ID';


--
-- Name: COLUMN organization_join_requests.department_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.organization_join_requests.department_id IS 'Target department ID';


--
-- Name: COLUMN organization_join_requests.tenant_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.organization_join_requests.tenant_id IS 'Target tenant ID (reserved for future use)';


--
-- Name: COLUMN organization_join_requests.default_group_role; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.organization_join_requests.default_group_role IS 'Default enterprise group role for applicant';


--
-- Name: COLUMN organization_join_requests.default_tenant_role; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.organization_join_requests.default_tenant_role IS 'Default tenant role for applicant (reserved)';


--
-- Name: COLUMN organization_join_requests.status; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.organization_join_requests.status IS 'Join request status: pending/approved/rejected/expired';


--
-- Name: COLUMN organization_join_requests.reason; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.organization_join_requests.reason IS 'Additional reason or review comment';


--
-- Name: COLUMN organization_join_requests.reviewer_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.organization_join_requests.reviewer_id IS 'Reviewer account ID';


--
-- Name: workspaces; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.workspaces (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    name character varying(255) NOT NULL,
    encrypt_public_key text,
    plan character varying(255) DEFAULT 'basic'::character varying NOT NULL,
    status character varying(255) DEFAULT 'normal'::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    custom_config text,
    organization_id uuid,
    department_id uuid,
    api_key_id uuid
);


--
-- Name: workspace_members; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.workspace_members (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    workspace_id uuid NOT NULL,
    account_id uuid NOT NULL,
    role character varying(16) DEFAULT 'normal'::character varying NOT NULL,
    invited_by uuid,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    current boolean DEFAULT false NOT NULL,
    role_id uuid,
    extensions jsonb DEFAULT '{}'::jsonb
);


--
-- Name: account_contexts account_contexts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_contexts
    ADD CONSTRAINT account_contexts_pkey PRIMARY KEY (account_id);


--
-- Name: account_integrates account_integrate_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_integrates
    ADD CONSTRAINT account_integrate_pkey PRIMARY KEY (id);


--
-- Name: accounts account_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.accounts
    ADD CONSTRAINT account_pkey PRIMARY KEY (id);


--
-- Name: department_members department_members_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.department_members
    ADD CONSTRAINT department_members_pkey PRIMARY KEY (id);


--
-- Name: departments departments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.departments
    ADD CONSTRAINT departments_pkey PRIMARY KEY (id);


--
-- Name: end_users end_user_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.end_users
    ADD CONSTRAINT end_user_pkey PRIMARY KEY (id);


--
-- Name: organizations enterprise_group_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organizations
    ADD CONSTRAINT enterprise_group_pkey PRIMARY KEY (id);


--
-- Name: organization_invite_links enterprise_invite_links_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organization_invite_links
    ADD CONSTRAINT enterprise_invite_links_pkey PRIMARY KEY (id);


--
-- Name: organization_join_requests enterprise_join_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organization_join_requests
    ADD CONSTRAINT enterprise_join_requests_pkey PRIMARY KEY (id);


--
-- Name: members members_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.members
    ADD CONSTRAINT members_pkey PRIMARY KEY (organization_id, account_id);


--
-- Name: roles roles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_pkey PRIMARY KEY (id);


--
-- Name: workspaces tenant_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.workspaces
    ADD CONSTRAINT tenant_pkey PRIMARY KEY (id);


--
-- Name: department_members uk_dept_member; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.department_members
    ADD CONSTRAINT uk_dept_member UNIQUE (department_id, account_id);


--
-- Name: departments uk_dept_name_parent; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.departments
    ADD CONSTRAINT uk_dept_name_parent UNIQUE (group_id, parent_id, name);


--
-- Name: organization_invite_links uk_enterprise_invite_links_token; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organization_invite_links
    ADD CONSTRAINT uk_enterprise_invite_links_token UNIQUE (token);


--
-- Name: roles uk_roles_group_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT uk_roles_group_name UNIQUE (group_id, name);


--
-- Name: workspace_members uk_workspace_members_workspace_account; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.workspace_members
    ADD CONSTRAINT uk_workspace_members_workspace_account UNIQUE (workspace_id, account_id);


--
-- Name: account_integrates unique_account_provider; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_integrates
    ADD CONSTRAINT unique_account_provider UNIQUE (account_id, provider);


--
-- Name: account_integrates unique_provider_open_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_integrates
    ADD CONSTRAINT unique_provider_open_id UNIQUE (provider, open_id);


--
-- Name: workspace_members workspace_members_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.workspace_members
    ADD CONSTRAINT workspace_members_pkey PRIMARY KEY (id);


--
-- Name: account_email_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX account_email_idx ON public.accounts USING btree (email);


--
-- Name: idx_accounts_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_accounts_deleted_at ON public.accounts USING btree (deleted_at);


--
-- Name: end_user_session_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX end_user_session_id_idx ON public.end_users USING btree (session_id, type);


--
-- Name: end_user_tenant_session_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX end_user_tenant_session_id_idx ON public.end_users USING btree (tenant_id, session_id, type);


--
-- Name: idx_accounts_email_unique_nonempty; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_accounts_email_unique_nonempty ON public.accounts USING btree (lower((email)::text)) WHERE (TRIM(BOTH FROM COALESCE(email, ''::character varying)) <> ''::text);


--
-- Name: idx_accounts_mobile_e164_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_accounts_mobile_e164_unique ON public.accounts USING btree (mobile_e164) WHERE (TRIM(BOTH FROM COALESCE(mobile_e164, ''::character varying)) <> ''::text);


--
-- Name: idx_dept_group_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_dept_group_id ON public.departments USING btree (group_id);


--
-- Name: idx_dept_parent_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_dept_parent_id ON public.departments USING btree (parent_id);


--
-- Name: idx_dept_sort; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_dept_sort ON public.departments USING btree (group_id, parent_id, sort_order);


--
-- Name: idx_dept_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_dept_status ON public.departments USING btree (status);


--
-- Name: idx_dm_account_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_dm_account_id ON public.department_members USING btree (account_id);


--
-- Name: idx_dm_department_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_dm_department_id ON public.department_members USING btree (department_id);


--
-- Name: idx_egaj_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_egaj_status ON public.members USING btree (status);


--
-- Name: idx_eil_department_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_eil_department_id ON public.organization_invite_links USING btree (department_id);


--
-- Name: idx_eil_expires_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_eil_expires_at ON public.organization_invite_links USING btree (expires_at);


--
-- Name: idx_eil_group_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_eil_group_id ON public.organization_invite_links USING btree (group_id);


--
-- Name: idx_eil_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_eil_status ON public.organization_invite_links USING btree (status);


--
-- Name: idx_ejr_account_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ejr_account_id ON public.organization_join_requests USING btree (account_id);


--
-- Name: idx_ejr_department_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ejr_department_id ON public.organization_join_requests USING btree (department_id);


--
-- Name: idx_ejr_group_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ejr_group_id ON public.organization_join_requests USING btree (group_id);


--
-- Name: idx_ejr_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ejr_status ON public.organization_join_requests USING btree (status);


--
-- Name: idx_members_account_role; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_members_account_role ON public.members USING btree (account_id, role);


--
-- Name: idx_roles_group_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_roles_group_id ON public.roles USING btree (group_id);


--
-- Name: idx_roles_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_roles_status ON public.roles USING btree (status);


--
-- Name: idx_taj_account_current; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_taj_account_current ON public.workspace_members USING btree (account_id, current);


--
-- Name: idx_taj_account_not_current; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_taj_account_not_current ON public.workspace_members USING btree (account_id) WHERE (current = false);


--
-- Name: idx_taj_account_tenant_role; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_taj_account_tenant_role ON public.workspace_members USING btree (account_id, workspace_id, role);


--
-- Name: idx_workspace_members_account_current; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_workspace_members_account_current ON public.workspace_members USING btree (account_id, current);


--
-- Name: idx_workspace_members_account_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_workspace_members_account_id ON public.workspace_members USING btree (account_id);


--
-- Name: idx_workspace_members_role_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_workspace_members_role_id ON public.workspace_members USING btree (role_id);


--
-- Name: idx_workspace_members_workspace_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_workspace_members_workspace_id ON public.workspace_members USING btree (workspace_id);


--
-- Name: idx_workspaces_organization_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_workspaces_organization_id ON public.workspaces USING btree (organization_id);


--
-- Name: members_account_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX members_account_id_idx ON public.members USING btree (account_id);


--
-- Name: members_organization_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX members_organization_id_idx ON public.members USING btree (organization_id);


--
-- Name: members enterprise_group_account_joins_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.members
    ADD CONSTRAINT enterprise_group_account_joins_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE;


--
-- Name: members enterprise_group_account_joins_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.members
    ADD CONSTRAINT enterprise_group_account_joins_group_id_fkey FOREIGN KEY (organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;


--
-- Name: departments fk_dept_group; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.departments
    ADD CONSTRAINT fk_dept_group FOREIGN KEY (group_id) REFERENCES public.organizations(id) ON DELETE CASCADE;


--
-- Name: departments fk_dept_parent; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.departments
    ADD CONSTRAINT fk_dept_parent FOREIGN KEY (parent_id) REFERENCES public.departments(id) ON DELETE SET NULL;


--
-- Name: department_members fk_dm_account; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.department_members
    ADD CONSTRAINT fk_dm_account FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE;


--
-- Name: department_members fk_dm_department; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.department_members
    ADD CONSTRAINT fk_dm_department FOREIGN KEY (department_id) REFERENCES public.departments(id) ON DELETE CASCADE;


--
-- Name: roles fk_eg_roles_created_by; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT fk_eg_roles_created_by FOREIGN KEY (created_by) REFERENCES public.accounts(id) ON DELETE RESTRICT;


--
-- Name: roles fk_eg_roles_group; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT fk_eg_roles_group FOREIGN KEY (group_id) REFERENCES public.organizations(id) ON DELETE CASCADE;


--
-- Name: organization_invite_links fk_eil_created_by; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organization_invite_links
    ADD CONSTRAINT fk_eil_created_by FOREIGN KEY (created_by) REFERENCES public.accounts(id) ON DELETE RESTRICT;


--
-- Name: organization_invite_links fk_eil_department; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organization_invite_links
    ADD CONSTRAINT fk_eil_department FOREIGN KEY (department_id) REFERENCES public.departments(id) ON DELETE SET NULL;


--
-- Name: organization_invite_links fk_eil_group; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organization_invite_links
    ADD CONSTRAINT fk_eil_group FOREIGN KEY (group_id) REFERENCES public.organizations(id) ON DELETE CASCADE;


--
-- Name: organization_invite_links fk_eil_tenant; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organization_invite_links
    ADD CONSTRAINT fk_eil_tenant FOREIGN KEY (tenant_id) REFERENCES public.workspaces(id) ON DELETE SET NULL;


--
-- Name: organization_join_requests fk_ejr_account; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organization_join_requests
    ADD CONSTRAINT fk_ejr_account FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE;


--
-- Name: organization_join_requests fk_ejr_department; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organization_join_requests
    ADD CONSTRAINT fk_ejr_department FOREIGN KEY (department_id) REFERENCES public.departments(id) ON DELETE SET NULL;


--
-- Name: organization_join_requests fk_ejr_group; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organization_join_requests
    ADD CONSTRAINT fk_ejr_group FOREIGN KEY (group_id) REFERENCES public.organizations(id) ON DELETE CASCADE;


--
-- Name: organization_join_requests fk_ejr_invite_link; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organization_join_requests
    ADD CONSTRAINT fk_ejr_invite_link FOREIGN KEY (invite_link_id) REFERENCES public.organization_invite_links(id) ON DELETE SET NULL;


--
-- Name: organization_join_requests fk_ejr_reviewer; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organization_join_requests
    ADD CONSTRAINT fk_ejr_reviewer FOREIGN KEY (reviewer_id) REFERENCES public.accounts(id) ON DELETE SET NULL;


--
-- Name: organization_join_requests fk_ejr_tenant; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.organization_join_requests
    ADD CONSTRAINT fk_ejr_tenant FOREIGN KEY (tenant_id) REFERENCES public.workspaces(id) ON DELETE SET NULL;


--
-- PostgreSQL database dump complete
--

\unrestrict YgIGeFc1bLvKbhfNvKUGyTKeq6qTXSXWyBiYH4reFhP1IxmdgSA07AhbCJFY4mR

