package baseline

var CompatViewsSchema = File{
	Name: "70_compat_views",
	Statements: []string{
		`CREATE VIEW public.enterprise_group_account_joins AS
 SELECT organization_id AS group_id,
    account_id,
    role,
    status,
    created_at,
    updated_at
   FROM public.members;`,
		`CREATE VIEW public.enterprise_group_roles AS
 SELECT id,
    group_id,
    name,
    description,
    status,
    created_by,
    created_at,
    updated_at
   FROM public.roles;`,
		`CREATE VIEW public.enterprise_groups AS
 SELECT id,
    name,
    status,
    created_at,
    updated_at,
    short_name
   FROM public.organizations;`,
		`CREATE VIEW public.enterprise_invite_links AS
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
   FROM public.organization_invite_links;`,
		`CREATE VIEW public.enterprise_join_requests AS
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
   FROM public.organization_join_requests;`,
		`CREATE VIEW public.tenant_account_joins AS
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
   FROM public.workspace_members;`,
		`CREATE VIEW public.tenants AS
 SELECT id,
    name,
    encrypt_public_key,
    plan,
    status,
    created_at,
    updated_at,
    custom_config
   FROM public.workspaces;`,
	},
}
