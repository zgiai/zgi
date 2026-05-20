'use client';

import React, { useMemo, useState } from 'react';
import { useT } from '@/i18n';
import {
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
} from '@/components/ui/dropdown-menu';
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip';
import { Input } from '@/components/ui/input';
import { CircleSlash2 } from 'lucide-react';
import type { UpstreamExportItem } from '../../store/store';
import type { NodesSuffix, WorkflowVariable } from '../../store/type';
import type { StructuredTypeField } from '../../types/input-var';
import { cn } from '@/lib/utils';
import {
  useWorkflowVariableCatalog,
  type WorkflowVariableCatalogGroup,
} from '../../hooks';

// Primitive type alias derived from workflow variable type
export type PrimitiveType = WorkflowVariable['type'];

export interface ValueSelectorMenuProps {
  /** Current node id where we are selecting a value for */
  nodeId: string | null | undefined;
  /** Selected value path [sourceNodeId, variableKey, subField, ...] */
  value?: string[];
  /** Change handler */
  onSelect?: (val: {
    sourceId: string;
    key: string;
    path?: string[];
    valuePath: string[];
    type: PrimitiveType;
  }) => void;
  /** Callback to close the parent menu/popover */
  onClose?: () => void;
  /** When true, only allow Start node variables */
  startOnly?: boolean;
  /** When true, only show writable variables */
  writableOnly?: boolean;
  /** Optional override for upstream groups */
  upstreamsOverride?: UpstreamExportItem[];
  /** Optional filter to restrict variable types */
  typeFilter?: (type: PrimitiveType) => boolean;
  /** Pin certain groups to the top */
  pinGroupsFirst?: (group: UpstreamExportItem) => boolean;
  /** Hide system variables (sys.*) entirely */
  hideSystem?: boolean;
}

interface NestedFieldsMenuProps {
  children: StructuredTypeField[];
  sourceId: string;
  parentPath: string[];
  onSelect: (sourceId: string, key: string, type: PrimitiveType, path: string[]) => void;
  typeFilter?: (type: PrimitiveType) => boolean;
}

/**
 * NestedFieldsMenu - Recursive component for rendering nested field structures
 */
