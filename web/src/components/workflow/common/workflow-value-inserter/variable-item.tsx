import React, { useCallback, useState } from 'react';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { NodesSuffix, WorkflowVariable } from '@/components/workflow/store/type';
import type { StructuredTypeField } from '@/components/workflow/types/input-var';
import { Badge } from '@/components/ui/badge';
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip';
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from '@/components/ui/dropdown-menu';
import { ChevronRight } from 'lucide-react';

export interface VariableInsertValue {
  sourceId: string;
  key: string;
  type: WorkflowVariable['type'];
  sourceTitle?: string;
  /** Full path for nested fields, e.g., ['result', 'user', 'name'] */
  path?: string[];
}

export interface VariableItemProps {
  variable: {
    key: string;
    type: WorkflowVariable['type'];
    writable?: boolean;
    description?: string;
    descriptionKey?: NodesSuffix;
    /** Nested children for json-parser outputs */
    children?: StructuredTypeField[];
  };
  sourceId: string;
  sourceTitle?: string;
  onSelect: (value: VariableInsertValue) => void;
}

/**
 * Get color class based on variable type
 */
const getTypeColor = (type: string) => {
  switch (type) {
    case 'string':
      return 'bg-blue-100 text-blue-800 hover:bg-blue-200 dark:bg-blue-900/30 dark:text-blue-300';
    case 'number':
      return 'bg-green-100 text-green-800 hover:bg-green-200 dark:bg-green-900/30 dark:text-green-300';
    case 'boolean':
      return 'bg-purple-100 text-purple-800 hover:bg-purple-200 dark:bg-purple-900/30 dark:text-purple-300';
    case 'object':
      return 'bg-orange-100 text-orange-800 hover:bg-orange-200 dark:bg-orange-900/30 dark:text-orange-300';
    case 'file':
      return 'bg-pink-100 text-pink-800 hover:bg-pink-200 dark:bg-pink-900/30 dark:text-pink-300';
    default:
      if (type.startsWith('array')) {
        return 'bg-indigo-100 text-indigo-800 hover:bg-indigo-200 dark:bg-indigo-900/30 dark:text-indigo-300';
      }
      return 'bg-gray-100 text-gray-800 hover:bg-gray-200 dark:bg-gray-800 dark:text-gray-300';
  }
};

interface NestedFieldItemProps {
  field: StructuredTypeField;
  parentPath: string[];
  sourceId: string;
  sourceTitle?: string;
  onSelect: (value: VariableInsertValue) => void;
  onClose: () => void;
  depth?: number;
}

/**
 * Recursive component for rendering nested field items
 */
const NestedFieldItem: React.FC<NestedFieldItemProps> = ({
  field,
  parentPath,
  sourceId,
  sourceTitle,
  onSelect,
  onClose,
  depth = 0,
}) => {
  const [expanded, setExpanded] = useState(false);
  // Array types should not expand children (array may be empty at runtime)
  const isArrayType = field.type.startsWith('array');
  const hasChildren = !isArrayType && Array.isArray(field.children) && field.children.length > 0;
  const currentPath = React.useMemo(() => [...parentPath, field.key], [parentPath, field.key]);

  const handleSelect = useCallback(() => {
    onSelect({
      sourceId,
      key: currentPath.join('.'), // Full path as dot-separated string
      type: field.type as WorkflowVariable['type'],
      sourceTitle,
      path: currentPath,
    });
    onClose();
  }, [sourceId, field.type, sourceTitle, currentPath, onSelect, onClose]);

  const handleToggle = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation();
      setExpanded(!expanded);
    },
    [expanded]
  );

  return (
    <div className={cn(depth > 0 && 'ml-3 pl-2 border-l border-border')}>
      <div
        className={cn(
          'flex items-center gap-1 py-1 px-1.5 rounded cursor-pointer',
          'hover:bg-accent/50 transition-colors group'
        )}
        onClick={handleSelect}
      >
        {/* Expand toggle for nested children */}
        {hasChildren ? (
          <button
            type="button"
            onClick={handleToggle}
            className="p-0.5 hover:bg-accent rounded shrink-0"
          >
            <ChevronRight
              className={cn(
                'h-3 w-3 text-muted-foreground transition-transform',
                expanded && 'rotate-90'
              )}
            />
          </button>
        ) : (
          <div className="w-4 shrink-0" />
        )}

        {/* Field name */}
        <span className="font-mono text-xs truncate flex-1">{field.key}</span>

        {/* Type badge */}
        <span className={cn('text-[10px] px-1 py-0.5 rounded shrink-0', getTypeColor(field.type))}>
          {field.type}
        </span>
      </div>

      {/* Nested children */}
      {hasChildren && expanded && (
        <div className="mt-0.5">
          {(field.children ?? []).map(child => (
            <NestedFieldItem
              key={child.key}
              field={child}
              parentPath={currentPath}
              sourceId={sourceId}
              sourceTitle={sourceTitle}
              onSelect={onSelect}
              onClose={onClose}
              depth={depth + 1}
            />
          ))}
        </div>
      )}
    </div>
  );
};

