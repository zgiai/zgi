--
-- PostgreSQL database dump
--

\restrict lRrBeBqEoi3LjPN09OjYCGuXXqxw2j0hOO0PC2aFoLbALWkTVGZY6TpZr7lal6M

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
-- Name: automation_action_runs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.automation_action_runs (
    id uuid NOT NULL,
    task_run_id uuid NOT NULL,
    task_action_id uuid NOT NULL,
    action_type character varying(32) NOT NULL,
    channel_type character varying(32),
    request_payload jsonb,
    response_payload jsonb,
    error_message text,
    status character varying(32) NOT NULL,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: automation_task_actions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.automation_task_actions (
    id uuid NOT NULL,
    task_id uuid NOT NULL,
    action_type character varying(32) NOT NULL,
    action_order integer DEFAULT 1 NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    config jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: automation_task_runs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.automation_task_runs (
    id uuid NOT NULL,
    task_id uuid NOT NULL,
    trigger_source character varying(32) NOT NULL,
    scheduled_for timestamp with time zone NOT NULL,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    status character varying(32) NOT NULL,
    runtime_context jsonb,
    error_summary text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: automation_tasks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.automation_tasks (
    id uuid NOT NULL,
    organization_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    name character varying(255) NOT NULL,
    description text,
    status character varying(32) NOT NULL,
    trigger_type character varying(32) DEFAULT 'schedule'::character varying NOT NULL,
    schedule_type character varying(32) NOT NULL,
    timezone character varying(64) NOT NULL,
    schedule_config jsonb NOT NULL,
    next_run_at timestamp with time zone,
    last_run_at timestamp with time zone,
    last_run_status character varying(32),
    source_type character varying(32) NOT NULL,
    source_ref character varying(255),
    source_snapshot jsonb,
    created_by uuid NOT NULL,
    updated_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: automation_action_runs automation_action_runs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.automation_action_runs
    ADD CONSTRAINT automation_action_runs_pkey PRIMARY KEY (id);


--
-- Name: automation_task_actions automation_task_actions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.automation_task_actions
    ADD CONSTRAINT automation_task_actions_pkey PRIMARY KEY (id);


--
-- Name: automation_task_runs automation_task_runs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.automation_task_runs
    ADD CONSTRAINT automation_task_runs_pkey PRIMARY KEY (id);


--
-- Name: automation_tasks automation_tasks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.automation_tasks
    ADD CONSTRAINT automation_tasks_pkey PRIMARY KEY (id);


--
-- Name: idx_automation_action_runs_task_action_id_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_automation_action_runs_task_action_id_created_at ON public.automation_action_runs USING btree (task_action_id, created_at DESC);


--
-- Name: idx_automation_action_runs_task_run_id_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_automation_action_runs_task_run_id_created_at ON public.automation_action_runs USING btree (task_run_id, created_at);


--
-- Name: idx_automation_task_actions_task_id_action_order; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_automation_task_actions_task_id_action_order ON public.automation_task_actions USING btree (task_id, action_order);


--
-- Name: idx_automation_task_runs_task_id_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_automation_task_runs_task_id_created_at ON public.automation_task_runs USING btree (task_id, created_at DESC);


--
-- Name: idx_automation_task_runs_task_id_scheduled_for; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_automation_task_runs_task_id_scheduled_for ON public.automation_task_runs USING btree (task_id, scheduled_for);


--
-- Name: idx_automation_tasks_scope_next_run_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_automation_tasks_scope_next_run_at ON public.automation_tasks USING btree (organization_id, workspace_id, next_run_at);


--
-- Name: idx_automation_tasks_scope_status_updated_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_automation_tasks_scope_status_updated_at ON public.automation_tasks USING btree (organization_id, workspace_id, status, updated_at DESC);


--
-- Name: idx_automation_tasks_status_next_run_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_automation_tasks_status_next_run_at ON public.automation_tasks USING btree (status, next_run_at);


--
-- Name: uq_automation_task_runs_task_id_scheduled_for_trigger_source; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_automation_task_runs_task_id_scheduled_for_trigger_source ON public.automation_task_runs USING btree (task_id, scheduled_for, trigger_source);


--
-- Name: automation_action_runs automation_action_runs_task_action_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.automation_action_runs
    ADD CONSTRAINT automation_action_runs_task_action_id_fkey FOREIGN KEY (task_action_id) REFERENCES public.automation_task_actions(id) ON DELETE CASCADE;


--
-- Name: automation_action_runs automation_action_runs_task_run_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.automation_action_runs
    ADD CONSTRAINT automation_action_runs_task_run_id_fkey FOREIGN KEY (task_run_id) REFERENCES public.automation_task_runs(id) ON DELETE CASCADE;


--
-- Name: automation_task_actions automation_task_actions_task_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.automation_task_actions
    ADD CONSTRAINT automation_task_actions_task_id_fkey FOREIGN KEY (task_id) REFERENCES public.automation_tasks(id) ON DELETE CASCADE;


--
-- Name: automation_task_runs automation_task_runs_task_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.automation_task_runs
    ADD CONSTRAINT automation_task_runs_task_id_fkey FOREIGN KEY (task_id) REFERENCES public.automation_tasks(id) ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--

\unrestrict lRrBeBqEoi3LjPN09OjYCGuXXqxw2j0hOO0PC2aFoLbALWkTVGZY6TpZr7lal6M

