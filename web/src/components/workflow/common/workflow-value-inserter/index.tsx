'use client';

import React, { useState, useEffect, useMemo, useCallback, useId } from 'react';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { WorkflowVariable } from '@/components/workflow/store/type';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Variable } from 'lucide-react';
import NodeSelector from './node-selector';
import VariableItem, { type VariableInsertValue } from './variable-item';
import {
  useWorkflowVariableCatalog,
  type WorkflowVariableCatalogVariable,
} from '../../hooks';

export interface WorkflowValueInserterProps {
  /** Current node id to load upstream variables */
  nodeId: string | null | undefined;
  /** Callback when user selects a variable to insert */
  onInsert?: (value: VariableInsertValue) => void;
  /** Additional CSS classes for the container */
  className?: string;
  /** When true, only allow Start node variables */
  startOnly?: boolean;
  /** When true, only show writable variables */
  writableOnly?: boolean;
  /** Optional filter to restrict variable types */
  typeFilter?: (type: WorkflowVariable['type']) => boolean;
  /** Maximum width for node tabs before showing "More" dropdown. If undefined, auto-fit to container width. */
  maxTabsWidth?: number;
  /** Disable insertion actions while keeping current values viewable. */
  disabled?: boolean;
  /** Optional variables appended before upstream variables. */
  extraVariables?: Array<{
    sourceId: string;
    key: string;
    label: string;
    type: WorkflowVariable['type'];
    sourceTitle?: string;
  }>;
  /** Title for extra variables. */
  extraGroupTitle?: string;
  /** Whether the variable panel should start collapsed. */
  defaultCollapsed?: boolean;
}

// Internal item and tab components moved to sibling files for maintainability.

interface InserterEmptyStateProps {
  title: string;
  description?: string;
  className?: string;
}

const InserterEmptyState: React.FC<InserterEmptyStateProps> = ({
  title,
  description,
  className,
}) => {
  return (
    <div
      className={cn(
        'flex min-h-28 flex-col items-center justify-center rounded-lg border border-dashed bg-background/80 px-4 py-6 text-center',
        className
      )}
    >
      <div className="mb-3 flex size-10 items-center justify-center rounded-full bg-muted shadow-sm">
        <Variable className="size-4 text-muted-foreground" />
      </div>
      <div className="text-sm font-medium text-foreground">{title}</div>
      {description ? (
        <div className="mt-1 max-w-[320px] text-xs leading-5 text-muted-foreground">
          {description}
        </div>
      ) : null}
    </div>
  );
};

/**
 * WorkflowValueInserter - A bookmark-like component for quickly inserting workflow variables
 *
 * Features:
 * - Bookmark-style node tabs that adapt to content width
 * - Overflow handling with "More" dropdown
 * - Variable selection with type indicators
 * - Insert event callback with complete variable information
 */
