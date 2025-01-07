// auth
export interface RegisterParams {
    username?: string;
    password?: string;
    email?: string;
}

export interface LoginParams {
    email?: string;
    password?: string;
}

// organization
export interface GetOrganizationParams {
    organization_id: string;
}

export interface UpdateOrganizationParams {
    name?: string;
    description?: string;
    is_active?: boolean;
}

export interface CreateOrganizationParams {
    name: string;
    description?: string;
    project?: {
        name?: string;
        description?: string;
    };
}

export interface SetOrgAdminParams {
    organization_id: string;
    user_ids: number[];
}

// project
export interface ListProjectParams {
    organization_id: string;
}

export interface CreateProjectParams {
    organization_id: string;
    name: string;
    description: string;
}

export interface UpdateProjectParams {
    name?: string;
    description?: string;
}

export interface GetProjectParams {
    project_id: string;
}

export interface GetApiKeyListParams {
    project_id: string;
    page_size?: number;
    page_num?: number;
}

export interface GetApiKeyParams {
    api_key_uuid: string;
}

export interface GetOrgPermissionParams {
    organization_id: string;
}

export interface GetOrgMembersListParams {
    organization_id: string;
    page_size?: number;
    page_num?: number;
}

export interface CreateApiKeyParams {
    name: string;
}

// admin
export interface GetUserByIdParams {
    user_id: string;
}

export interface GetUserListParams {
    page_size?: number;
    page_num?: number;
    user_type?: number;
}