/**
 * VariableItem - clickable badge representing a variable
 * Supports nested children expansion via popover for json-parser outputs
 */
const VariableItem: React.FC<VariableItemProps> = ({
  variable,
  sourceId,
  sourceTitle,
  onSelect,
}) => {
  const t = useT();
  const [open, setOpen] = useState(false);
  // Array types should not expand children (array may be empty at runtime)
  const isArrayType = variable.type.startsWith('array');
  const hasChildren =
    !isArrayType && Array.isArray(variable.children) && variable.children.length > 0;

  // Helper to resolve description text from descriptionKey and description
  const resolvedDescription = React.useMemo(() => {
    if (variable.descriptionKey) {
      return t(`nodes.${variable.descriptionKey}`, {
        innerType: variable.description || 'string',
      });
    }
    return variable.description || null;
  }, [t, variable.descriptionKey, variable.description]);

  const handleClick = useCallback(() => {
    onSelect({
      sourceId,
      key: variable.key,
      type: variable.type,
      sourceTitle,
      path: [variable.key],
    });
  }, [sourceId, variable.key, variable.type, sourceTitle, onSelect]);

  const handleClose = useCallback(() => {
    setOpen(false);
  }, []);

  const badgeContent = (
    <Badge
      role="button"
      tabIndex={0}
      onClick={hasChildren ? undefined : handleClick}
      onKeyDown={(e: React.KeyboardEvent<HTMLSpanElement>) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          if (!hasChildren) handleClick();
        }
      }}
      className={cn(
        'cursor-pointer select-none whitespace-nowrap rounded-md px-2 py-1 text-xs',
        'transition-colors inline-flex items-center gap-1',
        getTypeColor(variable.type)
      )}
    >
      {variable.key}
      {hasChildren && <ChevronRight className="h-3 w-3 opacity-60" />}
    </Badge>
  );

  // If has children, wrap in DropdownMenu for expansion
  if (hasChildren) {
    return (
      <DropdownMenu open={open} onOpenChange={setOpen}>
        <DropdownMenuTrigger asChild>{badgeContent}</DropdownMenuTrigger>
        <DropdownMenuContent
          side="bottom"
          align="start"
          className="w-64 max-h-[300px] overflow-y-auto p-1"
        >
          {/* Select root variable option */}
          <DropdownMenuItem
            onClick={() => {
              handleClick();
              handleClose();
            }}
            className="h-8 text-xs mb-1 border-b"
          >
            <span className="flex items-center gap-2 flex-1">
              <span className="font-mono font-medium">{variable.key}</span>
              <span
                className={cn(
                  'text-[10px] px-1 py-0.5 rounded ml-auto',
                  getTypeColor(variable.type)
                )}
              >
                {variable.type}
              </span>
            </span>
          </DropdownMenuItem>

          {/* Nested children */}
          <div className="ml-1 pl-1 border-l border-muted-foreground/30 py-0.5 space-y-0.5">
            {(variable.children ?? []).map(child => (
              <NestedFieldItem
                key={child.key}
                field={child}
                parentPath={[variable.key]}
                sourceId={sourceId}
                sourceTitle={sourceTitle}
                onSelect={onSelect}
                onClose={handleClose}
              />
            ))}
          </div>
        </DropdownMenuContent>
      </DropdownMenu>
    );
  }

  // No children - simple tooltip badge
  return (
    <Tooltip>
      <TooltipTrigger asChild>{badgeContent}</TooltipTrigger>
      <TooltipContent side="top" className="px-2 pt-1 pb-2 text-xs max-w-xs">
        <div>
          {t('nodes.end.outputs.type')}: {variable.type}
        </div>
        {resolvedDescription && (
          <div className="mt-1 text-muted-foreground">{resolvedDescription}</div>
        )}
      </TooltipContent>
    </Tooltip>
  );
};

export default React.memo(VariableItem);
