'use client';

import { useState } from 'react';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { ChevronRight, MoreHorizontal, Plus, Pencil, Trash2 } from 'lucide-react';
import type { Department } from '@/services/types/organization';
import { cn } from '@/lib/utils';

interface DepartmentTreeItemSidebarProps {
  department: Department;
  level?: number;
  selectedId: string | null;
  onSelect: (id: string) => void;
  searchKeyword?: string;
  // Actions
  onAddSubDepartment?: (department: Department) => void;
  onEditDepartment?: (department: Department) => void;
  onDeleteDepartment?: (department: Department) => void;
}

export function DepartmentTreeItemSidebar({
  department,
  level = 0,
  selectedId,
  onSelect,
  searchKeyword = '',
  onAddSubDepartment,
  onEditDepartment,
  onDeleteDepartment,
}: DepartmentTreeItemSidebarProps) {
  const t = useT('dashboard');
  const hasChildren = department.children && department.children.length > 0;
  const [isExpanded, setIsExpanded] = useState(level === 0);

  // Filter based on search
  const matchesSearch =
    !searchKeyword || department.name.toLowerCase().includes(searchKeyword.toLowerCase());

  if (!matchesSearch && !hasChildren) return null;

  const isSelected = selectedId === department.id;

  const handleToggle = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (hasChildren) {
      setIsExpanded(!isExpanded);
    }
  };

  const handleSelect = () => {
    onSelect(department.id);
  };

  const handleAddSubDepartment = (e: React.MouseEvent) => {
    e.stopPropagation();
    onAddSubDepartment?.(department);
  };

  const handleEditDepartment = (e: React.MouseEvent) => {
    e.stopPropagation();
    onEditDepartment?.(department);
  };

  const handleDeleteDepartment = (e: React.MouseEvent) => {
    e.stopPropagation();
    onDeleteDepartment?.(department);
  };

  // Show actions menu (exclude organization root node)
  const showActions =
    department.id !== 'ORG_ROOT' && (onAddSubDepartment || onEditDepartment || onDeleteDepartment);

  const canAddSubDepartment = level < 5 && onAddSubDepartment;

  return (
    <div className={cn(level > 0 && 'mb-0.5')}>
      <div
        className={cn(
          'group flex cursor-pointer items-center gap-1 rounded-lg pr-1.5 transition-colors hover:bg-muted',
          isSelected && 'bg-primary/10 text-primary shadow-[inset_3px_0_0_hsl(var(--primary))]'
        )}
        onClick={handleSelect}
      >
        {/* Expand/Collapse button */}
        {hasChildren ? (
          <Button
            variant="ghost"
            isIcon
            className={cn(
              'h-8 w-7 shrink-0 pl-1.5 text-muted-foreground hover:bg-transparent hover:text-primary',
              'transition-transform duration-200'
            )}
            onClick={handleToggle}
          >
            <ChevronRight
              className={cn('h-4 w-4 transition-transform duration-200', isExpanded && 'rotate-90')}
            />
          </Button>
        ) : (
          <div className="h-8 w-7 shrink-0" />
        )}

        {/* Department name with level-based styling */}
        <span
          className={cn(
            'flex-1 truncate text-sm transition-colors',
            level === 0 && 'font-semibold',
            level === 1 && 'font-medium',
            level >= 2 && 'font-normal',
            isSelected && 'text-primary',
            !isSelected && 'text-foreground'
          )}
        >
          {department.name}
        </span>

        {/* Actions menu */}
        {showActions && (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                isIcon
                className="h-7 w-7 shrink-0 rounded-md p-0 opacity-0 transition-opacity hover:bg-accent group-hover:opacity-100"
                onClick={e => e.stopPropagation()}
              >
                <MoreHorizontal className="h-3.5 w-3.5" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" onClick={e => e.stopPropagation()}>
              {canAddSubDepartment && (
                <DropdownMenuItem onClick={handleAddSubDepartment}>
                  <Plus className="h-4 w-4 mr-2" />
                  <span>{t('organization.contacts.departmentActions.addSubDepartment')}</span>
                </DropdownMenuItem>
              )}
              {onEditDepartment && (
                <DropdownMenuItem onClick={handleEditDepartment}>
                  <Pencil className="h-4 w-4 mr-2" />
                  <span>{t('organization.contacts.departmentActions.editDepartment')}</span>
                </DropdownMenuItem>
              )}
              {onDeleteDepartment && (
                <DropdownMenuItem onClick={handleDeleteDepartment} className="text-destructive">
                  <Trash2 className="h-4 w-4 mr-2" />
                  <span>{t('organization.contacts.departmentActions.delete')}</span>
                </DropdownMenuItem>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </div>

      {/* Children with vertical line */}
      {hasChildren && isExpanded && department.children && (
        <div className="ml-4 mt-1 space-y-1">
          {department.children.map(child => (
            <DepartmentTreeItemSidebar
              key={child.id}
              department={child}
              level={level + 1}
              selectedId={selectedId}
              onSelect={onSelect}
              searchKeyword={searchKeyword}
              onAddSubDepartment={onAddSubDepartment}
              onEditDepartment={onEditDepartment}
              onDeleteDepartment={onDeleteDepartment}
            />
          ))}
        </div>
      )}
    </div>
  );
}
