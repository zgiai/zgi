'use client';

import { useMemo, useState } from 'react';
import { useT } from '@/i18n';
import { useRouter } from 'next/navigation';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Shield, Plus, Ellipsis, Users, AlertTriangle } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useOrganizationRoles } from '@/hooks/organization/use-organization-roles';
import { Skeleton } from '@/components/ui/skeleton';
import { useLocale } from '@/hooks/use-locale';
import { pickLocale } from '@/utils/tool-helpers';
import { RoleMembersDrawer } from '@/components/dashboard/organization/role-members-drawer';
import { useRoleActions } from '@/hooks/organization/use-role-actions';
import { EditRoleInfoDialog } from '@/components/dashboard/organization/edit-role-info-dialog';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { toast } from 'sonner';
import type { Role } from '@/services/types/organization';
import {
  isSelectableWorkspacePermissionTemplate,
} from '@/utils/workspace-role-templates';

export default function PermissionsPage() {
  const t = useT('dashboard.organization.permissions');
  const tCommon = useT('common');
  const router = useRouter();
  const { roles, isLoading } = useOrganizationRoles();
  const { locale } = useLocale();
  const {
    deleteRole,
    isDeleting,
    updateRoleInfo,
    isUpdatingInfo,
    replaceAndDeleteRole,
    isReplacingAndDeleting,
  } = useRoleActions();
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [selectedRole, setSelectedRole] = useState<Role | null>(null);
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false);
  const [roleToDelete, setRoleToDelete] = useState<Role | null>(null);
  const [replacementRoleId, setReplacementRoleId] = useState('');
  const [isMigratingBeforeDelete, setIsMigratingBeforeDelete] = useState(false);
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [roleToEdit, setRoleToEdit] = useState<Role | null>(null);

  const handleViewMembers = (roleId: string) => {
    const role = roles.find(r => r.id === roleId);
    if (role) {
      setSelectedRole(role);
      setDrawerOpen(true);
    }
  };

  const handleConfigurePermissions = (roleId: string) => {
    router.push(`/dashboard/organization/permissions/${roleId}`);
  };

  const handleCreateScheme = () => {
    router.push('/dashboard/organization/permissions/new');
  };

  const handleDeleteClick = (role: Role) => {
    if (permissionTemplateRoles.length <= 1) {
      toast.warning(t('deleteConfirm.keepOneToast'));
      return;
    }
    setRoleToDelete(role);
    setReplacementRoleId(permissionTemplateRoles.find(item => item.id !== role.id)?.id ?? '');
    setDeleteConfirmOpen(true);
  };

  const resetDeleteDialog = () => {
    setDeleteConfirmOpen(false);
    setRoleToDelete(null);
    setReplacementRoleId('');
    setIsMigratingBeforeDelete(false);
  };

  const handleConfirmDelete = async () => {
    if (!roleToDelete) {
      return;
    }

    const roleMemberCount = roleToDelete.member_count ?? 0;

    if (roleMemberCount > 0) {
      try {
        if (!replacementRoleId) {
          toast.warning(t('deleteConfirm.replacementRequired'));
          return;
        }
        setIsMigratingBeforeDelete(true);
        const result = await replaceAndDeleteRole({
          roleId: roleToDelete.id,
          data: {
            replacement_role_id: replacementRoleId,
          },
        });

        if (result.failed_count > 0 || !result.deleted) {
          return;
        }
      } catch {
        return;
      } finally {
        setIsMigratingBeforeDelete(false);
      }
      resetDeleteDialog();
      return;
    }

    await deleteRole(roleToDelete.id);
    resetDeleteDialog();
  };

  const handleEditClick = (role: Role) => {
    setRoleToEdit(role);
    setEditDialogOpen(true);
  };

  const handleSaveRoleInfo = async (name: string, description: string) => {
    if (roleToEdit) {
      await updateRoleInfo({
        roleId: roleToEdit.id,
        data: {
          name,
          description,
        },
      });
    }
  };

  const getRoleDisplayName = (role: Role) =>
    role.name_i18n ? pickLocale(role.name_i18n, locale, role.name) : role.name;

  const getRoleDescription = (role: Role) =>
    role.description_i18n
      ? pickLocale(role.description_i18n, locale, role.description || '')
      : role.description || '';

  const permissionTemplateRoles = roles.filter(isSelectableWorkspacePermissionTemplate);
  const replacementRoleOptions = useMemo(
    () => permissionTemplateRoles.filter(role => role.id !== roleToDelete?.id),
    [permissionTemplateRoles, roleToDelete?.id]
  );
  const deleteRequiresMigration = (roleToDelete?.member_count ?? 0) > 0;
  const deleteLoading = isDeleting || isMigratingBeforeDelete || isReplacingAndDeleting;
  const canApplySelectedRoleTemplate =
    !!selectedRole && isSelectableWorkspacePermissionTemplate(selectedRole);

  return (
    <div className="h-full space-y-5 overflow-y-auto bg-bg-canvas/50 p-4 lg:p-6">
      {/* Header */}
      <div className="flex flex-col items-start justify-between gap-4 sm:flex-row sm:items-center">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight text-text-primary">{t('title')}</h1>
          <p className="mt-1 max-w-2xl text-sm text-text-secondary">{t('subtitle')}</p>
        </div>
        <Button
          onClick={handleCreateScheme}
          className="h-10 rounded-md bg-primary px-4 font-medium text-primary-foreground shadow-sm transition-colors hover:bg-primary-hover hover:text-primary-foreground"
        >
          <Plus className="mr-2 h-4 w-4" />
          {t('newRoleScheme.title')}
        </Button>
      </div>

      {/* Role Cards Grid */}
      <section className="space-y-3">
        <div>
          <h2 className="text-sm font-semibold text-text-primary">
            {t('sections.templateTitle')}
          </h2>
          <p className="mt-1 text-xs text-text-secondary">{t('sections.templateSubtitle')}</p>
        </div>
      <div className="mx-auto grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
        {/* Loading Skeleton */}
        {isLoading
          ? Array.from({ length: 3 }).map((_, index) => (
              <Card key={`skeleton-${index}`} className="rounded-xl border-border/80 bg-background">
                <CardContent className="px-5 pb-5 pt-6">
                  <div className="mb-5 flex items-center gap-3">
                    <Skeleton className="w-12 h-12 rounded-xl" />
                    <Skeleton className="h-6 w-28 rounded-lg" />
                  </div>
                  <Skeleton className="h-4 w-full mb-2" />
                  <Skeleton className="h-4 w-5/6 mb-6 min-h-[3rem]" />
                  <div className="flex items-center gap-2 mb-5">
                    <Skeleton className="h-3.5 w-3.5" />
                    <Skeleton className="h-3.5 w-20" />
                  </div>
                  <div className="flex gap-2">
                    <Skeleton className="h-9 flex-1 rounded-lg" />
                    <Skeleton className="h-9 flex-1 rounded-lg" />
                  </div>
                </CardContent>
              </Card>
            ))
          : /* Existing Role Cards */
            permissionTemplateRoles.map(role => (
              <Card
                key={role.id}
                className={cn(
                  'group relative overflow-hidden rounded-xl border-border/80 bg-background shadow-sm transition-colors hover:border-primary/30',
                  role.id === 'admin' ? 'ring-1 ring-primary/10' : ''
                )}
              >
                {/* Visual Decoration for Admin/Owner */}
                {role.id === 'admin' && (
                  <div className="absolute top-0 right-0 w-20 h-20 bg-primary/5 blur-2xl rounded-full -mr-6 -mt-6" />
                )}

                <CardContent className="relative z-10 px-5 pb-5 pt-6">
                  {/* More Options Menu */}
                  {role.editable && (
                    <div className="absolute top-3 right-3 opacity-0 group-hover:opacity-100 transition-opacity">
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button
                            variant="ghost"
                            isIcon
                            className="h-7 w-7 rounded-md hover:bg-accent/80"
                          >
                            <Ellipsis className="h-3.5 w-3.5" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end" className="min-w-[110px]">
                          <DropdownMenuItem
                            onClick={() => handleEditClick(role)}
                            className="text-xs cursor-pointer hover:bg-accent px-2.5 py-1.5"
                          >
                            {tCommon('edit')}
                          </DropdownMenuItem>
                          {role.deletable !== false ? (
                            <DropdownMenuItem
                              onClick={() => handleDeleteClick(role)}
                              className="text-xs text-destructive focus:text-destructive cursor-pointer hover:bg-destructive/5 px-2.5 py-1.5"
                            >
                              {tCommon('delete')}
                            </DropdownMenuItem>
                          ) : null}
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </div>
                  )}

                  {/* Role Icon & Name */}
                  <div className="mb-5 flex items-center gap-3 w-full">
                    <div
                      className={cn(
                        'flex h-11 w-11 shrink-0 items-center justify-center rounded-lg shadow-sm',
                        role.id === 'admin'
                          ? 'bg-primary text-primary-foreground'
                          : 'bg-card text-brand-main border'
                      )}
                    >
                      <Shield className="h-6 w-6" />
                    </div>
                    <div className="flex w-0 grow items-center gap-2">
                      <h3 className="text-lg font-bold text-text-primary tracking-tight w-0 grow line-clamp-2 break-words text-ellipsis">
                        {getRoleDisplayName(role)}
                      </h3>
                      {role.builtin && (
                        <span className="text-[9px] px-1.5 py-0.5 rounded-full bg-muted text-text-placeholder font-bold uppercase tracking-wider shrink-0">
                          System
                        </span>
                      )}
                    </div>
                  </div>

                  {/* Role Description */}
                  <p className="text-sm text-text-secondary mb-5 min-h-[3.5rem] leading-relaxed line-clamp-3 break-words">
                    {getRoleDescription(role)}
                  </p>

                  {/* Member Count */}
                  <div className="flex items-center gap-2 text-xs font-bold text-text-secondary mb-5 px-2.5 py-1.5 rounded-lg border border-border/20 w-fit">
                    <Users className="h-3 w-3 text-primary/60" />
                    <span>
                      {(role.member_count ?? 0) > 0
                        ? t('memberCount.people', {
                            count: role.member_count ?? 0,
                          })
                        : t('memberCount.noMembers')}
                    </span>
                  </div>

                  {/* Action Buttons */}
                  <div className="flex gap-2.5 mt-auto">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => handleViewMembers(role.id)}
                      className="h-9 flex-1 rounded-md text-xs font-semibold transition-colors"
                    >
                      {t('actions.viewMembers')}
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => handleConfigurePermissions(role.id)}
                      className="h-9 flex-1 rounded-md border-primary/20 text-xs font-semibold transition-colors hover:border-primary/40 hover:bg-primary/5 hover:text-primary"
                    >
                      {t('actions.configurePermissions')}
                    </Button>
                  </div>
                </CardContent>
              </Card>
            ))}
      </div>
      </section>

      {/* Modals remain functional */}
      <RoleMembersDrawer
        open={drawerOpen}
        onOpenChange={setDrawerOpen}
        roleId={selectedRole?.id ?? null}
        roleName={selectedRole ? getRoleDisplayName(selectedRole) : ''}
        canApplyTemplate={canApplySelectedRoleTemplate}
      />

      <ConfirmDialog
        open={deleteConfirmOpen && !deleteRequiresMigration}
        onOpenChange={open => {
          if (!open) {
            resetDeleteDialog();
          } else {
            setDeleteConfirmOpen(true);
          }
        }}
        title={t('deleteConfirm.title')}
        description={t('deleteConfirm.description', {
          roleName: roleToDelete ? getRoleDisplayName(roleToDelete) : '',
        })}
        confirmText={t('deleteConfirm.confirm')}
        cancelText={t('deleteConfirm.cancel')}
        loading={deleteLoading}
        onConfirm={handleConfirmDelete}
        variant="danger"
      />

      <Dialog
        open={deleteConfirmOpen && deleteRequiresMigration}
        onOpenChange={open => {
          if (!open) {
            resetDeleteDialog();
          } else {
            setDeleteConfirmOpen(true);
          }
        }}
      >
        <DialogContent size="md" className="p-0">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-xl font-bold tracking-tight">
              <AlertTriangle className="h-5 w-5 text-destructive" />
              {t('deleteConfirm.migrationTitle')}
            </DialogTitle>
            <DialogDescription className="text-sm text-muted-foreground">
              {t('deleteConfirm.migrationDescription', {
                roleName: roleToDelete ? getRoleDisplayName(roleToDelete) : '',
                count: roleToDelete?.member_count ?? 0,
              })}
            </DialogDescription>
          </DialogHeader>
          <DialogBody className="space-y-4">
            <div className="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900">
              {t('deleteConfirm.migrationWarning')}
            </div>
            <div className="space-y-2">
              <label className="text-sm font-semibold text-text-primary">
                {t('deleteConfirm.replacementLabel')}
              </label>
              <Select value={replacementRoleId} onValueChange={setReplacementRoleId}>
                <SelectTrigger className="h-10 w-full bg-background">
                  <SelectValue placeholder={t('deleteConfirm.replacementPlaceholder')} />
                </SelectTrigger>
                <SelectContent>
                  {replacementRoleOptions.map(role => (
                    <SelectItem key={role.id} value={role.id} textValue={getRoleDisplayName(role)}>
                      {getRoleDisplayName(role)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </DialogBody>
          <DialogFooter className="border-t bg-muted/50 px-6 pb-6 pt-4">
            <Button variant="ghost" onClick={resetDeleteDialog} disabled={deleteLoading}>
              {t('deleteConfirm.cancel')}
            </Button>
            <Button
              variant="destructive"
              onClick={handleConfirmDelete}
              disabled={!replacementRoleId || deleteLoading}
            >
              {deleteLoading ? t('deleteConfirm.migrating') : t('deleteConfirm.migrateAndDelete')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {roleToEdit && (
        <EditRoleInfoDialog
          title={t('config.editRoleInfo')}
          open={editDialogOpen}
          onOpenChange={open => {
            setEditDialogOpen(open);
            if (!open) {
              setRoleToEdit(null);
            }
          }}
          initialName={roleToEdit.name}
          initialDescription={roleToEdit.description || ''}
          onSave={handleSaveRoleInfo}
          isLoading={isUpdatingInfo}
        />
      )}
    </div>
  );
}
