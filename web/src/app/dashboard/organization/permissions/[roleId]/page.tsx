'use client';

import { useState, useEffect, useMemo } from 'react';
import { useRouter, useParams } from 'next/navigation';
import { Button } from '@/components/ui/button';
import { Switch } from '@/components/ui/switch';
import { ChevronLeft, Loader2, Pencil, Check } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useRoleDetail } from '@/hooks/organization/use-role-detail';
import { useRoleActions } from '@/hooks/organization/use-role-actions';
import { EditRoleInfoDialog } from '@/components/dashboard/organization/edit-role-info-dialog';
import { toast } from 'sonner';
import { Skeleton } from '@/components/ui/skeleton';
import { pickLocale } from '@/utils/tool-helpers';
import { useLocale } from '@/hooks/use-locale';
import { PERMISSION_MODULES } from '@/constants/permissions';
import { useT } from '@/i18n';
import { Badge } from '@/components/ui/badge';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';

export default function RoleConfigPage() {
  const { locale } = useLocale();

  const t = useT();
  const router = useRouter();
  const params = useParams();

  const roleId = params.roleId as string;
  const isNewRole = roleId === 'new';

  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [selectedPermissions, setSelectedPermissions] = useState<Set<string>>(new Set());
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [savedName, setSavedName] = useState('');
  const [savedDescription, setSavedDescription] = useState('');
  const [savedPermissions, setSavedPermissions] = useState<Set<string>>(new Set());
  const [leaveConfirmOpen, setLeaveConfirmOpen] = useState(false);
  const [newRoleInfoPrompted, setNewRoleInfoPrompted] = useState(false);

  const { role, isLoading: loading } = useRoleDetail(isNewRole ? null : roleId, !isNewRole);

  const { createRole, updateRolePermissions, updateRoleInfo, isSaving } = useRoleActions();

  // Initialize form data when role is loaded
  useEffect(() => {
    if (role) {
      const nextDescription =
        role?.builtin && role?.description_i18n
          ? pickLocale(role?.description_i18n, locale, role?.description || '')
          : role?.description || '';

      setName(role.name);
      setDescription(nextDescription);
      setSelectedPermissions(new Set(role.permissions));
      setSavedName(role.name);
      setSavedDescription(nextDescription);
      setSavedPermissions(new Set(role.permissions));
    } else if (isNewRole) {
      setSavedName('');
      setSavedDescription('');
      setSavedPermissions(new Set());
    }
  }, [role, locale, isNewRole]);

  useEffect(() => {
    if (isNewRole && !newRoleInfoPrompted) {
      setEditDialogOpen(true);
      setNewRoleInfoPrompted(true);
    }
  }, [isNewRole, newRoleInfoPrompted]);

  const hasPermissionChanges = useMemo(() => {
    if (selectedPermissions.size !== savedPermissions.size) return true;
    for (const code of selectedPermissions) {
      if (!savedPermissions.has(code)) return true;
    }
    return false;
  }, [savedPermissions, selectedPermissions]);

  const hasUnsavedChanges =
    name.trim() !== savedName.trim() ||
    description.trim() !== savedDescription.trim() ||
    hasPermissionChanges;

  useEffect(() => {
    const handleBeforeUnload = (event: BeforeUnloadEvent) => {
      if (!hasUnsavedChanges || isSaving) return;
      event.preventDefault();
      event.returnValue = '';
    };

    window.addEventListener('beforeunload', handleBeforeUnload);
    return () => window.removeEventListener('beforeunload', handleBeforeUnload);
  }, [hasUnsavedChanges, isSaving]);

  // Handle permission toggle
  const handlePermissionToggle = (code: string) => {
    setSelectedPermissions(prev => {
      const next = new Set(prev);
      if (next.has(code)) {
        next.delete(code);
      } else {
        next.add(code);
      }
      return next;
    });
  };

  // Get all permission codes
  const allPermissionCodes = PERMISSION_MODULES.flatMap(module =>
    module.permissions.map(p => p.code)
  );

  // Check if all permissions are enabled
  const isAllEnabled = allPermissionCodes.every(code => selectedPermissions.has(code));

  // Handle toggle all permissions
  const handleToggleAll = (checked: boolean) => {
    if (checked) {
      setSelectedPermissions(new Set(allPermissionCodes));
    } else {
      setSelectedPermissions(new Set());
    }
  };

  // Check if all permissions in a module are enabled
  const isModuleAllEnabled = (module: (typeof PERMISSION_MODULES)[0]) => {
    return module.permissions.every(permission => selectedPermissions.has(permission.code));
  };

  // Handle toggle all permissions for a module
  const handleToggleModule = (module: (typeof PERMISSION_MODULES)[0], checked: boolean) => {
    setSelectedPermissions(prev => {
      const next = new Set(prev);
      if (checked) {
        module.permissions.forEach(permission => {
          next.add(permission.code);
        });
      } else {
        module.permissions.forEach(permission => {
          next.delete(permission.code);
        });
      }
      return next;
    });
  };

  // Handle save role info
  const handleSaveRoleInfo = async (newName: string, newDescription: string) => {
    if (isNewRole) {
      setName(newName);
      setDescription(newDescription);
    } else {
      await updateRoleInfo({
        roleId,
        data: {
          name: newName,
          description: newDescription,
        },
      });
      setSavedName(newName);
      setSavedDescription(newDescription);
    }
  };

  // Handle save permissions
  const handleSave = async () => {
    if (!name.trim()) {
      toast.error(t('dashboard.organization.permissions.config.nameRequired'));
      return;
    }

    if (isNewRole) {
      // Create new role
      await createRole({
        name: name.trim(),
        description: description.trim() || undefined,
        permissions: Array.from(selectedPermissions),
      });
    } else {
      // Update role permissions
      await updateRolePermissions({
        roleId,
        data: { permissions: Array.from(selectedPermissions) },
      });
      setSavedPermissions(new Set(selectedPermissions));
    }
  };

  const handleAttemptLeave = () => {
    if (hasUnsavedChanges && !isSaving) {
      setLeaveConfirmOpen(true);
      return;
    }

    router.push('/dashboard/organization/permissions');
  };

  // Check if role is editable
  const canEdit = isNewRole || !!(role && !role.builtin);

  if (loading) {
    return (
      <div className="p-4 space-y-5">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-24 w-full" />
        <Skeleton className="h-48 w-full" />
      </div>
    );
  }

  return (
    <div className="h-full flex flex-col bg-bg-canvas/50 overflow-hidden">
      <header className="sticky top-0 z-20 flex items-center justify-between border-b border-border/60 bg-background px-6 py-4 lg:px-8">
        <div className="flex items-center gap-4">
          <Button
            variant="ghost"
            isIcon
            onClick={handleAttemptLeave}
            className="h-9 w-9 rounded-md border border-border/70 shadow-none transition-colors hover:bg-accent/80"
          >
            <ChevronLeft className="h-5 w-5" />
          </Button>
          <div className="flex-1">
            <div className="flex items-center gap-2">
              <h1 className="text-xl font-semibold tracking-tight text-text-primary">
                {isNewRole
                  ? name || t('dashboard.organization.permissions.config.createTitle')
                  : name || role?.name}
              </h1>
              {hasUnsavedChanges ? (
                <Badge variant="warning" className="font-medium">
                  {t('dashboard.organization.permissions.config.unsavedChanges')}
                </Badge>
              ) : null}
              {canEdit && (
                <Button
                  variant="ghost"
                  isIcon
                  className="h-7 w-7 rounded-md transition-colors hover:bg-accent/80"
                  onClick={() => setEditDialogOpen(true)}
                >
                  <Pencil className="h-3.5 w-3.5" />
                </Button>
              )}
            </div>
            {description && (
              <p className="text-xs text-text-secondary mt-0.5 line-clamp-3 max-w-xl break-words">
                {description}
              </p>
            )}
          </div>
        </div>
        <div className="flex items-center gap-3">
          <Button
            variant="ghost"
            onClick={handleAttemptLeave}
            className="h-9 rounded-md px-4 text-xs font-semibold transition-colors hover:bg-accent"
          >
            {t('common.cancel')}
          </Button>
          <Button
            onClick={handleSave}
            disabled={!canEdit || isSaving || (!isNewRole && !hasUnsavedChanges)}
            className="h-9 rounded-md bg-primary px-6 text-xs font-semibold text-primary-foreground shadow-sm transition-colors hover:bg-primary-hover hover:text-primary-foreground"
          >
            {isSaving ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin mr-2" />
            ) : (
              <Check className="h-3.5 w-3.5 mr-2" />
            )}
            {t('dashboard.organization.permissions.config.saveConfig')}
          </Button>
        </div>
      </header>

      {/* Main Content Area */}
      <main className="flex-1 space-y-5 overflow-y-auto p-4 scrollbar-thin scrollbar-thumb-muted-foreground/10 lg:p-6">
        <div className="max-w-7xl mx-auto space-y-6">
          {/* Global Enable All Header */}
          <div className="flex items-center justify-between rounded-xl border border-border/80 bg-background p-5 shadow-sm">
            <div>
              <h3 className="text-sm font-bold text-text-primary uppercase tracking-wider">
                {t('dashboard.organization.permissions.config.enableAll')}
              </h3>
              <p className="text-[11px] font-medium text-text-secondary mt-1">
                {isAllEnabled
                  ? t('dashboard.organization.permissions.config.allPermissionsEnabled')
                  : t('dashboard.organization.permissions.config.allPermissionsDisabled')}
              </p>
            </div>
            <Switch
              checked={isAllEnabled}
              onCheckedChange={handleToggleAll}
              disabled={!canEdit}
              className="data-[state=checked]:bg-primary"
            />
          </div>

          {/* Permission Modules */}
          <div className="space-y-8">
            {PERMISSION_MODULES.map(module => (
              <div
                key={module.key}
                className="animate-in overflow-hidden rounded-xl border border-border/80 bg-background shadow-sm fade-in slide-in-from-bottom-2 duration-500"
              >
                <div className="flex items-center justify-between border-b border-border/60 bg-background px-6 py-4">
                  <h3 className="text-sm font-bold text-text-primary uppercase tracking-wider">
                    {t(
                      `dashboard.organization.permissions.config.${module.title}` as Parameters<
                        typeof t
                      >[0]
                    )}
                  </h3>
                  <div className="flex items-center gap-3">
                    <span className="text-[11px] font-bold text-text-placeholder uppercase tracking-tight">
                      {t('dashboard.organization.permissions.config.enableAll')}
                    </span>
                    <Switch
                      checked={isModuleAllEnabled(module)}
                      onCheckedChange={checked => handleToggleModule(module, checked)}
                      disabled={!canEdit}
                      className="data-[state=checked]:bg-primary"
                    />
                  </div>
                </div>
                <div className="grid grid-cols-1 gap-3 bg-bg-canvas/30 p-5 md:grid-cols-2">
                  {module.permissions.map(permission => (
                    <label
                      key={permission.code}
                      htmlFor={permission.code + 'switch'}
                      className={cn(
                        'group flex cursor-pointer items-start justify-between rounded-lg border p-4 transition-colors',
                        !canEdit && 'cursor-not-allowed opacity-70',
                        selectedPermissions.has(permission.code)
                          ? 'border-primary/25 bg-primary/5'
                          : 'border-border bg-background hover:border-border/80 hover:bg-bg-canvas/60'
                      )}
                    >
                      <div className="flex-1 min-w-0 pr-4">
                        <div
                          className={cn(
                            'text-[13px] font-bold mb-1 transition-colors',
                            selectedPermissions.has(permission.code)
                              ? 'text-primary'
                              : 'text-text-primary'
                          )}
                        >
                          {t(
                            `dashboard.organization.permissions.config.${permission.name}` as Parameters<
                              typeof t
                            >[0]
                          )}
                        </div>
                        <div className="text-[11px] text-text-placeholder leading-relaxed line-clamp-3">
                          {t(
                            `dashboard.organization.permissions.config.${permission.description}` as Parameters<
                              typeof t
                            >[0]
                          )}
                        </div>
                      </div>
                      <div className="shrink-0 pt-0.5">
                        <Switch
                          id={permission.code + 'switch'}
                          checked={selectedPermissions.has(permission.code)}
                          onCheckedChange={() => handlePermissionToggle(permission.code)}
                          disabled={!canEdit}
                          className="data-[state=checked]:bg-primary"
                        />
                      </div>
                    </label>
                  ))}
                </div>
              </div>
            ))}
          </div>
        </div>
      </main>

      {/* Edit Role Info Dialog */}
      <EditRoleInfoDialog
        title={
          isNewRole
            ? t('dashboard.organization.permissions.config.newRole')
            : t('dashboard.organization.permissions.config.editRoleInfo')
        }
        open={editDialogOpen}
        onOpenChange={setEditDialogOpen}
        initialName={name || role?.name || ''}
        initialDescription={description || role?.description || ''}
        onSave={handleSaveRoleInfo}
      />

      <ConfirmDialog
        open={leaveConfirmOpen}
        onOpenChange={setLeaveConfirmOpen}
        title={t('dashboard.organization.permissions.config.leaveConfirm.title')}
        description={t('dashboard.organization.permissions.config.leaveConfirm.description')}
        confirmText={t('dashboard.organization.permissions.config.leaveConfirm.confirm')}
        cancelText={t('dashboard.organization.permissions.config.leaveConfirm.cancel')}
        onConfirm={() => router.push('/dashboard/organization/permissions')}
        variant="warning"
      />
    </div>
  );
}
