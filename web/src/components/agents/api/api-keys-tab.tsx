'use client';

import React, { useMemo, useState } from 'react';
import { Plus, MoreHorizontal, Copy, Edit, Trash2, KeyRound, Search, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';
import { Switch } from '@/components/ui/switch';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogBody,
  DialogDescription,
} from '@/components/ui/dialog';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { toast } from 'sonner';
import { formatDate } from '@/utils/format';
import {
  useAgentApiKeys,
  useCreateAgentApiKey,
  useUpdateAgentApiKey,
  useDeleteAgentApiKey,
} from '@/hooks/agent/use-agent-api-keys';
import { useT } from '@/i18n';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { agentService } from '@/services';
import { Badge } from '@/components/ui/badge';

interface ApiKeysTabProps {
  agentId: string;
}

export default function ApiKeysTab({ agentId }: ApiKeysTabProps) {
  const t = useT();

  const { keys, isLoading, isFetching, refetch } = useAgentApiKeys(agentId);

  // Search/filter
  const [search, setSearch] = useState('');
  const filteredKeys = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return keys;
    return keys.filter(
      k =>
        (k.name || '').toLowerCase().includes(q) || (k.key_prefix || '').toLowerCase().includes(q)
    );
  }, [keys, search]);
  const hasSearch = search.trim().length > 0;

  // Create dialog state
  const [formOpen, setFormOpen] = useState(false);
  const [isEdit, setIsEdit] = useState(false);
  const [secretOpen, setSecretOpen] = useState(false);
  const [createdSecretKey, setCreatedSecretKey] = useState<string>('');
  const [newName, setNewName] = useState('');
  const [newExpiresAt, setNewExpiresAt] = useState<string>('');
  const [hasExpiry, setHasExpiry] = useState(false);
  const [togglingId, setTogglingId] = useState<string | null>(null);

  const createMutation = useCreateAgentApiKey(agentId);

  const handleSubmit = async () => {
    if (!newName.trim()) {
      toast.error(t('agents.apiKeys.validation.missingName'), { description: t('common.error') });
      return;
    }
    if (isEdit) {
      if (!editingId) return;
      const payload = { name: newName.trim() };
      await updateMutation.mutateAsync(payload);
      setFormOpen(false);
      setEditingId(null);
      setNewName('');
      setNewExpiresAt('');
      setHasExpiry(false);
      return;
    }
    const now = new Date();
    let expiresISO: string | undefined = undefined;
    if (hasExpiry) {
      if (!newExpiresAt) {
        toast.error(t('agents.apiKeys.validation.missingExpiry'), {
          description: t('common.error'),
        });
        return;
      }
      const exp = new Date(newExpiresAt);
      if (isNaN(exp.getTime()) || exp <= now) {
        toast.error(t('agents.apiKeys.validation.invalidExpiry'), {
          description: t('common.error'),
        });
        return;
      }
      expiresISO = exp.toISOString();
    }
    const payload = {
      name: newName.trim(),
      expires_at: expiresISO,
    };
    const res = await createMutation.mutateAsync(payload);
    setFormOpen(false);
    const apiKeyVal = res?.data?.api_key || '';
    if (apiKeyVal) {
      setCreatedSecretKey(apiKeyVal);
      setSecretOpen(true);
    } else {
      toast.error(t('agents.apiKeys.errors.missingSecret'), { description: t('common.error') });
    }
    setNewName('');
    setNewExpiresAt('');
    setHasExpiry(false);
  };

  const [editingId, setEditingId] = useState<string | null>(null);
  const updateMutation = useUpdateAgentApiKey(agentId, editingId || '');

  const openEdit = (id: string) => {
    const target = keys.find(k => k.id === id);
    setIsEdit(true);
    setEditingId(id);
    setNewName(target?.name || '');
    if (target?.expires_at) {
      const d = new Date(target.expires_at);
      const local = new Date(d.getTime() - d.getTimezoneOffset() * 60000)
        .toISOString()
        .slice(0, 16);
      setHasExpiry(true);
      setNewExpiresAt(local);
    } else {
      setHasExpiry(false);
      setNewExpiresAt('');
    }
    setFormOpen(true);
  };

  const openCreate = () => {
    setIsEdit(false);
    setEditingId(null);
    setNewName('');
    setHasExpiry(false);
    setNewExpiresAt('');
    setFormOpen(true);
  };

  // Delete key
  const deleteMutation = useDeleteAgentApiKey(agentId);
  const handleDelete = async (id: string) => {
    await deleteMutation.mutateAsync(id);
  };

  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleteId, setDeleteId] = useState<string | null>(null);

  // Toggle status
  const handleToggleStatus = async (keyId: string, checked: boolean) => {
    try {
      setTogglingId(keyId);
      await agentService.updateAgentApiKey(agentId, keyId, {
        status: checked ? 'active' : 'inactive',
      });
      await refetch();
    } finally {
      setTogglingId(null);
    }
  };

  return (
    <div className="space-y-5 p-4 sm:p-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="flex items-center justify-center w-10 h-10 bg-primary/10 rounded-lg">
            <KeyRound className="h-5 w-5 text-primary" />
          </div>
          <div>
            <h2 className="text-2xl font-bold">{t('agents.apiKeys.title')}</h2>
            <p className="text-muted-foreground">{t('agents.apiKeys.description')}</p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" onClick={() => refetch()} disabled={isFetching}>
            {t('common.refresh')}
          </Button>
          <Button onClick={openCreate}>
            <Plus className="h-4 w-4" />
            {t('agents.apiKeys.actions.createKey')}
          </Button>
        </div>
      </div>

      {/* Table */}
      {isLoading ? (
        <div className="space-y-3">
          <Skeleton className="h-8 w-64" />
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
        </div>
      ) : keys.length === 0 ? (
        <div className="rounded-lg border border-dashed p-8 text-center">
          <div className="mx-auto mb-4 flex size-12 items-center justify-center rounded-lg bg-primary/10">
            <KeyRound className="size-6 text-primary" />
          </div>
          <h3 className="text-base font-semibold text-foreground">
            {t('agents.apiKeys.emptyTitle')}
          </h3>
          <p className="mx-auto mt-2 max-w-md text-sm leading-6 text-muted-foreground">
            {t('agents.apiKeys.emptyDescription')}
          </p>
          <div className="mt-5 flex justify-center gap-2">
            <Button onClick={openCreate}>
              <Plus className="size-4" />
              {t('agents.apiKeys.actions.createKey')}
            </Button>
          </div>
        </div>
      ) : (
        <div className="overflow-hidden rounded-lg border bg-background">
          <div className="flex items-center border-b bg-muted/20 px-4 py-3">
            <div className="relative w-full sm:w-80">
              <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder={t('agents.apiKeys.searchPlaceholder')}
                value={search}
                onChange={e => setSearch(e.target.value)}
                className="h-9 bg-background pl-9 pr-9 shadow-none"
              />
              {hasSearch ? (
                <Button
                  type="button"
                  variant="ghost"
                  isIcon
                  onClick={() => setSearch('')}
                  className="absolute right-1 top-1/2 size-7 -translate-y-1/2 text-muted-foreground"
                  aria-label={t('common.clear')}
                >
                  <X className="size-3.5" />
                </Button>
              ) : null}
            </div>
          </div>

          {filteredKeys.length === 0 ? (
            <div className="p-10 text-center">
              <h3 className="text-sm font-semibold text-foreground">
                {t('agents.apiKeys.emptySearchTitle')}
              </h3>
              <p className="mt-2 text-sm text-muted-foreground">
                {t('agents.apiKeys.emptySearchDescription')}
              </p>
              <Button variant="outline" onClick={() => setSearch('')} className="mt-4">
                {t('common.clear')}
              </Button>
            </div>
          ) : (
            <div className="overflow-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('agents.apiKeys.columns.name')}</TableHead>
                    <TableHead>{t('agents.apiKeys.columns.key')}</TableHead>
                    <TableHead>{t('agents.apiKeys.columns.updatedAt')}</TableHead>
                    <TableHead>{t('agents.apiKeys.columns.createdAt')}</TableHead>
                    <TableHead>{t('agents.apiKeys.columns.expiresAt')}</TableHead>
                    <TableHead>{t('agents.apiKeys.columns.status')}</TableHead>
                    <TableHead className="w-[80px]">
                      {t('agents.apiKeys.columns.actions')}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filteredKeys.map(k => (
                    <TableRow key={k.id}>
                      <TableCell className="font-medium">{k.name || '-'}</TableCell>
                      <TableCell>
                        <span className="font-mono">
                          {k.key_prefix ? `${k.key_prefix}*********` : '-'}
                        </span>
                      </TableCell>
                      <TableCell>{k.updated_at ? formatDate(k.updated_at) : '-'}</TableCell>
                      <TableCell>{k.created_at ? formatDate(k.created_at) : '-'}</TableCell>
                      <TableCell>
                        {k.expires_at ? formatDate(k.expires_at) : t('agents.apiKeys.noExpiry')}
                      </TableCell>
                      <TableCell>
                        {k.status === 'active' ? (
                          <Badge>{t('agents.apiKeys.active')}</Badge>
                        ) : k.status === 'inactive' ? (
                          <Badge variant="secondary">{t('agents.apiKeys.inactive')}</Badge>
                        ) : (
                          <Badge variant="destructive">{t('agents.apiKeys.revoked')}</Badge>
                        )}
                      </TableCell>
                      <TableCell>
                        <div className="flex items-center gap-2">
                          <Switch
                            checked={k.status === 'active'}
                            onCheckedChange={checked => handleToggleStatus(k.id, checked)}
                            disabled={togglingId === k.id}
                          />
                          <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                              <Button variant="ghost" isIcon>
                                <MoreHorizontal className="h-4 w-4" />
                              </Button>
                            </DropdownMenuTrigger>
                            <DropdownMenuContent align="end" className="w-40">
                              <DropdownMenuItem onClick={() => openEdit(k.id)}>
                                <Edit className="mr-2 h-4 w-4" /> {t('common.edit')}
                              </DropdownMenuItem>
                              <DropdownMenuSeparator />
                              {k.status !== 'revoked' ? (
                                <DropdownMenuItem
                                  className="text-destructive"
                                  onClick={() => {
                                    setDeleteId(k.id);
                                    setDeleteOpen(true);
                                  }}
                                >
                                  <Trash2 className="mr-2 h-4 w-4" /> {t('common.delete')}
                                </DropdownMenuItem>
                              ) : null}
                            </DropdownMenuContent>
                          </DropdownMenu>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </div>
      )}

      <Dialog open={formOpen} onOpenChange={setFormOpen}>
        <DialogContent size="md" className="overflow-hidden p-0">
          <DialogHeader>
            <DialogTitle>
              {isEdit ? t('agents.apiKeys.editTitle') : t('agents.apiKeys.createTitle')}
            </DialogTitle>
          </DialogHeader>
          <DialogBody className="space-y-5 pb-2">
            <div className="space-y-2">
              <Label>{t('common.name')}</Label>
              <Input
                value={newName}
                onChange={e => setNewName(e.target.value)}
                placeholder={t('agents.apiKeys.namePlaceholder')}
              />
            </div>
            {!isEdit && (
              <div className="space-y-3 rounded-lg border bg-muted/20 p-4">
                <div className="flex items-center justify-between">
                  <Label>{t('agents.apiKeys.expiryLabel')}</Label>
                  <div className="flex items-center gap-3">
                    <span className="text-xs text-muted-foreground">
                      {hasExpiry ? t('common.enabled') : t('agents.apiKeys.noExpiry')}
                    </span>
                    <Switch
                      checked={hasExpiry}
                      onCheckedChange={v => {
                        setHasExpiry(v);
                        if (!v) setNewExpiresAt('');
                      }}
                    />
                  </div>
                </div>
                <Input
                  type="datetime-local"
                  value={newExpiresAt}
                  onChange={e => setNewExpiresAt(e.target.value)}
                  disabled={!hasExpiry}
                  min={new Date(Date.now() - new Date().getTimezoneOffset() * 60000)
                    .toISOString()
                    .slice(0, 16)}
                  className="bg-background"
                />
              </div>
            )}
          </DialogBody>
          <DialogFooter className="border-t">
            <Button variant="outline" onClick={() => setFormOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button
              onClick={handleSubmit}
              disabled={isEdit ? updateMutation.isPending : createMutation.isPending}
            >
              {isEdit
                ? updateMutation.isPending
                  ? t('agents.apiKeys.saving')
                  : t('common.save')
                : createMutation.isPending
                  ? t('agents.apiKeys.creating')
                  : t('common.create')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        variant="danger"
        title={t('agents.apiKeys.deleteConfirm.title')}
        description={t('agents.apiKeys.deleteConfirm.description')}
        confirmText={t('common.delete')}
        cancelText={t('common.cancel')}
        onConfirm={() => {
          if (deleteId) handleDelete(deleteId);
        }}
        loading={deleteMutation.isPending}
        open={deleteOpen}
        onOpenChange={o => {
          setDeleteOpen(o);
          if (!o) setDeleteId(null);
        }}
      />

      {/* Secret Dialog (only shows once after creation) */}
      <Dialog open={secretOpen} onOpenChange={setSecretOpen}>
        <DialogContent size="md" className="overflow-hidden p-0">
          <DialogHeader>
            <DialogTitle>{t('agents.apiKeys.createdTitle')}</DialogTitle>
            <DialogDescription className="leading-6">
              {t('agents.apiKeys.createdNotice')}
            </DialogDescription>
          </DialogHeader>
          <DialogBody className="pb-2">
            <div className="flex items-center gap-3 rounded-lg border bg-muted/30 p-4">
              <code className="min-w-0 flex-1 break-all font-mono text-sm leading-6 text-foreground">
                {createdSecretKey || '-'}
              </code>
              <Button
                variant="outline"
                isIcon
                onClick={() => {
                  if (createdSecretKey) {
                    navigator.clipboard.writeText(createdSecretKey);
                    toast.success(t('agents.apiKeys.copiedTitle'));
                  }
                }}
                className="shrink-0"
                aria-label={t('agents.apiKeys.copy')}
              >
                <Copy className="h-4 w-4" />
              </Button>
            </div>
          </DialogBody>
          <DialogFooter className="border-t">
            <Button
              variant="outline"
              onClick={() => {
                setSecretOpen(false);
                setCreatedSecretKey('');
              }}
            >
              {t('common.close')}
            </Button>
            <Button
              onClick={() => {
                if (createdSecretKey) {
                  navigator.clipboard.writeText(createdSecretKey);
                  toast.success(t('agents.apiKeys.copiedTitle'));
                }
              }}
            >
              <Copy className="h-4 w-4" />
              {t('agents.apiKeys.copy')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
