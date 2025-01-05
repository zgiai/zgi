export interface RegisterParams {
    username?: string;
    password?: string;
    email?: string;
}

export interface LoginParams {
    email?: string;
    password?: string;
}

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

export interface CreateApiKeyParams {
    name: string;
}