const WorkflowValueInserter: React.FC<WorkflowValueInserterProps> = ({
  nodeId,
  onInsert,
  className,
  startOnly = false,
  writableOnly = false,
  typeFilter,
  maxTabsWidth,
  disabled = false,
  extraVariables,
  extraGroupTitle,
  defaultCollapsed = false,
}) => {
  const t = useT();
  const [activeNodeId, setActiveNodeId] = useState<string | null>(null);

  // Collapsible content id
  const contentId = useId();
  const [collapsed, setCollapsed] = useState(defaultCollapsed);

  const { regularGroups, systemGroup, environmentGroup, conversationGroup } =
    useWorkflowVariableCatalog({
      nodeId,
      startOnly,
      writableOnly,
      typeFilter,
    });

  const upstreamNodes = regularGroups;
  const systemVariables: WorkflowVariableCatalogVariable[] = systemGroup?.variables ?? [];
  const envVariables: WorkflowVariableCatalogVariable[] = environmentGroup?.variables ?? [];
  const convVariables: WorkflowVariableCatalogVariable[] = conversationGroup?.variables ?? [];
  const appendedVariables = extraVariables ?? [];

  // Set first node as active by default
  useEffect(() => {
    if (upstreamNodes.length > 0 && !activeNodeId) {
      setActiveNodeId(upstreamNodes[0].sourceId);
    }
  }, [upstreamNodes, activeNodeId]);

  // Get active node data
  const activeNode = useMemo(() => {
    return upstreamNodes.find(node => node.sourceId === activeNodeId);
  }, [upstreamNodes, activeNodeId]);

  // Handle variable selection
  const handleVariableSelect = useCallback(
    (value: VariableInsertValue) => {
      if (disabled) return;
      onInsert?.(value);
    },
    [disabled, onInsert]
  );

  // Handle node tab click
  const handleNodeClick = useCallback(
    (nextNodeId: string) => {
      if (disabled) return;
      setActiveNodeId(nextNodeId);
    },
    [disabled]
  );

  if (
    upstreamNodes.length === 0 &&
    systemVariables.length === 0 &&
    envVariables.length === 0 &&
    convVariables.length === 0 &&
    appendedVariables.length === 0
  ) {
    return (
      <div className={cn('space-y-2', className)}>
        {/* Header */}
        <div className="flex items-center justify-between px-1">
          <div className="text-sm font-medium">
            {t('nodes.valueInserter.headers.quickVariables')}
          </div>
        </div>
        <InserterEmptyState
          title={t('nodes.valueInserter.empty.noUpstream')}
          description={t('nodes.valueInserter.empty.noUpstreamDescription')}
        />
      </div>
    );
  }

  return (
    <div className={cn('space-y-2', disabled && 'opacity-60')} aria-disabled={disabled}>
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="font-medium text-sm">{t('nodes.valueInserter.headers.quickVariables')}</div>
        {(systemVariables.length > 0 || envVariables.length > 0 || convVariables.length > 0) && (
          <Button
            variant="link"
            size="sm"
            className="text-highlight text-sm p-0 h-auto hover:underline"
            onClick={() => setCollapsed(prev => !prev)}
            disabled={disabled}
            aria-label={t('nodes.valueInserter.aria.quickHeaderToggle')}
            aria-expanded={!collapsed}
            aria-controls={contentId}
          >
            {collapsed
              ? t('nodes.valueInserter.toggle.expand')
              : t('nodes.valueInserter.toggle.collapse')}
          </Button>
        )}
      </div>

      <div
        id={contentId}
        className={cn('rounded-lg bg-muted/50 space-y-2 p-3', collapsed && 'hidden', className)}
      >
        {appendedVariables.length > 0 && (
          <div>
            <div className="text-sm font-medium mb-2">
              {extraGroupTitle || t('nodes.valueInserter.headers.quickVariables')}
            </div>
            <div className="flex flex-wrap gap-2">
              {appendedVariables.map(variable => (
                <Badge
                  key={`${variable.sourceId}-${variable.key || variable.label}`}
                  role="button"
                  tabIndex={0}
                  onClick={() =>
                    handleVariableSelect({
                      sourceId: variable.sourceId,
                      key: variable.key,
                      type: variable.type,
                      sourceTitle: variable.sourceTitle,
                      path: variable.key ? [variable.key] : [],
                    })
                  }
                  onKeyDown={(event: React.KeyboardEvent<HTMLSpanElement>) => {
                    if (event.key === 'Enter' || event.key === ' ') {
                      event.preventDefault();
                      handleVariableSelect({
                        sourceId: variable.sourceId,
                        key: variable.key,
                        type: variable.type,
                        sourceTitle: variable.sourceTitle,
                        path: variable.key ? [variable.key] : [],
                      });
                    }
                  }}
                  className="cursor-pointer select-none whitespace-nowrap rounded-md px-2 py-1 text-xs"
                >
                  {variable.label}
                </Badge>
              ))}
            </div>
          </div>
        )}

        {/* System Variables Section */}
        {systemVariables.length > 0 && (
          <div>
            <div className="text-sm font-medium mb-2">
              {t('agents.workflow.systemVariables.title')}
            </div>
            <div className="flex flex-wrap gap-2">
              {systemVariables.map(variable => (
                <VariableItem
                  key={variable.selectionKey}
                  variable={{
                    key: variable.displayKey,
                    type: variable.type,
                    description: variable.description,
                    descriptionKey: variable.descriptionKey,
                    children: variable.children,
                  }}
                  sourceId="sys"
                  sourceTitle={t('agents.workflow.systemVariables.title')}
                  onSelect={value => {
                    // Keep key without 'sys.'; caller can assemble token with sourceId
                    handleVariableSelect({ ...value });
                  }}
                />
              ))}
            </div>
          </div>
        )}

        {/* Environment Variables Section */}
        {envVariables.length > 0 && (
          <div>
            <div className="text-sm font-medium mb-2">
              {t('agents.workflow.environmentVariables.title')}
            </div>
            <div className="flex flex-wrap gap-2">
              {envVariables.map(variable => (
                <VariableItem
                  key={variable.selectionKey}
                  variable={{
                    key: variable.displayKey,
                    type: variable.type,
                    description: variable.description,
                    children: variable.children,
                  }}
                  sourceId="environment"
                  sourceTitle={t('agents.workflow.environmentVariables.title')}
                  onSelect={handleVariableSelect}
                />
              ))}
            </div>
          </div>
        )}

        {/* Conversation Variables Section */}
        {convVariables.length > 0 && (
          <div>
            <div className="text-sm font-medium mb-2">
              {t('agents.workflow.conversationVariables.title')}
            </div>
            <div className="flex flex-wrap gap-2">
              {convVariables.map(variable => (
                <VariableItem
                  key={variable.selectionKey}
                  variable={{
                    key: variable.displayKey,
                    type: variable.type,
                    description: variable.description,
                    children: variable.children,
                  }}
                  sourceId="conversation"
                  sourceTitle={t('agents.workflow.conversationVariables.title')}
                  onSelect={handleVariableSelect}
                />
              ))}
            </div>
          </div>
        )}

        {/* Node tabs - bookmark style (exclude environment & conversation groups) */}
        {upstreamNodes.length > 0 && (
          <div>
            <div className="text-sm font-medium mb-2">
              {t('nodes.valueInserter.headers.nodeVariables')}
            </div>
            <NodeSelector
              upstreamNodes={upstreamNodes}
              activeNodeId={activeNodeId}
              onSelect={handleNodeClick}
              maxTabsWidth={maxTabsWidth}
            />

            {/* Variables list (only show if there are regular nodes) */}
            {upstreamNodes.length > 0 && (
              <div className="pt-2">
                {activeNode ? (
                  <div className="space-y-1">
                    {activeNode.variables.length > 0 ? (
                      <div className="flex flex-wrap gap-2">
                        {activeNode.variables.map(variable => (
                          <VariableItem
                            key={variable.selectionKey}
                            variable={variable}
                            sourceId={activeNode.sourceId}
                            sourceTitle={activeNode.sourceTitle}
                            onSelect={handleVariableSelect}
                          />
                        ))}
                      </div>
                    ) : (
                      <InserterEmptyState
                        title={t('nodes.valueInserter.empty.noVariablesInNode')}
                        description={t('nodes.valueInserter.empty.noVariablesInNodeDescription')}
                        className="min-h-24 py-5"
                      />
                    )}
                  </div>
                ) : (
                  <InserterEmptyState
                    title={t('nodes.valueInserter.tips.selectNode')}
                    description={t('nodes.valueInserter.tips.selectNodeDescription')}
                    className="min-h-24 py-5"
                  />
                )}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
};

export default React.memo(WorkflowValueInserter);
