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
import { useUpdateDepartment } from '@/hooks/organization/use-department-actions';
import type { Department } from '@/services/types/organization';

interface EditDepartmentDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  department: Department | null;
  departments: Department[];
}

export function EditDepartmentDialog({
  open,
  onOpenChange,
  department,
  departments,
}: EditDepartmentDialogProps) {
  const t = useT('dashboard');
  const { updateDepartment, isUpdating } = useUpdateDepartment();

  const [name, setName] = useState('');
  const [nameError, setNameError] = useState('');
  const [selectedParentId, setSelectedParentId] = useState<string>('');
  const [selectOpen, setSelectOpen] = useState(false);
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set());

  // Initialize form when department changes
  useEffect(() => {
    if (department) {
      setName(department.name);
      setNameError('');
      // Use empty string to represent organization root if parent_id is null/undefined/empty
      setSelectedParentId(department.parent_id || 'ORG_ROOT');
    } else {
      setName('');
      setNameError('');
      setSelectedParentId('');
    }
  }, [department]);

  // Expand first level by default when select opens
  useEffect(() => {
    if (selectOpen && expandedIds.size === 0) {
      setExpandedIds(new Set(departments.map(dept => dept.id)));
    }
  }, [selectOpen, departments, expandedIds.size]);

  // Get selected parent department name for display
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

  const handleToggleExpand = useCallback((id: string) => {
    setExpandedIds(prev => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  }, []);

  const isChildDepartment = useCallback((targetId: string, currentDept: Department): boolean => {
    if (!currentDept.children) return false;
    for (const child of currentDept.children) {
      if (child.id === targetId) return true;
      if (isChildDepartment(targetId, child)) return true;
    }
    return false;
  }, []);

  const handleSelect = useCallback(
    (id: string) => {
      // Prevent selecting itself or its children as parent
      if (department && (id === department.id || isChildDepartment(id, department))) {
        return;
      }
      setSelectedParentId(id);
      setSelectOpen(false);
    },
    [department, isChildDepartment]
  );

  const handleSubmit = async () => {
    if (!department) return;
    const trimmedName = name.trim();

    if (!trimmedName) {
      setNameError(t('organization.validation.nameRequired'));
      return;
    }

    if (trimmedName.length > 50) {
      setNameError(t('organization.validation.nameTooLong', { max: 50 }));
      return;
    }

    try {
      // If organization root is selected (or kept as ORG_ROOT), pass empty string for parent_id
      // (Assuming 'ORG_ROOT' is the ID for the virtual organization node)
      const parentId = selectedParentId === 'ORG_ROOT' ? '' : selectedParentId;

      await updateDepartment({
        deptId: department.id,
        data: {
          name: trimmedName,
          parent_id: parentId,
        },
      });
      onOpenChange(false);
    } catch (error) {
      console.error('Failed to update department:', error);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md p-0 overflow-visible flex flex-col">
        <DialogHeader>
          <DialogTitle className="text-xl font-bold tracking-tight">
            {t('organization.contacts.editDepartment.title')}
          </DialogTitle>
        </DialogHeader>

        <DialogBody className="py-6 space-y-6 overflow-visible">
          <div className="space-y-2">
            <Label htmlFor="dept-name" className="text-sm font-bold text-foreground ml-1">
              {t('organization.contacts.editDepartment.departmentName')}
            </Label>
            <Input
              id="dept-name"
              value={name}
              onChange={e => {
                setName(e.target.value);
                if (nameError) setNameError('');
              }}
              placeholder={t('organization.contacts.editDepartment.departmentNamePlaceholder')}
              maxLength={50}
              showCharacterCount
              errorText={nameError}
              className="h-12 rounded-xl border focus:border-brand-main focus:ring-brand-main/10 transition-all"
            />
          </div>

          <div className="space-y-2">
            <Label className="text-sm font-bold text-foreground ml-1">
              {t('organization.contacts.editDepartment.parentDepartment')}
            </Label>
            <div className="relative">
              <button
                type="button"
                onClick={() => setSelectOpen(!selectOpen)}
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
                    : t('organization.contacts.editDepartment.selectDepartment')}
                </span>
                <ChevronsUpDown className="h-4 w-4 opacity-40 shrink-0" />
              </button>
              {selectOpen && (
                <>
                  <div className="fixed inset-0 z-40" onClick={() => setSelectOpen(false)} />
                  <div className="absolute z-50 mt-2 w-full max-h-[300px] overflow-y-auto rounded-xl border bg-popover shadow-premium animate-in fade-in slide-in-from-top-2 duration-200">
                    <div className="p-1">
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
          <Button
            variant="ghost"
            size="xl"
            onClick={() => onOpenChange(false)}
            disabled={isUpdating}
            className="px-6 font-semibold"
          >
            {t('organization.contacts.editDepartment.cancel')}
          </Button>
          <Button
            size="xl"
            onClick={handleSubmit}
            disabled={!name.trim() || !selectedParentId || isUpdating}
            className="px-8 font-semibold"
          >
            {isUpdating
              ? t('organization.contacts.editDepartment.saving')
              : t('organization.contacts.editDepartment.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
