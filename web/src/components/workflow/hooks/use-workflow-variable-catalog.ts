'use client';

import { useCallback, useMemo } from 'react';
import { useT, type NodesSuffix } from '@/i18n';
import { useWorkflowStore } from '../store';
import type { UpstreamExportItem } from '../store';
import type { StructuredTypeField } from '../types/input-var';
import {
  buildVariableSelectionKey,
  hasMatchingStructuredField,
  isSpecialVariableSource,
  type WorkflowPrimitiveType,
} from '../common/variable-reference';

export interface WorkflowVariableCatalogOptions {
  nodeId: string | null | undefined;
  startOnly?: boolean;
  writableOnly?: boolean;
  upstreamsOverride?: UpstreamExportItem[];
  typeFilter?: (type: WorkflowPrimitiveType) => boolean;
  pinGroupsFirst?: (group: WorkflowVariableCatalogGroup) => boolean;
  hideSystem?: boolean;
}

export interface WorkflowVariableCatalogVariable {
  sourceId: string;
  sourceTitle: string;
  sourceNodeType: string;
  sourceNodeTitle?: string;
  key: string;
  displayKey: string;
  type: WorkflowPrimitiveType;
  writable?: boolean;
  description?: string;
  descriptionKey?: NodesSuffix;
  children?: StructuredTypeField[];
  valuePath: string[];
  selectionKey: string;
  isSpecialSource: boolean;
}

export interface WorkflowVariableCatalogSelection extends WorkflowVariableCatalogVariable {
  keyPath: string[];
  displayPath: string;
  displayText: string;
}

export interface WorkflowVariableCatalogGroup {
  sourceId: string;
  sourceTitle: string;
  sourceNodeType: string;
  sourceNodeTitle?: string;
  isSpecialSource: boolean;
  variables: WorkflowVariableCatalogVariable[];
}

export interface WorkflowVariableCatalogResult {
  systemGroup: WorkflowVariableCatalogGroup | null;
  environmentGroup: WorkflowVariableCatalogGroup | null;
  conversationGroup: WorkflowVariableCatalogGroup | null;
  regularGroups: WorkflowVariableCatalogGroup[];
  allGroups: WorkflowVariableCatalogGroup[];
  topLevelOptions: WorkflowVariableCatalogSelection[];
  selectionIndex: Map<string, WorkflowVariableCatalogSelection>;
  totalSelectableCount: number;
}

function matchesVariableType(
  variable: {
    type: WorkflowPrimitiveType;
    children?: StructuredTypeField[];
  },
  typeFilter?: (type: WorkflowPrimitiveType) => boolean
) {
  if (!typeFilter) return true;
  if (typeFilter(variable.type)) return true;

  return hasMatchingStructuredField(
    variable.children,
    field => typeFilter(field.type as WorkflowPrimitiveType)
  );
}

function buildSelectionEntry(args: {
  sourceId: string;
  sourceTitle: string;
  sourceNodeType: string;
  sourceNodeTitle?: string;
  isSpecialSource: boolean;
  key: string;
  type: WorkflowPrimitiveType;
  writable?: boolean;
  description?: string;
  descriptionKey?: NodesSuffix;
  children?: StructuredTypeField[];
  valuePath: string[];
}): WorkflowVariableCatalogSelection {
  const displayPath = args.valuePath.slice(1).join('.') || args.key;

  return {
    sourceId: args.sourceId,
    sourceTitle: args.sourceTitle,
    sourceNodeType: args.sourceNodeType,
    sourceNodeTitle: args.sourceNodeTitle,
    key: args.key,
    displayKey: args.valuePath[args.valuePath.length - 1] || args.key,
    type: args.type,
    writable: args.writable,
    description: args.description,
    descriptionKey: args.descriptionKey,
    children: args.children,
    valuePath: args.valuePath,
    selectionKey: buildVariableSelectionKey(args.valuePath) || args.valuePath.join('::'),
    keyPath: args.valuePath.slice(1),
    displayPath,
    displayText: `${args.sourceTitle} (${displayPath})`,
    isSpecialSource: args.isSpecialSource,
  };
}

