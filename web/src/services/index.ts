// ZGI Services - Unified API Layer
// This file exports all services and service utilities

// Service architecture exports
export { BaseService } from '@/lib/http/services';

export type { ServiceConfig, PaginationParams } from '@/lib/http/services';

// Specific service implementations
export { authenticationService, authService } from './auth.service';
export { uploadService } from './upload.service';
export { fileManageService } from './file-manage.service';
export * from './account.service';
export * from './organization.service';
export * from './workspace.service';
export * from './dataset.service';
export * from './dataset-folder.service';
export * from './plugin.service';
export * from './agent.service';
export * from './workflow.service';
export * from './workflow-test.service';
export * from './tool.service';
export * from './db.service';
export * from './webapp.service';
export * from './aichat.service';
export * from './image-runtime.service';
export * from './setup.service';
export * from './statistics.service';
export * from './automation.service';
export * from './content-parse.service';
export * from './prompt.service';
export * from './shortlink.service';
// LLM management services
export * from './provider.service';
export * from './model.service';
export * from './channel.service';
// API Key service
export * from './apikey.service';
export * from './dashboard.service';
// Workspace Quota service
export * from './workspace-quota.service';

// Service types
export type * from './types/auth';
export type * from './types/file';

// Common types (avoiding conflicts)
export type {
  PaginationParams as CommonPaginationParams,
  StatusFilter,
  ApiResponseData,
  SuccessResponse,
  Permission,
  BusinessError,
} from './types/common';

export type * from './types/dataset';
export type * from './types/dataset-folder';
export type * from './types/organization';
export type * from './types/plugin';
export type * from './types/agent';
export type * from './types/workflow-test';
export type * from './types/db';
export type * from './types/webapp';
export type * from './types/aichat';
export type * from './types/image-runtime';
export type * from './types/setup';
export type * from './types/statistics';
export type * from './types/automation';
// Types of LLM management
export type * from './types/provider';
export type * from './types/model';
export type * from './types/channel';
// API Key types
export type * from './types/apikey';
export type * from './types/dashboard';
// Workspace Quota types
export type * from './types/workspace-quota';

export type * from './types/tool';
export type * from './types/content-parse';
export type * from './types/prompt';

// Legacy exports for backward compatibility
export { default as authServiceDefault } from './auth.service';
export { default as uploadServiceDefault } from './upload.service';
export { default as fileServiceDefault } from './file.service';

// HTTP clients for direct usage (use sparingly)
export { http, webappHttp } from '@/lib/http';
