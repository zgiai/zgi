'use client';

import { useEffect, useState, type FormEvent } from 'react';
import Link from 'next/link';
import { useLocale } from 'next-intl';
import { CalendarDays, Clock3, Pencil } from 'lucide-react';
import { toast } from 'sonner';
import { formatConnectorDateTime } from '@/components/connectors/connector-metadata';
import { ConnectionChatDefaultButton } from './connection-chat-default-card';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
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
import { Label } from '@/components/ui/label';
import { useRenamePersonalConnection } from '@/hooks/connectors/use-connectors';
import { useT } from '@/i18n';
import type { PersonalConnectionDetail } from '@/services/types/connector';
import { buildRenameConnectionRequest } from './connection-management-model';
import {
  personalConnectionDisplayName,
  personalConnectionSecondaryIdentity,
} from './personal-connections-model';

function initials(value: string): string {
  const parts = value.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return 'ID';
  return parts
    .slice(0, 2)
    .map(part => Array.from(part)[0] ?? '')
    .join('')
    .toUpperCase();
}

interface ConnectionAccountCardProps {
  connection: PersonalConnectionDetail;
  connectorName: string;
  renameInitiallyOpen?: boolean;
}

export function ConnectionAccountCard({
  connection,
  connectorName,
  renameInitiallyOpen = false,
}: ConnectionAccountCardProps) {
  const t = useT('connectors');
  const locale = useLocale();
  const [renameOpen, setRenameOpen] = useState(renameInitiallyOpen);
  const [name, setName] = useState(connection.name);
  const renameMutation = useRenamePersonalConnection(connection.id);
  const request = buildRenameConnectionRequest(connection, name);
  const unchanged = request?.name === connection.name.trim();
  const connectionName = personalConnectionDisplayName(
    connection,
    t('connections.unknownConnection')
  );
  const identity = personalConnectionSecondaryIdentity(
    connection,
    t('connections.unknownIdentity')
  );

  useEffect(() => {
    if (renameOpen) setName(connection.name);
  }, [connection.name, renameOpen]);

  const submitRename = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!request || unchanged) return;
    try {
      await renameMutation.mutateAsync(request);
      toast.success(t('feedback.renamed'));
      setRenameOpen(false);
    } catch {
      toast.error(t('feedback.renameFailed'));
    }
  };

  return (
    <>
      <Card className="h-full">
        <CardHeader className="flex-row items-center justify-between gap-3 space-y-0">
          <CardTitle>{t('account.title')}</CardTitle>
          <Button size="sm" variant="ghost" onClick={() => setRenameOpen(true)}>
            <Pencil aria-hidden="true" className="size-3.5" />
            {t('account.rename')}
          </Button>
        </CardHeader>
        <CardContent className="space-y-5">
          <div className="flex min-w-0 items-center gap-4 border-b pb-5">
            <div
              aria-hidden="true"
              className="flex size-16 shrink-0 items-center justify-center rounded-full bg-primary/10 text-xl font-semibold text-primary"
            >
              {initials(connectionName)}
            </div>
            <div className="min-w-0">
              <div className="flex items-center gap-2"><p className="truncate text-lg font-semibold">{connectionName}</p><ConnectionChatDefaultButton connection={connection} /></div>
              <p className="mt-1 truncate text-sm text-muted-foreground">{identity}</p>
              <p className="mt-1 truncate text-xs text-muted-foreground">
                {t('account.externalAccount', { connector: connectorName })}
              </p>
            </div>
          </div>

          <dl className="space-y-3 text-sm">
            <div className="grid grid-cols-[auto_minmax(0,1fr)] items-start gap-3">
              <CalendarDays aria-hidden="true" className="mt-0.5 size-4 text-muted-foreground" />
              <div className="grid min-w-0 gap-1 sm:grid-cols-[110px_minmax(0,1fr)]">
                <dt className="text-muted-foreground">{t('account.createdAt')}</dt>
                <dd className="break-words font-medium">
                  {formatConnectorDateTime(
                    connection.created_at,
                    locale,
                    t('account.neverVerified')
                  )}
                </dd>
              </div>
            </div>
            <div className="grid grid-cols-[auto_minmax(0,1fr)] items-start gap-3">
              <Clock3 aria-hidden="true" className="mt-0.5 size-4 text-muted-foreground" />
              <div className="grid min-w-0 gap-1 sm:grid-cols-[110px_minmax(0,1fr)]">
                <dt className="text-muted-foreground">{t('account.lastVerifiedAt')}</dt>
                <dd className="break-words font-medium">
                  {formatConnectorDateTime(
                    connection.last_verified_at,
                    locale,
                    t('account.neverVerified')
                  )}
                </dd>
              </div>
            </div>
          </dl>

          <Button asChild variant="outline" className="w-full sm:w-auto">
            <Link href={`/console/connectors/${encodeURIComponent(connection.connector_id)}`}>
              {t('account.viewConnectorDetails')}
            </Link>
          </Button>
        </CardContent>
      </Card>

      <Dialog open={renameOpen} onOpenChange={setRenameOpen}>
        <DialogContent size="sm">
          <form onSubmit={submitRename}>
            <DialogHeader>
              <DialogTitle>{t('account.renameTitle')}</DialogTitle>
              <DialogDescription>{t('account.renameDescription')}</DialogDescription>
            </DialogHeader>
            <DialogBody>
              <div className="space-y-2">
                <Label htmlFor="connector-connection-name">{t('account.nameLabel')}</Label>
                <Input
                  id="connector-connection-name"
                  value={name}
                  maxLength={128}
                  showCharacterCount
                  autoFocus
                  placeholder={t('account.namePlaceholder')}
                  errorText={
                    !request && name !== connection.name ? t('account.invalidName') : undefined
                  }
                  onChange={event => setName(event.target.value)}
                />
              </div>
            </DialogBody>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setRenameOpen(false)}>
                {t('reconnect.cancel')}
              </Button>
              <Button
                type="submit"
                loading={renameMutation.isPending}
                disabled={!request || unchanged}
              >
                {t('account.saveName')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  );
}
