'use client';

import { useMemo, useState, useCallback, useRef, useEffect } from 'react';
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Input } from '@/components/ui/input';
import { Search, Building2 } from 'lucide-react';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { useWorkspaces } from '@/hooks/workspace/use-workspaces';

export interface WorkspaceSelectorValue {
  id: string;
  name: string;
}

const emptyExcludedWorkspaceIds: string[] = [];

export interface WorkspaceSelectorProps {
  /** Current selected workspace ID */
  value?: WorkspaceSelectorValue;
  /** Callback triggered when selection changes */
  onChange?: (value: WorkspaceSelectorValue) => void;
  /** Placeholder text displayed when no value selected */
  placeholder?: string;
  /** Additional CSS classes for the trigger */
  className?: string;
  /** Disable select */
  disabled?: boolean;
  /** Enable search functionality */
  searchable?: boolean;
  /** Automatically select the first workspace when no value is provided */
  autoSelectFirst?: boolean;
  /** Workspace IDs to hide from the dropdown */
  excludedWorkspaceIds?: string[];
}

/**
 * WorkspaceSelector – A dropdown component for selecting a workspace.
 * Features include search functionality and object-based value handling.
 */
export function WorkspaceSelector({
  value,
  onChange,
  placeholder,
  className,
  disabled = false,
  searchable = true,
  autoSelectFirst = false,
  excludedWorkspaceIds = emptyExcludedWorkspaceIds,
}: WorkspaceSelectorProps) {
  const t = useT('common');
  const [searchQuery, setSearchQuery] = useState('');
  const searchInputRef = useRef<HTMLInputElement>(null);

  // Fetch workspaces in the current organization. Creation dialogs use this
  // selector to choose the owning workspace without changing the global context.
  const workspaceQuery = useWorkspaces('', 1, 1000);
  const workspaces = workspaceQuery.workspaces;
  const isLoading = workspaceQuery.isLoading;
  const excludedWorkspaceIdSet = useMemo(
    () => new Set(excludedWorkspaceIds.filter(Boolean)),
    [excludedWorkspaceIds]
  );
  const selectableWorkspaces = useMemo(
    () =>
      workspaces.filter(
        (workspace: WorkspaceSelectorValue) => !excludedWorkspaceIdSet.has(workspace.id)
      ),
    [excludedWorkspaceIdSet, workspaces]
  );

  // Use translated placeholder if none provided
  const effectivePlaceholder = placeholder || t('workspaceSelector.placeholder');

  useEffect(() => {
    if (!autoSelectFirst || value?.id || isLoading || !selectableWorkspaces.length) {
      return;
    }

    const [firstWorkspace] = selectableWorkspaces;
    if (firstWorkspace) {
      onChange?.({ id: firstWorkspace.id, name: firstWorkspace.name });
    }
  }, [autoSelectFirst, isLoading, onChange, selectableWorkspaces, value?.id]);

  // Clear search when dropdown closes or component unmounts
  useEffect(() => {
    return () => {
      setSearchQuery('');
    };
  }, []);

  // Filter workspaces based on search query, but keep the selected value pinned at the top
  const filteredWorkspaces = useMemo(() => {
    if (!selectableWorkspaces) return selectableWorkspaces;
    if (!searchQuery) return selectableWorkspaces;
    return selectableWorkspaces.filter((t: WorkspaceSelectorValue) =>
      t.name.toLowerCase().includes(searchQuery.toLowerCase())
    );
  }, [searchQuery, selectableWorkspaces]);

  // Ensure the current selection remains visible even if it doesn't match the search
  const computedWorkspaces = useMemo(() => {
    const base = filteredWorkspaces || [];
    const selId = value?.id;
    if (!selId) return base;
    const exists = base.some((w: WorkspaceSelectorValue) => w.id === selId);
    if (exists) return base;
    const selectedFromSource =
      (selectableWorkspaces || []).find((w: WorkspaceSelectorValue) => w.id === selId) || value;
    return [selectedFromSource, ...base];
  }, [filteredWorkspaces, selectableWorkspaces, value]);

  // Whether current value exists in options; used for fallback display
  const valueInOptions = useMemo(() => {
    if (!value?.id) return false;
    return (selectableWorkspaces || []).some((w: WorkspaceSelectorValue) => w.id === value.id);
  }, [selectableWorkspaces, value]);

  // Handle workspace selection
  const handleWorkspaceSelect = useCallback(
    (workspaceValue: string) => {
      // Guard against empty/invalid values emitted by underlying Select when clearing
      if (!workspaceValue || workspaceValue.trim() === '') return;
      try {
        const parsed = JSON.parse(workspaceValue) as WorkspaceSelectorValue;
        if (parsed && parsed.id) {
          onChange?.(parsed);
        }
      } catch (error) {
        // Ignore parsing errors silently; do not break user interaction
        console.error('Failed to parse workspace value:', error);
      }
    },
    [onChange]
  );

  // Handle search input changes
  const handleSearchChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    setSearchQuery(e.target.value);
  }, []);

  // Handle search input key events
  const handleSearchKeyDown = useCallback((e: React.KeyboardEvent<HTMLInputElement>) => {
    // Prevent the select from closing when typing in search
    e.stopPropagation();
  }, []);

  // Clear search when opening dropdown
  const handleOpenChange = useCallback((open: boolean) => {
    if (open && searchInputRef.current) {
      // Focus search input when dropdown opens
      setTimeout(() => {
        searchInputRef.current?.focus();
      }, 100);
    } else if (!open) {
      // Clear search when dropdown closes
      setSearchQuery('');
    }
  }, []);

  // Convert value to string for Select component
  const selectValue = useMemo(() => {
    if (value) {
      return JSON.stringify(value);
    }
    // Use empty string to keep Select controlled from first render
    return '';
  }, [value]);

  return (
    <Select
      value={selectValue}
      onValueChange={handleWorkspaceSelect}
      disabled={disabled || isLoading}
      onOpenChange={handleOpenChange}
    >
      <SelectTrigger
        className={cn('w-full', className)}
        id="workspace-selector-trigger"
        isLoading={isLoading}
      >
        <div className="flex items-center gap-2 overflow-hidden">
          <Building2 className="h-4 w-4 shrink-0 opacity-70" />
          {/* Fallback label when selected value is not present in options */}
          {value && !isLoading && !valueInOptions ? (
            <span className="truncate">{value.name || 'Unknown'}</span>
          ) : (
            // Otherwise, let Radix render the current selected item's text
            <SelectValue placeholder={effectivePlaceholder} />
          )}
        </div>
      </SelectTrigger>
      <SelectContent className="h-[400px] px-0" data-workspace-selector-content>
        <div className="h-full flex flex-col">
          {/* Search input */}
          {searchable && (
            <div className="p-2 border-b">
              <div className="relative">
                <Search className="absolute left-2 top-2.5 h-4 w-4 text-muted-foreground pointer-events-none" />
                <Input
                  ref={searchInputRef}
                  placeholder={t('workspaceSelector.search')}
                  value={searchQuery}
                  onChange={handleSearchChange}
                  onKeyDown={handleSearchKeyDown}
                  className="pl-8"
                  autoComplete="off"
                />
              </div>
            </div>
          )}

          {/* Workspace list */}
          <div className="h-0 grow overflow-y-auto p-1">
            {isLoading ? (
              <div className="px-3 py-6 text-center text-muted-foreground">
                {t('workspaceSelector.loading')}
              </div>
            ) : computedWorkspaces && computedWorkspaces.length > 0 ? (
              <SelectGroup>
                {computedWorkspaces.map((workspace: WorkspaceSelectorValue) => (
                  <SelectItem
                    key={workspace.id}
                    value={JSON.stringify({ id: workspace.id, name: workspace.name })}
                    className="cursor-pointer mx-1 rounded-sm"
                  >
                    <div className="flex items-center gap-2">
                      <span className="truncate">{workspace.name}</span>
                    </div>
                  </SelectItem>
                ))}
              </SelectGroup>
            ) : (
              <div className="px-3 py-6 text-center text-muted-foreground">
                {searchQuery
                  ? t('workspaceSelector.noResults')
                  : t('workspaceSelector.noWorkspaces')}
              </div>
            )}
          </div>
        </div>
      </SelectContent>
    </Select>
  );
}
