package baseline

var RuntimeSystemSchema = File{
	Name: "60_runtime_system",
	Statements: []string{
		`CREATE TABLE public.batch_hit_testing_tasks (
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
);`,
		`CREATE TABLE public.seed_executions (
    name character varying(100) NOT NULL,
    version character varying(50) NOT NULL,
    executed_at timestamp with time zone DEFAULT now() NOT NULL,
    executed_by character varying(50) DEFAULT 'manual'::character varying NOT NULL,
    status character varying(20) DEFAULT 'success'::character varying NOT NULL
);`,
	},
}
