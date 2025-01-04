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
    project?: CreateProjectParams;
}

export interface ListProjectParams {
    organization_id: string;
}

export interface CreateProjectParams {
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

export interface GetOrgPermissionParams {
    organization_id: string;
}