export function useWorkflowVariableCatalog({
  nodeId,
  startOnly = false,
  writableOnly = false,
  upstreamsOverride,
  typeFilter,
  pinGroupsFirst,
  hideSystem = false,
}: WorkflowVariableCatalogOptions): WorkflowVariableCatalogResult {
  const t = useT();
  const getUpstreamVariables = useWorkflowStore.use.getUpstreamVariables();
  const getUpstreamWritableVariables = useWorkflowStore.use.getUpstreamWritableVariables?.();
  const graphVersion = useWorkflowStore(state => state.graphVersion);

  const upstreamsRaw = useMemo<UpstreamExportItem[]>(() => {
    if (Array.isArray(upstreamsOverride)) return upstreamsOverride;
    if (!nodeId) return [];

    if (writableOnly && typeof getUpstreamWritableVariables === 'function') {
      return getUpstreamWritableVariables(nodeId) || [];
    }

    return getUpstreamVariables(nodeId) || [];
  }, [
    getUpstreamVariables,
    getUpstreamWritableVariables,
    graphVersion,
    nodeId,
    upstreamsOverride,
    writableOnly,
  ]);

  const upstreams = useMemo<UpstreamExportItem[]>(() => {
    return startOnly ? upstreamsRaw.filter(group => group.nodeType === 'start') : upstreamsRaw;
  }, [startOnly, upstreamsRaw]);

  const getSourceTitle = useCallback(
    (group: UpstreamExportItem) => {
      if (group.nodeId === 'sys') return t('agents.workflow.systemVariables.title');
      if (group.nodeId === 'environment') return t('agents.workflow.environmentVariables.title');
      if (group.nodeId === 'conversation') return t('agents.workflow.conversationVariables.title');
      return group.nodeTitle ?? group.nodeId;
    },
    [t]
  );

  const catalog = useMemo<WorkflowVariableCatalogResult>(() => {
    let systemGroup: WorkflowVariableCatalogGroup | null = hideSystem
      ? null
      : {
          sourceId: 'sys',
          sourceTitle: t('agents.workflow.systemVariables.title'),
          sourceNodeType: 'start',
          isSpecialSource: true,
          variables: [],
        };
    let environmentGroup: WorkflowVariableCatalogGroup | null = null;
    let conversationGroup: WorkflowVariableCatalogGroup | null = null;
    const regularGroups: WorkflowVariableCatalogGroup[] = [];
    const selectionIndex = new Map<string, WorkflowVariableCatalogSelection>();

    const registerSelection = (entry: WorkflowVariableCatalogSelection) => {
      selectionIndex.set(entry.selectionKey, entry);

      const children = entry.type.startsWith('array') ? [] : entry.children;
      if (!Array.isArray(children) || children.length === 0) return;

      const walkChildren = (fields: StructuredTypeField[], parentPath: string[]) => {
        fields.forEach(field => {
          const valuePath = [entry.sourceId, ...parentPath, field.key];
          const nestedEntry = buildSelectionEntry({
            sourceId: entry.sourceId,
            sourceTitle: entry.sourceTitle,
            sourceNodeType: entry.sourceNodeType,
            sourceNodeTitle: entry.sourceNodeTitle,
            isSpecialSource: entry.isSpecialSource,
            key: entry.key,
            type: field.type as WorkflowPrimitiveType,
            writable: entry.writable,
            description: undefined,
            descriptionKey: field.descriptionKey as NodesSuffix | undefined,
            children: field.children,
            valuePath,
          });
          selectionIndex.set(nestedEntry.selectionKey, nestedEntry);

          if (!field.type.startsWith('array') && Array.isArray(field.children) && field.children.length) {
            walkChildren(field.children, [...parentPath, field.key]);
          }
        });
      };

      walkChildren(children, [entry.key]);
    };

    upstreams.forEach(group => {
      const sourceTitle = getSourceTitle(group);
      const isSpecialGroup = isSpecialVariableSource(group.nodeId);
      const nextGroup: WorkflowVariableCatalogGroup = {
        sourceId: group.nodeId,
        sourceTitle,
        sourceNodeType: group.nodeType,
        sourceNodeTitle: group.nodeTitle,
        isSpecialSource: isSpecialGroup,
        variables: [],
      };

      (group.variables || []).forEach(variable => {
        const sourceId =
          typeof variable.key === 'string' && variable.key.startsWith('sys.') ? 'sys' : group.nodeId;
        const key =
          sourceId === 'sys' && typeof variable.key === 'string'
            ? variable.key.slice(4)
            : String(variable.key);
        const catalogVariable: WorkflowVariableCatalogVariable = {
          sourceId,
          sourceTitle:
            sourceId === 'sys' ? t('agents.workflow.systemVariables.title') : sourceTitle,
          sourceNodeType: sourceId === 'sys' ? 'start' : group.nodeType,
          sourceNodeTitle: sourceId === 'sys' ? undefined : group.nodeTitle,
          key,
          displayKey: key,
          type: variable.type as WorkflowPrimitiveType,
          writable: variable.writable,
          description: variable.description,
          descriptionKey: variable.descriptionKey as NodesSuffix | undefined,
          children: variable.children,
          valuePath: [sourceId, key],
          selectionKey: buildVariableSelectionKey([sourceId, key]) || `${sourceId}::${key}`,
          isSpecialSource: isSpecialVariableSource(sourceId),
        };

        if (!matchesVariableType(catalogVariable, typeFilter)) {
          return;
        }

        registerSelection(
          buildSelectionEntry({
            sourceId: catalogVariable.sourceId,
            sourceTitle: catalogVariable.sourceTitle,
            sourceNodeType: catalogVariable.sourceNodeType,
            sourceNodeTitle: catalogVariable.sourceNodeTitle,
            isSpecialSource: catalogVariable.isSpecialSource,
            key: catalogVariable.key,
            type: catalogVariable.type,
            writable: catalogVariable.writable,
            description: catalogVariable.description,
            descriptionKey: catalogVariable.descriptionKey,
            children: catalogVariable.children,
            valuePath: catalogVariable.valuePath,
          })
        );

        if (catalogVariable.sourceId === 'sys') {
          if (!systemGroup) return;
          systemGroup.variables.push(catalogVariable);
          return;
        }

        nextGroup.variables.push(catalogVariable);
      });

      if (nextGroup.variables.length === 0) return;

      if (group.nodeId === 'environment') {
        environmentGroup = nextGroup;
        return;
      }

      if (group.nodeId === 'conversation') {
        conversationGroup = nextGroup;
        return;
      }

      regularGroups.push(nextGroup);
    });

    const orderedRegularGroups = pinGroupsFirst
      ? [
          ...regularGroups.filter(pinGroupsFirst),
          ...regularGroups.filter(group => !pinGroupsFirst(group)),
        ]
      : regularGroups;

    const allGroups = [
      ...(environmentGroup ? [environmentGroup] : []),
      ...(conversationGroup ? [conversationGroup] : []),
      ...orderedRegularGroups,
      ...(systemGroup && systemGroup.variables.length > 0 ? [systemGroup] : []),
    ];

    const topLevelOptions: WorkflowVariableCatalogSelection[] = allGroups.flatMap(group =>
      group.variables.map(variable => ({
        ...variable,
        keyPath: [variable.key],
        displayPath: variable.key,
        displayText: `${variable.sourceTitle} (${variable.key})`,
      }))
    );

    return {
      systemGroup,
      environmentGroup,
      conversationGroup,
      regularGroups: orderedRegularGroups,
      allGroups,
      topLevelOptions,
      selectionIndex,
      totalSelectableCount: topLevelOptions.length,
    };
  }, [getSourceTitle, hideSystem, pinGroupsFirst, t, typeFilter, upstreams]);

  return catalog;
}
