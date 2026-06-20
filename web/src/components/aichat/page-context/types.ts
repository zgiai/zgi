import type {
  AIChatCapabilityDescriptor,
  AIChatCapabilityRisk,
  AIChatContextItem,
  AIChatContextItemType,
  AIChatContextAdapterHints,
  AIChatContextMetadata,
  AIChatContextPresentationHint,
  AIChatContextRegistrationOptions,
  AIChatContextRefreshHint,
  AIChatContextRelation,
  AIChatContextRisk,
  AIChatContextToolGovernanceHint,
  AIChatOperationCapability,
  AIChatOperationContext,
  AIChatOperationResource,
  AIChatResourceType,
} from '../contextual/types';

export type {
  AIChatCapabilityDescriptor,
  AIChatCapabilityDescriptor as PageContextCapabilityDescriptor,
  AIChatCapabilityRisk,
  AIChatCapabilityRisk as PageContextCapabilityRisk,
  AIChatCapabilityStatus,
  AIChatCapabilityStatus as PageContextCapabilityStatus,
  AIChatContextItem,
  AIChatContextItem as PageContextItem,
  AIChatContextItemType,
  AIChatContextItemType as PageContextItemType,
  AIChatContextAdapterHints,
  AIChatContextAdapterHints as PageContextAdapterHints,
  AIChatContextMetadata,
  AIChatContextMetadata as PageContextMetadata,
  AIChatContextMetadataValue,
  AIChatContextMetadataValue as PageContextMetadataValue,
  AIChatContextPresentationHint,
  AIChatContextPresentationHint as PageContextPresentationHint,
  AIChatContextRegistrationOptions,
  AIChatContextRefreshHint,
  AIChatContextRefreshHint as PageContextRefreshHint,
  AIChatContextRelation,
  AIChatContextRelation as PageContextRelation,
  AIChatContextRisk,
  AIChatContextRisk as PageContextRisk,
  AIChatContextToolGovernanceHint,
  AIChatContextToolGovernanceHint as PageContextToolGovernanceHint,
  AIChatOperationCapability,
  AIChatOperationCapability as PageOperationCapability,
  AIChatOperationContext,
  AIChatOperationContext as PageOperationContext,
  AIChatOperationMetadata,
  AIChatOperationMetadata as PageOperationMetadata,
  AIChatOperationMetadataValue,
  AIChatOperationMetadataValue as PageOperationMetadataValue,
  AIChatOperationRelation,
  AIChatOperationRelation as PageOperationRelation,
  AIChatOperationResource,
  AIChatOperationResource as PageOperationResource,
  AIChatResourceStatus,
  AIChatResourceStatus as PageContextResourceStatus,
  AIChatResourceType,
  AIChatResourceType as PageContextResourceType,
  AIChatToolGovernancePermissionTier,
  AIChatToolGovernancePermissionTier as PageContextToolGovernancePermissionTier,
} from '../contextual/types';

export type AIChatPageContextItem = AIChatContextItem;
export type AIChatPageContextItemType = AIChatContextItemType;
export type AIChatPageContextRelation = AIChatContextRelation;
export type AIChatPageContextCapability = AIChatCapabilityDescriptor;
export type AIChatPageContextCapabilityRisk = AIChatCapabilityRisk;
export type AIChatPageOperationContext = AIChatOperationContext;
export type AIChatPageOperationResource = AIChatOperationResource;
export type AIChatPageOperationCapability = AIChatOperationCapability;
export type AIChatPageContextRisk = AIChatContextRisk;
export type AIChatPageContextMetadata = AIChatContextMetadata;
export type AIChatPageContextRefreshHint = AIChatContextRefreshHint;
export type AIChatPageContextAdapterHints = AIChatContextAdapterHints;
export type AIChatPageContextPresentationHint = AIChatContextPresentationHint;
export type AIChatPageContextToolGovernanceHint = AIChatContextToolGovernanceHint;

export type AIChatPageContextVisibility = 'visible' | 'selected' | 'current' | 'background';

export interface AIChatPageContextRegistrationOptions
  extends AIChatContextRegistrationOptions {
  priority?: number;
  visibility?: AIChatPageContextVisibility;
}

export interface AIChatPageContextSuggestion {
  id: string;
  label: string;
  prompt: string;
  resourceType?: AIChatResourceType;
  resourceId?: string;
  risk?: AIChatCapabilityRisk;
}

export interface AIChatPageContextAdapterSuggestions {
  suggestions?: AIChatPageContextSuggestion[];
}

export type PageContextRegistrationOptions = AIChatPageContextRegistrationOptions;
export type PageContextVisibility = AIChatPageContextVisibility;
export type PageContextSuggestion = AIChatPageContextSuggestion;
