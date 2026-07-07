'use client';

import { useEffect, useMemo, useState } from 'react';
import {
  AlertTriangle,
  CheckCircle2,
  Layers3,
  Loader2,
  RotateCcw,
  Save,
  Search,
  ShieldCheck,
} from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Switch } from '@/components/ui/switch';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import {
  PERMISSION_MODULES,
  formatPermissionFallbackDescription,
  formatPermissionFallbackLabel,
  getMissingPermissionDependencies,
  normalizeSelectablePermissionCodes,
} from '@/constants/permissions';
import type { PermissionModule } from '@/constants/permissions';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { pickLocale } from '@/utils/tool-helpers';
import {
  isAssignableWorkspaceAdminRole,
  isSelectableWorkspacePermissionTemplate,
} from '@/utils/workspace-role-templates';
import type { Role } from '@/services/types/organization';
import type { WorkspaceMemberAccount } from '@/services/types/workspace';

interface WorkspaceMemberPermissionsDialogProps {
  open: boolean;
  member: WorkspaceMemberAccount | null;
  onOpenChange: (open: boolean) => void;
  onSave: (memberId: string, permissions: string[]) => Promise<void>;
  roleTemplates?: Role[];
  onApplyTemplate?: (memberId: string, roleId: string) => Promise<void>;
  isSaving?: boolean;
  isApplyingTemplate?: boolean;
}

