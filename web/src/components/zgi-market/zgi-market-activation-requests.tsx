'use client';

import { useState } from 'react';
import Link from 'next/link';
import { Inbox } from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
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
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Textarea } from '@/components/ui/textarea';
import {
  useActivateZgiMarketListing,
  useInstallZgiMarketSkill,
  useSettleZgiMarketActivationRequest,
  useZgiMarketActivationRequests,
} from '@/hooks/use-zgi-market';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import type { MarketActivationRequest } from '@/services/types/zgi-market';
import { formatDate } from '@/utils/format';

/**
 * Whether approving a request can activate the item by itself. A connector needs an
 * authorization, and an API asked for as a whole needs an offering chosen: those are handled on
 * the item's page and then marked handled.
 */
export function activatesOnApproval(
  row: Pick<MarketActivationRequest, 'listing_kind' | 'offering_id'>
) {
  switch (row.listing_kind) {
    case 'mcp':
    case 'plugin':
    case 'skill':
      return true;
    case 'api':
      return row.offering_id > 0;
    default:
      return false;
  }
}

type Tab = 'pending' | 'handled';

/** The administrators' entry to the members' activation requests, with the pending count. */
export function ZgiMarketActivationRequests() {
  const t = useT();
  const { locale } = useLocale();
  const [open, setOpen] = useState(false);
  const [tab, setTab] = useState<Tab>('pending');
  const [rejecting, setRejecting] = useState<MarketActivationRequest | null>(null);
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState<string | null>(null);

  // One read answers at most 100 requests, newest first: the pending ones are read by themselves so
  // that newer handled ones never push them out of the list.
  const requests = useZgiMarketActivationRequests(tab === 'pending' ? { status: 'pending' } : {});
  const activate = useActivateZgiMarketListing();
  const install = useInstallZgiMarketSkill();
  const settle = useSettleZgiMarketActivationRequest();

  const all = requests.data?.data ?? [];
  const pending = requests.data?.pending ?? 0;
  const rows = all.filter(row =>
    tab === 'pending' ? row.status === 'pending' : row.status !== 'pending'
  );

  const approve = async (row: MarketActivationRequest) => {
    setBusy(row.id);
    try {
      const automatic = activatesOnApproval(row);
      if (automatic && row.listing_kind === 'skill') {
        const result = await install.mutateAsync({ slug: row.listing_slug });
        if (!result.installed) throw new Error(result.conflict ?? 'not installed');
      } else if (automatic) {
        await activate.mutateAsync({
          listingId: row.listing_id,
          ...(row.offering_id ? { offeringId: row.offering_id } : {}),
        });
      }
      try {
        await settle.mutateAsync({ id: row.id, action: 'approve' });
        toast.success(
          automatic ? t('market.activation.approved') : t('market.activation.approvedOnly')
        );
      } catch {
        toast.error(t('market.activation.settleFailed'));
      }
    } catch {
      toast.error(t('market.activation.approveFailed'));
    } finally {
      setBusy(null);
    }
  };

  const reject = () => {
    if (!rejecting) return;
    settle.mutate(
      { id: rejecting.id, action: 'reject', note: note.trim() },
      {
        onSuccess: () => {
          toast.success(t('market.activation.rejectDone'));
          setRejecting(null);
          setNote('');
        },
        onError: () => toast.error(t('market.activation.settleFailed')),
      }
    );
  };

  return (
    <>
      <Button variant="outline" size="sm" onClick={() => setOpen(true)}>
        <Inbox className="size-4" aria-hidden="true" />
        {t('market.activation.requestsButton')}
        {pending > 0 ? (
          <Badge variant="destructive" className="ml-0.5 h-5 min-w-5 justify-center px-1.5">
            {pending}
          </Badge>
        ) : null}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{t('market.activation.requestsTitle')}</DialogTitle>
            <DialogDescription>{t('market.activation.requestsDescription')}</DialogDescription>
          </DialogHeader>
          <DialogBody className="flex max-h-[60vh] flex-col gap-3 overflow-y-auto">
            <Tabs value={tab} onValueChange={value => setTab(value as Tab)}>
              <TabsList>
                <TabsTrigger value="pending">
                  {t('market.activation.tabPending')}
                  {pending > 0 ? <span className="ml-1.5 text-xs">{pending}</span> : null}
                </TabsTrigger>
                <TabsTrigger value="handled">{t('market.activation.tabHandled')}</TabsTrigger>
              </TabsList>
            </Tabs>
            {requests.isPending ? (
              <Skeleton className="h-24 w-full" />
            ) : rows.length === 0 ? (
              <p className="py-8 text-center text-sm text-muted-foreground">
                {tab === 'pending'
                  ? t('market.activation.empty')
                  : t('market.activation.emptyHandled')}
              </p>
            ) : (
              <ul className="flex flex-col divide-y rounded-md border">
                {rows.map(row => {
                  const href = `/console/market/catalog/${row.listing_kind}/${row.listing_slug}`;
                  const automatic = activatesOnApproval(row);
                  return (
                    <li key={row.id} className="flex flex-col gap-2 p-3">
                      <div className="flex flex-wrap items-center gap-2">
                        <Link
                          href={href}
                          className="font-medium hover:underline"
                          onClick={() => setOpen(false)}
                        >
                          {row.listing_title || row.listing_slug}
                        </Link>
                        <Badge variant="secondary">
                          {t(`market.catalog.kinds.${row.listing_kind}`)}
                        </Badge>
                        {row.offering_title ? (
                          <span className="text-xs text-muted-foreground">
                            {t('market.activation.offering', { name: row.offering_title })}
                          </span>
                        ) : null}
                        {row.status !== 'pending' ? (
                          <Badge variant={row.status === 'approved' ? 'success' : 'outline'}>
                            {t(`market.activation.status.${row.status}`)}
                          </Badge>
                        ) : null}
                      </div>
                      <p className="text-sm">{row.reason || t('market.activation.noReason')}</p>
                      <p className="text-xs text-muted-foreground">
                        {row.requester_name || row.requester_id} ·{' '}
                        {t('market.activation.requestedAt', {
                          date: formatDate(row.created_at, 'YYYY-MM-DD HH:mm', { locale }),
                        })}
                        {row.status !== 'pending' && row.reviewer_name
                          ? ` · ${t('market.activation.reviewedBy', { name: row.reviewer_name })}`
                          : ''}
                        {row.review_note ? ` · ${row.review_note}` : ''}
                      </p>
                      {row.status === 'pending' ? (
                        <div className="flex flex-wrap items-center gap-2">
                          {automatic ? null : (
                            <Button asChild size="sm" variant="outline">
                              <Link href={href} onClick={() => setOpen(false)}>
                                {t('market.activation.open')}
                              </Link>
                            </Button>
                          )}
                          <Button
                            size="sm"
                            disabled={busy !== null}
                            onClick={() => void approve(row)}
                          >
                            {automatic
                              ? t('market.activation.approve')
                              : t('market.activation.approveOnly')}
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            disabled={busy !== null}
                            onClick={() => setRejecting(row)}
                          >
                            {t('market.activation.reject')}
                          </Button>
                          {automatic ? null : (
                            <span className="text-xs text-muted-foreground">
                              {t('market.activation.needsManualHandling')}
                            </span>
                          )}
                        </div>
                      ) : null}
                    </li>
                  );
                })}
              </ul>
            )}
          </DialogBody>
        </DialogContent>
      </Dialog>
      <Dialog open={rejecting !== null} onOpenChange={value => (value ? null : setRejecting(null))}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t('market.activation.rejectTitle')}</DialogTitle>
          </DialogHeader>
          <DialogBody className="flex flex-col gap-2">
            <Label htmlFor="zgi-market-reject-note">{t('market.activation.rejectNote')}</Label>
            <Textarea
              id="zgi-market-reject-note"
              value={note}
              maxLength={500}
              rows={3}
              onChange={event => setNote(event.target.value)}
            />
          </DialogBody>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRejecting(null)}>
              {t('common.cancel')}
            </Button>
            <Button variant="destructive" disabled={settle.isPending} onClick={reject}>
              {t('market.activation.confirmReject')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