const NestedFieldsMenu: React.FC<NestedFieldsMenuProps> = ({
  children,
  sourceId,
  parentPath,
  onSelect,
  typeFilter,
}) => {
  const t = useT('nodes');
  return (
    <div className="ml-1 pl-1 border-l border-muted-foreground/30 py-0.5 space-y-0.5">
      {children.map((field: StructuredTypeField) => {
        const fieldDesc = field.descriptionKey ? t(field.descriptionKey) : null;
        const fieldContent = (
          <span className="flex items-center gap-1 flex-1">
            <span className="truncate text-start">{field.key}</span>
            <span className="text-[10px] lowercase text-muted-foreground shrink-0 opacity-70 ml-auto">
              {field.type}
            </span>
          </span>
        );

        const currentPath = [...parentPath, field.key];
        // Array types should not expand their element children (array may be empty at runtime)
        // But non-array types should expand to allow selecting nested array fields
        const isArrayType = field.type.startsWith('array');
        const hasNestedChildren =
          !isArrayType && Array.isArray(field.children) && field.children.length > 0;
        const filteredChildren = hasNestedChildren
          ? (field.children ?? []).filter(f => {
              // Include field if it matches typeFilter OR has nested children that might match
              const selfMatches =
                typeof typeFilter === 'function' ? typeFilter(f.type as PrimitiveType) : true;
              const hasNestedMatch =
                !f.type.startsWith('array') && Array.isArray(f.children) && f.children.length > 0;
              return selfMatches || hasNestedMatch;
            })
          : [];
        const hasFilteredChildren = filteredChildren.length > 0;

        // If has nested children (non-array), render as sub-menu
        if (hasFilteredChildren) {
          // Check if the parent field itself matches the typeFilter
          const parentMatchesFilter =
            typeof typeFilter === 'function' ? typeFilter(field.type as PrimitiveType) : true;

          return (
            <DropdownMenuSub key={field.key}>
              <DropdownMenuSubTrigger
                className={cn(
                  'h-7 text-xs',
                  parentMatchesFilter ? 'cursor-pointer' : 'cursor-default'
                )}
              >
                {fieldContent}
              </DropdownMenuSubTrigger>
              <DropdownMenuSubContent className="min-w-[140px] max-h-[250px] overflow-y-auto p-1">
                {/* Only allow selecting the parent field if it matches typeFilter */}
                {parentMatchesFilter && (
                  <DropdownMenuItem
                    onClick={() =>
                      onSelect(
                        sourceId,
                        currentPath.join('.'),
                        field.type as PrimitiveType,
                        currentPath
                      )
                    }
                    className="h-7 text-xs mb-1 border-b cursor-pointer"
                  >
                    <span className="flex items-center gap-1 flex-1 italic text-muted-foreground">
                      <span className="truncate">{field.key}</span>
                      <span className="text-[10px] ml-auto">{field.type}</span>
                    </span>
                  </DropdownMenuItem>
                )}
                <NestedFieldsMenu
                  children={filteredChildren}
                  sourceId={sourceId}
                  parentPath={currentPath}
                  onSelect={onSelect}
                  typeFilter={typeFilter}
                />
              </DropdownMenuSubContent>
            </DropdownMenuSub>
          );
        }

        // Leaf field - simple menu item
        // Check if this leaf field matches the typeFilter
        const leafMatchesFilter =
          typeof typeFilter === 'function' ? typeFilter(field.type as PrimitiveType) : true;
        const onClick = leafMatchesFilter
          ? () =>
              onSelect(sourceId, currentPath.join('.'), field.type as PrimitiveType, currentPath)
          : undefined;

        return (
          <Tooltip key={field.key}>
            <TooltipTrigger asChild>
              <DropdownMenuItem
                onClick={onClick}
                className={cn(
                  'h-7 text-xs',
                  leafMatchesFilter ? 'cursor-pointer' : 'cursor-not-allowed opacity-50'
                )}
                disabled={!leafMatchesFilter}
              >
                {fieldContent}
              </DropdownMenuItem>
            </TooltipTrigger>
            <TooltipContent side="right" className="max-w-xs text-xs">
              <div className="font-medium">{field.key}</div>
              {fieldDesc && <div className="mt-1 text-muted-foreground">{fieldDesc}</div>}
            </TooltipContent>
          </Tooltip>
        );
      })}
    </div>
  );
};

/**
 * ValueSelectorMenu
 * - Reusable menu content for variable selection
 * - Contains search input and hierarchical list
 * - Intended to be used within DropdownMenuContent or similar containers
 */
