'use client';

import { useState, useEffect, useMemo } from 'react';
import { useT } from '@/i18n';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { ChevronsUpDown } from 'lucide-react';
import { cn } from '@/lib/utils';
import { DepartmentTreeItemDropdown } from '@/components/dashboard/organization/department-tree-item-dropdown';
import type { Department, DepartmentMember } from '@/services/types/organization';

interface EditMemberDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  member: DepartmentMember | null;
  departments: Department[];
  onSave: (newDeptId: string, member_name?: string) => Promise<void>;
  isSaving?: boolean;
}

export function EditMemberDialog({
  open,
  onOpenChange,
  member,
  departments,
  onSave,
  isSaving = false,
}: EditMemberDialogProps) {
  const t = useT('dashboard');
  const [selectedDepartmentId, setSelectedDepartmentId] = useState<string>('');
  const [memberName, setMemberName] = useState<string>('');
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set());
  const [selectOpen, setSelectOpen] = useState(false);

  // Get selected department name for display
  const getDepartmentName = useMemo(() => {
    const findDept = (depts: Department[], id: string): string | null => {
      for (const dept of depts) {
        if (dept.id === id) return dept.name;
        if (dept.children) {
          const found = findDept(dept.children, id);
          if (found) return found;
        }
      }
      return null;
    };
    return (id: string) => findDept(departments, id);
  }, [departments]);

  // Initialize selected department when member changes
  useEffect(() => {
    if (member) {
      setSelectedDepartmentId(member.department_id);
      setMemberName(member.member_name || member.account_name || '');
    } else {
      setSelectedDepartmentId('');
      setMemberName('');
    }
  }, [member]);

  // Expand first level by default when select opens
  useEffect(() => {
    if (selectOpen && expandedIds.size === 0) {
      setExpandedIds(new Set(departments.map(dept => dept.id)));
    }
  }, [selectOpen, departments, expandedIds.size]);

  const handleToggleExpand = (id: string) => {
    setExpandedIds(prev => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  const handleSelect = (id: string) => {
    setSelectedDepartmentId(id);
    setSelectOpen(false);
  };

  // Handle save
  const handleSave = async () => {
    if (member) {
      const deptId = selectedDepartmentId === 'ORG_ROOT' ? '' : selectedDepartmentId;
      const isDeptChanged = member.department_id !== selectedDepartmentId;
      const isNameChanged = (member.member_name || member.account_name) !== memberName;

      if (isDeptChanged || isNameChanged) {
        await onSave(deptId, isNameChanged ? memberName : undefined);
      }
    }
    onOpenChange(false);
  };

  // Handle dialog close
  const handleOpenChange = (isOpen: boolean) => {
    if (!isOpen) {
      setSelectedDepartmentId('');
    }
    onOpenChange(isOpen);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-md p-0 overflow-visible flex flex-col">
        <DialogHeader>
          <DialogTitle className="text-xl font-bold tracking-tight">
            {t('organization.contacts.editDialog.title')}
          </DialogTitle>
        </DialogHeader>

        <DialogBody className="py-6 space-y-6 overflow-visible">
          <div className="space-y-2">
            <Label htmlFor="edit-member-name" className="text-sm font-bold text-foreground ml-1">
              {t('organization.contacts.editDialog.name')}
            </Label>
            <Input
              id="edit-member-name"
              value={memberName}
              onChange={e => setMemberName(e.target.value)}
              placeholder={t('organization.contacts.editDialog.name')}
              className="h-12 rounded-xl bg-card border text-foreground font-medium focus:ring-2 focus:ring-brand-main/20"
              maxLength={50}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="edit-member-email" className="text-sm font-bold text-foreground ml-1">
              {t('organization.contacts.editDialog.email')}
            </Label>
            <Input
              id="edit-member-email"
              value={member?.account_email || ''}
              readOnly
              disabled
              className="h-12 rounded-xl bg-muted border text-muted-foreground font-medium cursor-not-allowed"
            />
          </div>

          <div className="space-y-2">
            <Label
              htmlFor="edit-member-department"
              className="text-sm font-bold text-foreground ml-1"
            >
              {t('organization.contacts.editDialog.department')}
            </Label>
            <div className="relative">
              <button
                type="button"
                id="edit-member-department"
                onClick={() => setSelectOpen(!selectOpen)}
                className={cn(
                  'flex h-12 w-full items-center justify-between rounded-xl border bg-card px-4 py-2 text-sm font-medium transition-all duration-200',
                  selectOpen
                    ? 'border-brand-main ring-2 ring-brand-main/10'
                    : 'border hover:border-strong hover:bg-muted'
                )}
              >
                <span className="truncate">
                  {selectedDepartmentId
                    ? getDepartmentName(selectedDepartmentId)
                    : t('organization.contacts.editDialog.selectDepartment')}
                </span>
                <ChevronsUpDown className="h-4 w-4 opacity-40 shrink-0" />
              </button>
              {selectOpen && (
                <>
                  <div className="fixed inset-0 z-40" onClick={() => setSelectOpen(false)} />
                  <div className="absolute z-50 mt-2 w-full max-h-[300px] overflow-y-auto rounded-xl border bg-popover shadow-premium animate-in fade-in slide-in-from-top-2 duration-200">
                    <div className="p-2">
                      {departments.map(dept => (
                        <DepartmentTreeItemDropdown
                          key={dept.id}
                          department={dept}
                          level={0}
                          selectedId={selectedDepartmentId}
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
            onClick={() => handleOpenChange(false)}
            className="px-6 font-semibold"
          >
            {t('organization.contacts.editDialog.cancel')}
          </Button>
          <Button
            size="xl"
            onClick={handleSave}
            disabled={isSaving}
            className="px-8 font-semibold"
          >
            {isSaving
              ? t('organization.contacts.editDialog.saving')
              : t('organization.contacts.editDialog.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
