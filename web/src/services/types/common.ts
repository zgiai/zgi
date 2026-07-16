// Common type definitions

export interface PaginationParams {
  page?: number;
  limit?: number;
}

export interface SearchParams extends PaginationParams {
  search?: string;
}

export interface StatusFilter extends PaginationParams {
  status?: string;
}

export interface ApiResponseData<T> {
  data: T;
  message?: string;
  code?: string;
}

export interface SuccessResponse {
  result: 'success';
  message?: string;
}

export interface CommonErrorResponse {
  code: string;
  message: string;
  status: number;
}

export interface Permission {
  has_permission: boolean;
}

export interface BusinessError {
  businessError: {
    code: string;
    message: string;
  };
}

export interface AgentResourceImpactAgent {
  agent_id: string;
  name?: string;
  description?: string;
  icon_type?: 'text' | 'image' | string;
  icon?: string;
}

export interface AgentResourceBoundImpact {
  code: 'agent_resource_bound';
  operation: string;
  binding_type?: string;
  resource_id: string;
  agents: AgentResourceImpactAgent[];
  impact_token: string;
  expires_at: number;
}

export interface AgentBindingMutationConfirmation {
  agent_binding_action: 'unbind';
  impact_token: string;
}