export const ValueSelectorMenu: React.FC<ValueSelectorMenuProps> = ({
  nodeId,
  onSelect,
  onClose,
  startOnly = false,
  writableOnly = false,
  upstreamsOverride,
  typeFilter,
  pinGroupsFirst,
  hideSystem = false,
}) => {
  const t = useT();
  const [query, setQuery] = useState('');
  const { regularGroups, environmentGroup, conversationGroup, systemGroup } =
    useWorkflowVariableCatalog({
      nodeId,
      startOnly,
      writableOnly,
      upstreamsOverride,
      typeFilter,
      pinGroupsFirst: pinGroupsFirst
        ? group =>
            pinGroupsFirst({
              nodeId: group.sourceId,
              nodeType: group.sourceNodeType as UpstreamExportItem['nodeType'],
              nodeTitle: group.sourceNodeTitle,
              variables: [],
            })
        : undefined,
      hideSystem,
    });

  const resolveDescription = React.useCallback(
    (descriptionKey?: NodesSuffix, description?: string): string | null => {
      if (descriptionKey) {
        return t(`nodes.${descriptionKey}`, { innerType: description || 'string' });
      }
      return description || null;
    },
    [t]
  );

  const filterChildrenByQuery = React.useCallback(
    (children: StructuredTypeField[] | undefined, currentQuery: string): StructuredTypeField[] => {
      if (!children || children.length === 0) return [];

      return children
        .map(field => {
          const nextChildren = field.type.startsWith('array')
            ? field.children
            : filterChildrenByQuery(field.children, currentQuery);
          const matchesSelf = field.key.toLowerCase().includes(currentQuery);
          const matchesType =
            typeof typeFilter === 'function' ? typeFilter(field.type as PrimitiveType) : true;
          const hasNested = Array.isArray(nextChildren) && nextChildren.length > 0;

          if (!matchesSelf && !hasNested) return null;
          if (!matchesType && !hasNested) return null;

          return { ...field, children: hasNested ? nextChildren : field.children };
        })
        .filter(Boolean) as StructuredTypeField[];
    },
    [typeFilter]
  );

  const filterGroupsByQuery = React.useCallback(
    (groups: WorkflowVariableCatalogGroup[]) => {
      const nonEmpty = groups.filter(group => group.variables.length > 0);
      const q = query.trim().toLowerCase();
      if (!q) return nonEmpty;

      return nonEmpty
        .map(group => {
          const groupMatches = group.sourceTitle.toLowerCase().includes(q);
          const matchedVariables = group.variables.filter(variable => {
            if (variable.displayKey.toLowerCase().includes(q)) return true;
            return filterChildrenByQuery(variable.children, q).length > 0;
          });

          if (groupMatches) return group;
          return { ...group, variables: matchedVariables };
        })
        .filter(group => group.variables.length > 0);
    },
    [filterChildrenByQuery, query]
  );

  const menuGroups = useMemo(() => {
    return filterGroupsByQuery([
      ...(environmentGroup ? [environmentGroup] : []),
      ...(conversationGroup ? [conversationGroup] : []),
      ...regularGroups,
    ]);
  }, [conversationGroup, environmentGroup, filterGroupsByQuery, regularGroups]);

  const sysVisible = useMemo(() => {
    if (!systemGroup) return [];

    const q = query.trim().toLowerCase();
    if (!q) return systemGroup.variables;
    return systemGroup.variables.filter(variable => {
      if (variable.displayKey.toLowerCase().includes(q)) return true;
      return filterChildrenByQuery(variable.children, q).length > 0;
    });
  }, [filterChildrenByQuery, query, systemGroup]);

  const handleItemSelect = (sourceId: string, key: string, type: PrimitiveType, path: string[]) => {
    onSelect?.({
      sourceId,
      key,
      path,
      valuePath: [sourceId, ...path],
      type,
    });
    onClose?.();
  };

  return (
    <>
      <div className="px-2 py-2 sticky top-0 bg-popover z-10 border-b">
        <Input
          value={query}
          onChange={(e: React.ChangeEvent<HTMLInputElement>) => setQuery(e.target.value)}
          placeholder={t('agents.workflow.search') || 'Search...'}
          className="h-8 text-xs"
          autoFocus
        />
      </div>

      {menuGroups.map(src => {
        if (src.variables.length === 0) return null;
        return (
          <DropdownMenuGroup key={src.sourceId}>
            <DropdownMenuLabel className="text-xs text-muted-foreground">
              {src.sourceTitle}
            </DropdownMenuLabel>
            {src.variables.map(v => {
              const val = `${v.sourceId}::${v.key}`;
              const desc = resolveDescription(v.descriptionKey, v.description);
              const matchesSelf =
                typeof typeFilter === 'function' ? typeFilter(v.type as PrimitiveType) : true;

              const itemContent = (
                <span
                  className={cn('flex items-center gap-1 flex-1', !matchesSelf && 'opacity-50')}
                >
                  <span className="truncate text-start">{v.displayKey}</span>
                  <span className="text-[10px] uppercase text-muted-foreground shrink-0 ml-auto">
                    {v.type}
                  </span>
                </span>
              );

              // Check for nested children
              // Array types should not expand their element children (array may be empty at runtime)
              // But non-array types should expand to allow selecting nested array fields
              const isArrayType = v.type.startsWith('array');
              const children = isArrayType
                ? [] // Don't expand array element children
                : (v.children as StructuredTypeField[] | undefined)?.filter(f => {
                    // Include field if it matches typeFilter OR has nested children that might match
                    const selfMatches =
                      typeof typeFilter === 'function' ? typeFilter(f.type as PrimitiveType) : true;
                    const hasNestedMatch =
                      !f.type.startsWith('array') &&
                      Array.isArray(f.children) &&
                      f.children.length > 0;
                    return selfMatches || hasNestedMatch;
                  });
              const hasChildren = Array.isArray(children) && children.length > 0;
              const hasExpandable = hasChildren;

              if (hasExpandable) {
                return (
                  <DropdownMenuSub key={val}>
                    <DropdownMenuSubTrigger
                      className={cn('cursor-pointer', !matchesSelf && 'cursor-default')}
                      onClick={_e => {
                        if (matchesSelf) {
                          handleItemSelect(v.sourceId, v.key, v.type as PrimitiveType, [v.key]);
                        }
                      }}
                      onKeyDown={e => {
                        if (e.key === 'Enter') {
                          if (matchesSelf) {
                            e.preventDefault();
                            handleItemSelect(v.sourceId, v.key, v.type as PrimitiveType, [v.key]);
                          }
                        }
                      }}
                    >
                      {itemContent}
                    </DropdownMenuSubTrigger>
                    <DropdownMenuSubContent className="min-w-[160px] max-h-[300px] overflow-y-auto p-1">
                      <NestedFieldsMenu
                        children={children || []}
                        sourceId={v.sourceId}
                        parentPath={[v.key]}
                        onSelect={handleItemSelect}
                        typeFilter={typeFilter}
                      />
                    </DropdownMenuSubContent>
                  </DropdownMenuSub>
                );
              }

              // For leaf variables (no expandable children), check if it matches typeFilter
              const onClick = matchesSelf
                ? () => handleItemSelect(v.sourceId, v.key, v.type as PrimitiveType, [v.key])
                : undefined;

              return (
                <Tooltip key={val}>
                  <TooltipTrigger asChild>
                    <DropdownMenuItem
                      onClick={onClick}
                      className={cn(
                        matchesSelf ? 'cursor-pointer' : 'cursor-not-allowed opacity-50'
                      )}
                      disabled={!matchesSelf}
                    >
                      {itemContent}
                    </DropdownMenuItem>
                  </TooltipTrigger>
                  <TooltipContent side="right" className="max-w-xs text-xs">
                    <div className="font-medium">{v.displayKey}</div>
                    {desc && <div className="mt-1 text-muted-foreground">{desc}</div>}
                  </TooltipContent>
                </Tooltip>
              );
            })}
          </DropdownMenuGroup>
        );
      })}

      {sysVisible.length > 0 && (
        <DropdownMenuGroup>
          <DropdownMenuLabel className="text-xs text-muted-foreground">
            {t('agents.workflow.systemVariables.title')}
          </DropdownMenuLabel>
          {sysVisible.map(v => {
            const desc = resolveDescription(v.descriptionKey, v.description);
            const itemContent = (
              <span className="flex items-center gap-1 flex-1">
                <span className="truncate text-start">{v.displayKey}</span>
                <span className="text-[10px] uppercase text-muted-foreground shrink-0 ml-auto">
                  {v.type}
                </span>
              </span>
            );
            const onClick = () => handleItemSelect('sys', v.key, v.type, [v.key]);

            return (
              <Tooltip key={`sys::${v.key}`}>
                <TooltipTrigger asChild>
                  <DropdownMenuItem onClick={onClick}>{itemContent}</DropdownMenuItem>
                </TooltipTrigger>
                <TooltipContent side="right" className="max-w-xs text-xs">
                  <div className="font-medium">{v.displayKey}</div>
                  {desc && <div className="mt-1 text-muted-foreground">{desc}</div>}
                </TooltipContent>
              </Tooltip>
            );
          })}
        </DropdownMenuGroup>
      )}

      {menuGroups.length === 0 && sysVisible.length === 0 && (
        <div className="flex items-center justify-center gap-1 w-full py-10 text-muted-foreground">
          <CircleSlash2 size={32} />
        </div>
      )}
    </>
  );
};
