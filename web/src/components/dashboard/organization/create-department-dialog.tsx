'use client';

import React, { useState, useEffect, useCallback } from 'react';
import { useT } from '@/i18n';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogBody,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { ChevronsUpDown } from 'lucide-react';
import { DepartmentTreeItemDropdown } from '@/components/dashboard/organization/department-tree-item-dropdown';
import { cn } from '@/lib/utils';
import { useCreateDepartment } from '@/hooks/organization/use-department-actions';
import { toast } from 'sonner';
import type { Department } from '@/services/types/organization';

interface CreateDepartmentDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  departments: Department[];
  parentDepartmentId?: string;
}

export function CreateDepartmentDialog({
  open,
  onOpenChange,
  departments,
  parentDepartmentId,
}: CreateDepartmentDialogProps) {
  const t = useT('dashboard');
  const { createDepartment, isCreating } = useCreateDepartment();
  const [departmentName, setDepartmentName] = useState('');
  const [nameError, setNameError] = useState('');
  const [selectedParentId, setSelectedParentId] = useState<string>(parentDepartmentId || '');
  const [selectOpen, setSelectOpen] = useState(false);
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set());

  // Calculate department level (0-based, where 0 is first level)
  const getDepartmentLevel = (id: string, depts: Department[], currentLevel = 0): number | null => {
    if (id === 'ORG_ROOT') return -1; // Root is level -1, so first level is 0
    for (const dept of depts) {
      if (dept.id === id) return currentLevel;
      if (dept.children) {
        const found = getDepartmentLevel(id, dept.children, currentLevel + 1);
        if (found !== null) return found;
      }
    }
    return null;
  };

  // Update selected parent when parentDepartmentId prop changes
  useEffect(() => {
    if (parentDepartmentId) {
      setSelectedParentId(parentDepartmentId);
    }
  }, [parentDepartmentId]);

  // Get selected department name for display
  const getDepartmentName = (id: string): string | null => {
    const findDept = (depts: Department[], targetId: string): string | null => {
      for (const dept of depts) {
        if (dept.id === targetId) return dept.name;
        if (dept.children) {
          const found = findDept(dept.children, targetId);
          if (found) return found;
        }
      }
      return null;
    };
    return findDept(departments, id);
  };

  // Expand first level by default when select opens
  const handleSelectOpen = (open: boolean) => {
    setSelectOpen(open);
    if (open && expandedIds.size === 0) {
      setExpandedIds(new Set(departments.map(dept => dept.id)));
    }
  };

  const handleToggleExpand = useCallback((id: string) => {
    setExpandedIds((prev: Set<string>) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  }, []);

  const handleSelect = useCallback((id: string) => {
    setSelectedParentId(id);
    setSelectOpen(false);
  }, []);

  const handleSubmit = async () => {
    const trimmedName = departmentName.trim();
    if (!trimmedName) {
      setNameError(t('organization.validation.nameRequired'));
      return;
    }

    if (trimmedName.length > 50) {
      setNameError(t('organization.validation.nameTooLong', { max: 50 }));
      return;
    }

    if (!selectedParentId) {
      return;
    }

    // Check if selected parent is already at level 4 (5th level)
    // If so, cannot create sub-department (max 5 levels)
    const parentLevel = getDepartmentLevel(selectedParentId, departments);
    if (parentLevel !== null && parentLevel >= 4) {
      toast.error(t('organization.contacts.createDepartment.maxLevelReached'), {
        description: t('organization.contacts.createDepartment.maxLevelReachedDesc'),
      });
      return;
    }

    try {
      // If organization root is selected, use empty string for parent_id (creates top-level department)
      const parentId = selectedParentId === 'ORG_ROOT' ? '' : selectedParentId;

      await createDepartment({
        name: trimmedName,
        parent_id: parentId,
      });

      // Reset form
      setDepartmentName('');
      setNameError('');
      setSelectedParentId('');
      onOpenChange(false);
    } catch (error) {
      // Error is handled by the mutation
      console.error('Failed to create department:', error);
    }
  };

  const handleCancel = () => {
    setDepartmentName('');
    setNameError('');
    setSelectedParentId('');
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md p-0 overflow-visible flex flex-col">
        <DialogHeader>
          <DialogTitle className="text-xl font-bold tracking-tight">
            {t('organization.contacts.createDepartment.title')}
          </DialogTitle>
        </DialogHeader>

        <DialogBody className="py-6 space-y-6 overflow-visible">
          <div className="space-y-2">
            <Label htmlFor="department-name" className="text-sm font-bold text-foreground ml-1">
              {t('organization.contacts.createDepartment.departmentName')}
            </Label>
            <Input
              id="department-name"
              placeholder={t('organization.contacts.createDepartment.departmentNamePlaceholder')}
              value={departmentName}
              onChange={e => {
                setDepartmentName(e.target.value);
                if (nameError) setNameError('');
              }}
              maxLength={50}
              showCharacterCount
              errorText={nameError}
              className="h-12 rounded-xl border focus:border-brand-main focus:ring-brand-main/10 transition-all"
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="parent-department" className="text-sm font-bold text-foreground ml-1">
              {t('organization.contacts.createDepartment.parentDepartment')}
            </Label>
            <div className="relative">
              <button
                type="button"
                id="parent-department"
                onClick={() => handleSelectOpen(!selectOpen)}
                className={cn(
                  'flex h-12 w-full items-center justify-between rounded-xl border bg-card px-4 py-2 text-sm font-medium transition-all duration-200',
                  selectOpen
                    ? 'border-brand-main ring-2 ring-brand-main/10'
                    : 'border hover:border-strong hover:bg-muted'
                )}
              >
                <span className="truncate">
                  {selectedParentId
                    ? getDepartmentName(selectedParentId)
                    : t('organization.contacts.createDepartment.selectDepartment')}
                </span>
                <ChevronsUpDown className="h-4 w-4 opacity-40 shrink-0" />
              </button>
              {selectOpen && (
                <>
                  <div className="fixed inset-0 z-40" onClick={() => handleSelectOpen(false)} />
                  <div className="absolute z-50 mt-2 w-full max-h-[300px] overflow-y-auto rounded-xl border bg-popover shadow-premium animate-in fade-in slide-in-from-top-2 duration-200">
                    <div className="p-2">
                      {departments.map(dept => (
                        <DepartmentTreeItemDropdown
                          key={dept.id}
                          department={dept}
                          level={0}
                          selectedId={selectedParentId}
                          onSelect={handleSelect}
                          expandedIds={expandedIds}
                          onToggleExpand={handleToggleExpand}
                          showCheckIcon
                        />
                      ))}
                    </div>
                  </div>
                </>
              )}
            </div>
          </div>
        </DialogBody>

        <DialogFooter className="bg-muted/50 pt-4 pb-6 px-6 border-t gap-3">
          <Button variant="ghost" size="xl" onClick={handleCancel} className="px-6 font-semibold">
            {t('organization.contacts.createDepartment.cancel')}
          </Button>
          <Button
            size="xl"
            onClick={handleSubmit}
            disabled={!departmentName.trim() || !selectedParentId || isCreating}
            className="px-8 font-semibold"
          >
            {isCreating
              ? t('organization.contacts.createDepartment.creating')
              : t('organization.contacts.createDepartment.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
