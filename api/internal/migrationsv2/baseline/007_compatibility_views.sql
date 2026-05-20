--
-- PostgreSQL database dump
--

\restrict kGp4BeJAQFSPJIYXoDDuKxUw69Q08psGhCugq3WW5V0SJjiNSmGmcIoi9G0VGmP

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

--
-- Name: enterprise_group_account_joins; Type: VIEW; Schema: public; Owner: -
--

CREATE VIEW public.enterprise_group_account_joins AS
 SELECT organization_id AS group_id,
    account_id,
    role,
    status,
    created_at,
    updated_at
   FROM public.members;


--
-- Name: enterprise_group_roles; Type: VIEW; Schema: public; Owner: -
--

CREATE VIEW public.enterprise_group_roles AS
 SELECT id,
    group_id,
    name,
    description,
    status,
    created_by,
    created_at,
    updated_at
   FROM public.roles;


--
-- Name: enterprise_groups; Type: VIEW; Schema: public; Owner: -
--

CREATE VIEW public.enterprise_groups AS
 SELECT id,
    name,
    status,
    created_at,
    updated_at,
    short_name
   FROM public.organizations;


--
-- Name: enterprise_invite_links; Type: VIEW; Schema: public; Owner: -
--

CREATE VIEW public.enterprise_invite_links AS
 SELECT id,
    group_id,
    department_id,
    tenant_id,
    token,
    status,
    require_approval,
    default_group_role,
    default_tenant_role,
    expires_at,
    created_by,
    created_at,
    updated_at
   FROM public.organization_invite_links;


--
-- Name: enterprise_join_requests; Type: VIEW; Schema: public; Owner: -
--

CREATE VIEW public.enterprise_join_requests AS
 SELECT id,
    group_id,
    invite_link_id,
    account_id,
    department_id,
    tenant_id,
    default_group_role,
    default_tenant_role,
    status,
    reason,
    reviewer_id,
    created_at,
    reviewed_at
   FROM public.organization_join_requests;


--
-- Name: tenant_account_joins; Type: VIEW; Schema: public; Owner: -
--

CREATE VIEW public.tenant_account_joins AS
 SELECT id,
    workspace_id AS tenant_id,
    account_id,
    role,
    role_id,
    current,
    created_at,
    updated_at,
    invited_by,
    extensions,
    workspace_id
   FROM public.workspace_members;


--
-- Name: tenants; Type: VIEW; Schema: public; Owner: -
--

CREATE VIEW public.tenants AS
 SELECT id,
    name,
    encrypt_public_key,
    plan,
    status,
    created_at,
    updated_at,
    custom_config
   FROM public.workspaces;


--
-- PostgreSQL database dump complete
--

\unrestrict kGp4BeJAQFSPJIYXoDDuKxUw69Q08psGhCugq3WW5V0SJjiNSmGmcIoi9G0VGmP