export function WorkspaceMemberPermissionsDialog({
  open,
  member,
  onOpenChange,
  onSave,
  roleTemplates = [],
  onApplyTemplate,
  isSaving = false,
  isApplyingTemplate = false,
}: WorkspaceMemberPermissionsDialogProps) {
  const { locale } = useLocale();
  const t = useT('dashboard.organization.workspaceManagement.detail.memberPermissions');
  const rootT = useT();
  const rootTWithHas = rootT as typeof rootT & { has?: (key: string) => boolean };
  const [selectedPermissions, setSelectedPermissions] = useState<Set<string>>(new Set());
  const [templateDialogOpen, setTemplateDialogOpen] = useState(false);
  const [templateSearch, setTemplateSearch] = useState('');
  const [pendingDependencyChange, setPendingDependencyChange] = useState<{
    label: string;
    permissionCodes: string[];
    dependencies: string[];
  } | null>(null);

  const originalPermissions = useMemo(
    () => new Set(normalizeSelectablePermissionCodes(member?.permissions)),
    [member?.permissions]
  );
  const permissionModules = useMemo(
    () =>
      PERMISSION_MODULES.map(module => ({
        ...module,
        permissions: module.permissions.filter(
          permission => normalizeSelectablePermissionCodes([permission.code]).length > 0
        ),
      })).filter(module => module.permissions.length > 0),
    []
  );
  const allPermissionCodes = useMemo(
    () =>
      permissionModules.flatMap(module => module.permissions.map(permission => permission.code)),
    [permissionModules]
  );
  const workspaceAdminRole = useMemo(
    () => roleTemplates.find(isAssignableWorkspaceAdminRole),
    [roleTemplates]
  );
  const permissionTemplateRoles = useMemo(
    () => roleTemplates.filter(isSelectableWorkspacePermissionTemplate),
    [roleTemplates]
  );
  const getRoleDisplayName = (role: Role) =>
    role.name_i18n ? pickLocale(role.name_i18n, locale, role.name) : role.name;
  const getRoleDescription = (role: Role) =>
    role.description_i18n
      ? pickLocale(role.description_i18n, locale, role.description || '')
      : role.description || '';
  const isOwner = member?.role === 'owner' || member?.permission_source === 'owner';
  const isAdmin = member?.role === 'admin';
  const canChangeRole = Boolean(member) && !isOwner && Boolean(onApplyTemplate);
  const canEditDirectPermissions = Boolean(member) && !isOwner && !isAdmin;
  const canToggleAdmin = canChangeRole && Boolean(workspaceAdminRole);
  const currentTemplateId =
    member?.permission_source === 'direct'
      ? ''
      : member?.permission_template_role_id || member?.role_id || '';
  const currentTemplate = useMemo(
    () => roleTemplates.find(role => role.id === currentTemplateId),
    [currentTemplateId, roleTemplates]
  );
  const currentTemplateSelectable = useMemo(
    () => permissionTemplateRoles.some(role => role.id === currentTemplateId),
    [permissionTemplateRoles, currentTemplateId]
  );
  const currentTemplateName = currentTemplate
    ? getRoleDisplayName(currentTemplate)
    : member?.role_name || (member?.permission_source === 'direct' ? t('source.direct') : '');
  const currentTemplateDescription =
    member?.permission_source === 'direct'
      ? t('template.description')
      : currentTemplate && currentTemplateSelectable
        ? getRoleDescription(currentTemplate)
        : t('template.currentMissingDescription');
  const shouldWarnCurrentTemplate =
    Boolean(member) &&
    !isOwner &&
    !isAdmin &&
    member?.permission_source !== 'direct' &&
    Boolean(currentTemplateId || member?.role_name) &&
    (!currentTemplate || !currentTemplateSelectable || member?.permission_source === 'legacy_role');
  const normalizedTemplateSearch = templateSearch.trim().toLowerCase();
  const filteredTemplates = normalizedTemplateSearch
    ? permissionTemplateRoles.filter(role => {
        const name = getRoleDisplayName(role).toLowerCase();
        const description = getRoleDescription(role).toLowerCase();
        return (
          name.includes(normalizedTemplateSearch) || description.includes(normalizedTemplateSearch)
        );
      })
    : permissionTemplateRoles;
  const isBusy = isSaving || isApplyingTemplate;
  const sourceLabel = isOwner
    ? t('source.owner')
    : isAdmin
      ? member?.role_name || t('source.admin')
      : member?.permission_source === 'direct'
        ? t('source.direct')
        : member?.permission_source === 'legacy_role'
          ? t('source.legacy')
          : t('source.template');
  const selectedCount = selectedPermissions.size;
  const hasChanges = useMemo(() => {
    if (selectedPermissions.size !== originalPermissions.size) return true;
    for (const permission of selectedPermissions) {
      if (!originalPermissions.has(permission)) return true;
    }
    return false;
  }, [originalPermissions, selectedPermissions]);

  useEffect(() => {
    if (!open) {
      setPendingDependencyChange(null);
      return;
    }
    setSelectedPermissions(new Set(originalPermissions));
    setTemplateDialogOpen(false);
    setTemplateSearch('');
  }, [member, open, originalPermissions]);

  const translatePermission = (key: string, fallback: string) => {
    const fullKey = `dashboard.organization.permissions.config.${key}`;
    if (typeof rootTWithHas.has === 'function' && !rootTWithHas.has(fullKey)) {
      return fallback;
    }
    return rootT(fullKey as Parameters<typeof rootT>[0]);
  };

  const getPermissionLabel = (code: string) =>
    translatePermission(`permissions.${code}.name`, formatPermissionFallbackLabel(code, locale));

  const getModuleLabel = (module: PermissionModule) =>
    translatePermission(module.title, formatPermissionFallbackLabel(module.key, locale));

  const dependencySeparator = locale?.toLowerCase().startsWith('zh') ? '、' : ', ';
  const dependencyConfirmDescription = pendingDependencyChange
    ? t('dependencyConfirm.description', {
        permission: pendingDependencyChange.label,
        dependencies: pendingDependencyChange.dependencies
          .map(getPermissionLabel)
          .join(dependencySeparator),
      })
    : '';

  const addPermissionsWithDependencies = (
    permissionCodes: readonly string[],
    dependencies: readonly string[] = []
  ) => {
    setSelectedPermissions(prev => {
      const next = new Set(prev);
      permissionCodes.forEach(permissionCode => next.add(permissionCode));
      dependencies.forEach(dependency => next.add(dependency));
      return next;
    });
  };

  const togglePermission = (permissionCode: string) => {
    if (selectedPermissions.has(permissionCode)) {
      setSelectedPermissions(prev => {
        const next = new Set(prev);
        next.delete(permissionCode);
        return next;
      });
      return;
    }

    const dependencies = getMissingPermissionDependencies(selectedPermissions, [permissionCode]);
    if (dependencies.length > 0) {
      setPendingDependencyChange({
        label: getPermissionLabel(permissionCode),
        permissionCodes: [permissionCode],
        dependencies,
      });
      return;
    }

    addPermissionsWithDependencies([permissionCode]);
  };

  const confirmDependencyChange = () => {
    if (!pendingDependencyChange) return;
    addPermissionsWithDependencies(
      pendingDependencyChange.permissionCodes,
      pendingDependencyChange.dependencies
    );
    setPendingDependencyChange(null);
  };

  const toggleModule = (module: PermissionModule, checked: boolean) => {
    if (!checked) {
      setSelectedPermissions(prev => {
        const next = new Set(prev);
        for (const permission of module.permissions) {
          next.delete(permission.code);
        }
        return next;
      });
      return;
    }

    const permissionCodes = module.permissions
      .map(permission => permission.code)
      .filter(permissionCode => !selectedPermissions.has(permissionCode));
    if (permissionCodes.length === 0) return;

    const dependencies = getMissingPermissionDependencies(selectedPermissions, permissionCodes);
    if (dependencies.length > 0) {
      setPendingDependencyChange({
        label: getModuleLabel(module),
        permissionCodes,
        dependencies,
      });
      return;
    }

    addPermissionsWithDependencies(permissionCodes);
  };

  const moduleChecked = (module: PermissionModule) =>
    module.permissions.every(permission => selectedPermissions.has(permission.code));

  const handleSave = async () => {
    if (!member || !canEditDirectPermissions) return;
    await onSave(member.id, normalizeSelectablePermissionCodes(Array.from(selectedPermissions)));
  };

  const handleApplyRoleOrTemplate = async (role: Role) => {
    if (!member || !canChangeRole || !role.id || !onApplyTemplate) return;
    await onApplyTemplate(member.id, role.id);
    setSelectedPermissions(new Set(normalizeSelectablePermissionCodes(role.permissions)));
    setTemplateDialogOpen(false);
    setTemplateSearch('');
  };

  const handleAdminSwitchChange = async (checked: boolean) => {
    if (!checked) {
      setTemplateDialogOpen(true);
      return;
    }
    if (!workspaceAdminRole) return;
    await handleApplyRoleOrTemplate(workspaceAdminRole);
  };

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent size="xl" className="h-[92vh] max-w-[min(1280px,calc(100vw-3rem))]">
          <DialogHeader className="px-8 pb-3">
            <DialogTitle className="flex items-center gap-2">
              <ShieldCheck className="h-5 w-5 text-primary" />
              {t('title')}
            </DialogTitle>
            <DialogDescription>
              {member
                ? t('description', { name: member.member_name || member.name })
                : t('emptyDescription')}
            </DialogDescription>
          </DialogHeader>

          <DialogBody className="flex min-h-0 flex-col gap-4 overflow-hidden px-8 py-2">
            <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border bg-muted/30 px-4 py-3">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-sm font-semibold">
                    {member?.member_name || member?.name}
                  </span>
                  <Badge variant={isOwner ? 'default' : 'secondary'} className="rounded-md">
                    {sourceLabel}
                  </Badge>
                </div>
                <p className="mt-1 text-xs text-muted-foreground">
                  {!canEditDirectPermissions
                    ? isOwner
                      ? t('ownerHint')
                      : t('governanceHint')
                    : t('selectedCount', { count: selectedCount })}
                </p>
              </div>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={!canEditDirectPermissions || isBusy}
                  onClick={() => setSelectedPermissions(new Set(originalPermissions))}
                >
                  <RotateCcw className="mr-1.5 h-3.5 w-3.5" />
                  {t('reset')}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={!canEditDirectPermissions || isBusy}
                  onClick={() => setSelectedPermissions(new Set(allPermissionCodes))}
                >
                  {t('selectAll')}
                </Button>
              </div>
            </div>

            {canChangeRole ? (
              <div className="rounded-lg border bg-background px-4 py-3">
                <div className="flex flex-col gap-3">
                  <div className="flex items-center justify-between gap-4 rounded-lg border bg-muted/20 px-3 py-2">
                    <div className="min-w-0">
                      <div className="text-sm font-semibold text-foreground">
                        {t('adminToggle.title')}
                      </div>
                      <p className="mt-0.5 text-xs leading-5 text-muted-foreground">
                        {workspaceAdminRole
                          ? t('adminToggle.description')
                          : t('adminToggle.missing')}
                      </p>
                    </div>
                    <Switch
                      checked={isAdmin}
                      onCheckedChange={value => void handleAdminSwitchChange(value)}
                      disabled={!canToggleAdmin || isBusy}
                    />
                  </div>

                  <Dialog open={templateDialogOpen} onOpenChange={setTemplateDialogOpen}>
                    {isAdmin ? (
                      <div className="flex flex-col gap-3 rounded-lg border bg-primary/5 px-3 py-3 sm:flex-row sm:items-center sm:justify-between">
                        <div className="min-w-0">
                          <div className="text-sm font-semibold text-foreground">
                            {t('adminToggle.currentAdminTitle')}
                          </div>
                          <p className="mt-1 text-xs leading-5 text-muted-foreground">
                            {t('adminToggle.currentAdminDescription')}
                          </p>
                        </div>
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={isBusy}
                          onClick={() => setTemplateDialogOpen(true)}
                        >
                          {t('adminToggle.demote')}
                        </Button>
                      </div>
                    ) : (
                      <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
                        <div className="flex min-w-0 items-start gap-3">
                          <div className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
                            <Layers3 className="h-4 w-4" />
                          </div>
                          <div className="min-w-0">
                            <div className="text-sm font-semibold text-foreground">
                              {t('template.title')}
                            </div>
                            <p className="mt-1 text-xs leading-5 text-muted-foreground">
                              {t('template.description')}
                            </p>
                          </div>
                        </div>
                        <button
                          type="button"
                          disabled={isBusy}
                          onClick={() => setTemplateDialogOpen(true)}
                          className={cn(
                            'flex w-full min-w-0 items-center justify-between gap-3 rounded-lg border bg-muted/20 px-3 py-2 text-left transition-colors hover:border-primary/40 hover:bg-primary/5 lg:w-[360px]',
                            isBusy &&
                              'cursor-not-allowed opacity-70 hover:border-border hover:bg-muted/20'
                          )}
                        >
                          <span className="min-w-0">
                            <span className="block text-[11px] font-medium text-muted-foreground">
                              {t('template.currentLabel')}
                            </span>
                            <span className="mt-0.5 flex min-w-0 items-center gap-1.5">
                              <span className="truncate text-sm font-semibold text-foreground">
                                {currentTemplateName || t('template.placeholder')}
                              </span>
                              {shouldWarnCurrentTemplate ? (
                                <Tooltip>
                                  <TooltipTrigger asChild>
                                    <span className="inline-flex shrink-0 text-destructive">
                                      <AlertTriangle className="h-3.5 w-3.5" />
                                    </span>
                                  </TooltipTrigger>
                                  <TooltipContent variant="destructive">
                                    {t('template.deprecatedTooltip')}
                                  </TooltipContent>
                                </Tooltip>
                              ) : null}
                            </span>
                            <span className="mt-1 line-clamp-2 text-xs leading-4 text-muted-foreground">
                              {currentTemplateDescription || t('template.description')}
                            </span>
                          </span>
                          <span className="shrink-0 rounded-md border bg-background px-2.5 py-1 text-xs font-medium text-foreground">
                            {isApplyingTemplate ? (
                              <Loader2 className="h-3.5 w-3.5 animate-spin" />
                            ) : (
                              t('template.change')
                            )}
                          </span>
                        </button>
                      </div>
                    )}

                    <DialogContent size="lg" className="max-h-[78vh]">
                      <DialogHeader>
                        <DialogTitle>{t('template.chooseTitle')}</DialogTitle>
                        <DialogDescription>{t('template.chooseDescription')}</DialogDescription>
                      </DialogHeader>
                      <DialogBody className="space-y-3">
                        <Input
                          value={templateSearch}
                          onChange={event => setTemplateSearch(event.target.value)}
                          placeholder={t('template.searchPlaceholder')}
                          leftIcon={<Search className="h-4 w-4" />}
                        />
                        <ScrollArea className="h-[420px] pr-3">
                          <div className="space-y-2">
                            {filteredTemplates.length > 0 ? (
                              filteredTemplates.map(role => {
                                const isCurrent = currentTemplateId === role.id;
                                return (
                                  <button
                                    key={role.id}
                                    type="button"
                                    disabled={isBusy || isCurrent}
                                    onClick={() => void handleApplyRoleOrTemplate(role)}
                                    className={cn(
                                      'flex w-full items-start justify-between gap-3 rounded-lg border p-3 text-left transition-colors hover:border-primary/40 hover:bg-primary/5',
                                      isCurrent
                                        ? 'border-primary/30 bg-primary/5'
                                        : 'border-border bg-background',
                                      (isBusy || isCurrent) && 'cursor-default'
                                    )}
                                  >
                                    <span className="min-w-0">
                                      <span className="flex items-center gap-2 text-sm font-semibold text-foreground">
                                        <span className="truncate">{getRoleDisplayName(role)}</span>
                                        {isCurrent ? (
                                          <Badge
                                            variant="secondary"
                                            className="shrink-0 rounded-md"
                                          >
                                            {t('template.current')}
                                          </Badge>
                                        ) : null}
                                      </span>
                                      {getRoleDescription(role) ? (
                                        <span className="mt-1 line-clamp-2 text-xs leading-4 text-muted-foreground">
                                          {getRoleDescription(role)}
                                        </span>
                                      ) : null}
                                    </span>
                                    {isCurrent ? (
                                      <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-primary" />
                                    ) : null}
                                  </button>
                                );
                              })
                            ) : (
                              <div className="rounded-lg border border-dashed py-10 text-center text-sm text-muted-foreground">
                                {t('template.empty')}
                              </div>
                            )}
                          </div>
                        </ScrollArea>
                      </DialogBody>
                      <DialogFooter>
                        <Button
                          variant="outline"
                          onClick={() => setTemplateDialogOpen(false)}
                          disabled={isBusy}
                        >
                          {t('cancel')}
                        </Button>
                      </DialogFooter>
                    </DialogContent>
                  </Dialog>
                </div>
                <p className="mt-2 text-[11px] leading-4 text-muted-foreground">
                  {t('template.hint')}
                </p>
              </div>
            ) : null}

            {isAdmin ? (
              <div className="flex min-h-0 flex-1 items-center justify-center rounded-lg border border-dashed bg-muted/20 px-8 py-10 text-center">
                <div className="max-w-md">
                  <div className="mx-auto flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10 text-primary">
                    <ShieldCheck className="h-5 w-5" />
                  </div>
                  <h3 className="mt-4 text-sm font-semibold text-foreground">
                    {t('adminPermissionPlaceholder.title')}
                  </h3>
                  <p className="mt-2 text-sm leading-6 text-muted-foreground">
                    {t('adminPermissionPlaceholder.description')}
                  </p>
                </div>
              </div>
            ) : (
              <ScrollArea className="min-h-0 flex-1 pr-3">
                <div className="space-y-4">
                  {permissionModules.map(module => {
                    const checked = moduleChecked(module);
                    return (
                      <section key={module.key} className="rounded-lg border bg-background">
                        <div className="flex items-center justify-between gap-3 border-b px-4 py-3">
                          <div>
                            <h3 className="text-sm font-semibold">
                              {translatePermission(
                                module.title,
                                formatPermissionFallbackLabel(module.key, locale)
                              )}
                            </h3>
                            <p className="mt-0.5 text-xs text-muted-foreground">
                              {t('moduleCount', { count: module.permissions.length })}
                            </p>
                          </div>
                          <label className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
                            <Checkbox
                              checked={checked}
                              disabled={!canEditDirectPermissions || isBusy}
                              onCheckedChange={value => toggleModule(module, Boolean(value))}
                            />
                            {t('moduleAll')}
                          </label>
                        </div>
                        <div className="grid grid-cols-1 gap-2 p-3 md:grid-cols-2 xl:grid-cols-3">
                          {module.permissions.map(permission => {
                            const permissionChecked = selectedPermissions.has(permission.code);
                            return (
                              <label
                                key={permission.code}
                                className={cn(
                                  'flex cursor-pointer items-start gap-3 rounded-md border p-3 transition-colors',
                                  permissionChecked
                                    ? 'border-primary/30 bg-primary/5'
                                    : 'border-border bg-background hover:bg-muted/40',
                                  (!canEditDirectPermissions || isBusy) &&
                                    'cursor-not-allowed opacity-70'
                                )}
                              >
                                <Checkbox
                                  checked={permissionChecked}
                                  disabled={!canEditDirectPermissions || isBusy}
                                  onCheckedChange={() => togglePermission(permission.code)}
                                  className="mt-0.5"
                                />
                                <span className="min-w-0">
                                  <span className="block text-xs font-semibold text-foreground">
                                    {translatePermission(
                                      permission.name,
                                      formatPermissionFallbackLabel(permission.code, locale)
                                    )}
                                  </span>
                                  <span className="mt-1 block break-words text-[11px] leading-4 text-muted-foreground">
                                    {translatePermission(
                                      permission.description,
                                      formatPermissionFallbackDescription(permission.code, locale)
                                    )}
                                  </span>
                                </span>
                              </label>
                            );
                          })}
                        </div>
                      </section>
                    );
                  })}
                </div>
              </ScrollArea>
            )}
          </DialogBody>

          <DialogFooter className="border-t bg-background/95 px-8 py-4">
            <Button variant="outline" onClick={() => onOpenChange(false)} disabled={isBusy}>
              {t('cancel')}
            </Button>
            <Button
              onClick={handleSave}
              disabled={!canEditDirectPermissions || isBusy || !hasChanges}
            >
              <Save className="mr-1.5 h-4 w-4" />
              {isSaving ? t('saving') : t('save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <ConfirmDialog
        open={Boolean(pendingDependencyChange)}
        onOpenChange={nextOpen => {
          if (!nextOpen) setPendingDependencyChange(null);
        }}
        title={t('dependencyConfirm.title')}
        description={dependencyConfirmDescription}
        confirmText={t('dependencyConfirm.confirm')}
        cancelText={t('dependencyConfirm.cancel')}
        onConfirm={confirmDependencyChange}
      />
    </>
  );
}
