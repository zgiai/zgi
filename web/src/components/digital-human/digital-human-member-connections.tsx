'use client';

import * as React from 'react';
import Link from 'next/link';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ExternalLink, Loader2, PlugZap } from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useT } from '@/i18n';
import { ENABLE_DIGITAL_HUMAN_PER_USER_CONNECTORS } from '@/lib/config';
import { digitalHumanService } from '@/services/digital-human.service';
import { getErrorMessage } from '@/utils/error-notifications';

const NONE = '__none__';

/**
 * @component DigitalHumanMemberConnections
 * @category Feature
 * @status Beta
 * @description Lets a workspace member choose which of their own Connections
 * a DigitalHuman's per-user Connector slots act with (ADR 0007). It renders
 * nothing when the DigitalHuman has no per-user slot.
 * @usage Placed in the DigitalHuman chat header.
 * @example
 * <DigitalHumanMemberConnections id={digitalHumanId} />
 */
export function DigitalHumanMemberConnections({ id }: { id: string }) {
  const t = useT('digitalHumans');
  const queryClient = useQueryClient();
  const [open, setOpen] = React.useState(false);
  const queryKey = ['digital-human', id, 'member-connector-slots'];
  const slots = useQuery({
    queryKey,
    queryFn: () => digitalHumanService.listMemberConnectorSlots(id),
    enabled: ENABLE_DIGITAL_HUMAN_PER_USER_CONNECTORS,
    retry: false,
  });
  const select = useMutation({
    mutationFn: ({ slotKey, connectionId }: { slotKey: string; connectionId: string | null }) =>
      connectionId
        ? digitalHumanService.selectMemberConnectorSlot(id, slotKey, connectionId)
        : digitalHumanService.clearMemberConnectorSlot(id, slotKey),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey });
      toast.success(t('memberConnections.saved'));
    },
    onError: error => {
      toast.error(getErrorMessage(error));
    },
  });
  const items = slots.data?.data.items ?? [];
  if (!ENABLE_DIGITAL_HUMAN_PER_USER_CONNECTORS || items.length === 0) return null;
  const missing = items.filter(item => item.state !== 'connected').length;

  return (
    <>
      <Button variant="ghost" size="sm" onClick={() => setOpen(true)}>
        <PlugZap className="mr-2 size-4" />
        {t('memberConnections.button')}
        {missing > 0 ? (
          <Badge variant="outline" className="ml-2 px-1.5 text-[10px]">
            {missing}
          </Badge>
        ) : null}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>{t('memberConnections.title')}</DialogTitle>
            <DialogDescription>{t('memberConnections.description')}</DialogDescription>
          </DialogHeader>
          <DialogBody className="space-y-3">
            {items.map(item => (
              <div key={item.slot_key} className="space-y-2 rounded-lg border p-3">
                <div className="flex items-center justify-between gap-2">
                  <span className="min-w-0 truncate text-sm font-medium">{item.name}</span>
                  <Badge variant={item.state === 'connected' ? 'secondary' : 'outline'}>
                    {t(`memberConnections.states.${item.state}`)}
                  </Badge>
                </div>
                {item.options.length === 0 ? (
                  <p className="text-xs text-muted-foreground">
                    {t('memberConnections.noOptions')}
                  </p>
                ) : (
                  <Select
                    value={item.connection_id ?? NONE}
                    disabled={select.isPending}
                    onValueChange={value =>
                      select.mutate({
                        slotKey: item.slot_key,
                        connectionId: value === NONE ? null : value,
                      })
                    }
                  >
                    <SelectTrigger className="h-9">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value={NONE}>{t('memberConnections.none')}</SelectItem>
                      {item.options.map(option => (
                        <SelectItem key={option.connection_id} value={option.connection_id}>
                          {option.account ? `${option.name} · ${option.account}` : option.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
                <Button variant="link" size="sm" className="h-auto px-0 text-xs" asChild>
                  <Link href={`/console/connectors/${encodeURIComponent(item.connector_id)}`}>
                    <ExternalLink className="mr-1 size-3" />
                    {t('memberConnections.create')}
                  </Link>
                </Button>
              </div>
            ))}
            {select.isPending ? (
              <Loader2 className="size-4 animate-spin text-muted-foreground" />
            ) : null}
          </DialogBody>
        </DialogContent>
      </Dialog>
    </>
  );
}
