'use client';

import React, { useMemo, useState } from 'react';
import { Plus, MoreHorizontal, Copy, Edit, Trash2, KeyRound } from 'lucide-react';
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
import { Alert } from '@/components/ui/alert';
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
        status: checked ? 'active' : 'revoked',
      });
      await refetch();
    } finally {
      setTogglingId(null);
    }
  };

  return (
    <div className="space-y-6 p-4">
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

      {/* Toolbar */}
      <div className="flex items-center gap-3">
        <Input
          placeholder={t('agents.apiKeys.searchPlaceholder')}
          value={search}
          onChange={e => setSearch(e.target.value)}
          className="max-w-sm"
        />
      </div>

      {/* Table */}
      {isLoading ? (
        <div className="space-y-3">
          <Skeleton className="h-8 w-64" />
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
        </div>
      ) : filteredKeys.length === 0 ? (
        <div className="border rounded-lg p-8 text-center text-muted-foreground">
          {t('agents.apiKeys.empty')}
        </div>
      ) : (
        <div className="rounded-md border overflow-auto">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('agents.apiKeys.columns.name')}</TableHead>
                <TableHead>{t('agents.apiKeys.columns.key')}</TableHead>
                <TableHead>{t('agents.apiKeys.columns.updatedAt')}</TableHead>
                <TableHead>{t('agents.apiKeys.columns.createdAt')}</TableHead>
                <TableHead>{t('agents.apiKeys.columns.expiresAt')}</TableHead>
                <TableHead>{t('agents.apiKeys.columns.status')}</TableHead>
                <TableHead className="w-[80px]">{t('agents.apiKeys.columns.actions')}</TableHead>
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
                            <Edit className="h-4 w-4 mr-2" /> {t('common.edit')}
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem
                            className="text-destructive"
                            onClick={() => {
                              setDeleteId(k.id);
                              setDeleteOpen(true);
                            }}
                          >
                            <Trash2 className="h-4 w-4 mr-2" /> {t('common.delete')}
                          </DropdownMenuItem>
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

      <Dialog open={formOpen} onOpenChange={setFormOpen}>
        <DialogContent className="max-w-md p-0 overflow-hidden">
          <DialogHeader>
            <DialogTitle className="text-xl font-bold tracking-tight">
              {isEdit ? t('agents.apiKeys.editTitle') : t('agents.apiKeys.createTitle')}
            </DialogTitle>
          </DialogHeader>
          <DialogBody className="py-6 space-y-6">
            <div className="space-y-2">
              <Label className="text-sm font-bold text-neutral-700 tracking-tight">
                {t('common.name')}
              </Label>
              <Input
                value={newName}
                onChange={e => setNewName(e.target.value)}
                placeholder={t('agents.apiKeys.namePlaceholder')}
                className="h-11 rounded-xl border-neutral-200 focus:ring-blue-500/20 transition-all"
              />
            </div>
            {!isEdit && (
              <div className="space-y-3 bg-neutral-50/50 p-4 rounded-2xl border border-neutral-100">
                <div className="flex items-center justify-between">
                  <Label className="text-sm font-bold text-neutral-700 tracking-tight">
                    {t('agents.apiKeys.expiryLabel')}
                  </Label>
                  <div className="flex items-center gap-3">
                    <span className="text-xs font-medium text-neutral-500 uppercase tracking-wider">
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
                  className="h-11 rounded-xl border-neutral-200 focus:ring-blue-500/20 transition-all bg-white disabled:opacity-50"
                />
              </div>
            )}
          </DialogBody>
          <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t">
            <Button
              variant="ghost"
              onClick={() => setFormOpen(false)}
              className="font-bold rounded-xl h-11 px-6 hover:bg-neutral-100"
            >
              {t('common.cancel')}
            </Button>
            <Button
              onClick={handleSubmit}
              disabled={isEdit ? updateMutation.isPending : createMutation.isPending}
              className="font-bold rounded-xl h-11 px-8 shadow-premium transition-all active:scale-95"
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
        <DialogContent className="max-w-md p-0 overflow-hidden">
          <DialogHeader>
            <DialogTitle className="text-xl font-bold tracking-tight text-emerald-600">
              {t('agents.apiKeys.createdTitle')}
            </DialogTitle>
          </DialogHeader>
          <DialogBody className="py-6 space-y-6">
            <Alert
              variant="destructive"
              className="rounded-2xl border-red-100 bg-red-50 text-red-900 shadow-sm animate-in fade-in slide-in-from-top-2 duration-500"
            >
              <span className="text-xs font-medium leading-relaxed">
                {t('agents.apiKeys.createdNotice')}
              </span>
            </Alert>
            <div className="flex items-center gap-3 bg-neutral-900 text-white rounded-2xl p-5 shadow-2xl group transition-all hover:scale-[1.02]">
              <div className="font-mono break-all text-sm flex-1 leading-relaxed opacity-90">
                {createdSecretKey || '-'}
              </div>
              <Button
                variant="ghost"
                isIcon
                onClick={() => {
                  if (createdSecretKey) {
                    navigator.clipboard.writeText(createdSecretKey);
                    toast.success(t('agents.apiKeys.copiedTitle'));
                  }
                }}
                className="h-10 w-10 text-white hover:bg-white/10 rounded-xl"
              >
                <Copy className="h-5 w-5" />
              </Button>
            </div>
          </DialogBody>
          <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t">
            <Button
              variant="ghost"
              onClick={() => {
                setSecretOpen(false);
                setCreatedSecretKey('');
              }}
              className="font-bold rounded-xl h-11 px-6 hover:bg-neutral-100"
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
              className="font-bold rounded-xl h-11 px-8 shadow-premium transition-all active:scale-95 bg-emerald-600 hover:bg-emerald-700"
            >
              {t('agents.apiKeys.copy')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
