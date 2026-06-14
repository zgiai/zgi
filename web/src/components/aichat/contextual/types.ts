'use client';

export type AIChatContextItemType =
  | 'agent'
  | 'workflow'
  | 'file'
  | 'task'
  | 'dataset'
  | 'database'
  | 'page'
  | 'selection'
  | 'log'
  | 'custom';

export type AIChatResourceType = AIChatContextItemType;
export type AIChatCapabilityRisk = 'low' | 'medium' | 'high';
export type AIChatContextRisk = AIChatCapabilityRisk;
export type AIChatResourceStatus =
  | 'available'
  | 'draft'
  | 'published'
  | 'dirty'
  | 'stale'
  | 'readonly'
  | 'disabled'
  | 'error'
  | (string & {});
export type AIChatCapabilityStatus =
  | 'available'
  | 'unavailable'
  | 'disabled'
  | 'preview'
  | (string & {});

export type AIChatContextMetadataValue = string | number | boolean | null | undefined;
export type AIChatContextMetadata = Record<string, AIChatContextMetadataValue>;

export interface AIChatContextRelation {
  type: string;
  resourceType: AIChatResourceType;
  resourceId: string;
  title?: string;
  metadata?: AIChatContextMetadata;
}

export interface AIChatCapabilityDescriptor {
  id: string;
  title?: string;
  description?: string;
  risk: AIChatCapabilityRisk;
  requiresConfirmation?: boolean;
  status?: AIChatCapabilityStatus;
  permissions?: string[];
  metadata?: AIChatContextMetadata;
}

export interface AIChatContextItem {
  id: string;
  type: AIChatContextItemType;
  title: string;
  subtitle?: string;
  description?: string;
  href?: string;
  source?: string;
  risk?: AIChatContextRisk;
  permissions?: string[];
  status?: AIChatResourceStatus;
  relations?: AIChatContextRelation[];
  capabilities?: AIChatCapabilityDescriptor[];
  metadata?: AIChatContextMetadata;
}

export interface AIChatContextRegistrationOptions {
  scopeId?: string;
  replace?: boolean;
}

export interface AIChatOperationRelation {
  relation_type: string;
  resource_type: AIChatResourceType;
  resource_id: string;
  title?: string;
  metadata?: Record<string, string | number | boolean | null>;
}

export interface AIChatOperationResource {
  resource_id: string;
  resource_type: AIChatResourceType;
  title: string;
  subtitle?: string;
  href?: string;
  source?: string;
  status?: AIChatResourceStatus;
  risk?: AIChatCapabilityRisk;
  permissions?: string[];
  metadata?: Record<string, string | number | boolean | null>;
  relations?: AIChatOperationRelation[];
  capability_ids?: string[];
}

export interface AIChatOperationCapability {
  id: string;
  title?: string;
  description?: string;
  resource_id: string;
  resource_type: AIChatResourceType;
  risk: AIChatCapabilityRisk;
  requires_confirmation?: boolean;
  status?: AIChatCapabilityStatus;
  permissions?: string[];
  metadata?: Record<string, string | number | boolean | null>;
}

export interface AIChatOperationContext {
  schema: 'zgi.aichat.operation_context.v1';
  version: 1;
  resources: AIChatOperationResource[];
  capabilities: AIChatOperationCapability[];
  risk_summary: {
    level?: AIChatCapabilityRisk;
    requires_confirmation: boolean;
  };
  summary: {
    resource_count: number;
    capability_count: number;
    highest_risk?: AIChatCapabilityRisk;
    omitted_resource_count: number;
    omitted_capability_count: number;
  };
}
