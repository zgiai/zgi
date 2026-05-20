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
