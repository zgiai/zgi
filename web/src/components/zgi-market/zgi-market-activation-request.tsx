'use client';

import { useState } from 'react';
import { Clock3 } from 'lucide-react';
import { toast } from 'sonner';
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
import { Textarea } from '@/components/ui/textarea';
import {
  useRequestZgiMarketActivation,
  useSettleZgiMarketActivationRequest,
  useZgiMarketActivationRequests,
} from '@/hooks/use-zgi-market';
import { useT } from '@/i18n';
import type { MarketListing } from '@/services/types/zgi-market';

const maxReason = 500;

/**
 * What a member who may not activate sees in place of the activate button: a request to the
 * administrators, where it stands, and how it was answered.
 */
export function ZgiMarketActivationRequest({
  listing,
  title,
}: {
  listing: MarketListing;
  title: string;
}) {
  const t = useT();
  const requests = useZgiMarketActivationRequests({ listing_id: listing.id, mine: true });
  const request = useRequestZgiMarketActivation();
  const settle = useSettleZgiMarketActivationRequest();
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState('');

  // The newest request decides: pending waits, rejected may be asked again.
  const latest = requests.data?.data[0];
  const only =
    listing.kind === 'api' && listing.offerings.length === 1 ? listing.offerings[0] : null;

  const submit = () =>
    request.mutate(
      {
        listing_id: listing.id,
        listing_kind: listing.kind,
        listing_slug: listing.slug,
        listing_title: title,
        ...(only ? { offering_id: only.id, offering_title: only.tier || only.slug } : {}),
        reason: reason.trim(),
      },
      {
        onSuccess: () => {
          setOpen(false);
          setReason('');
          toast.success(t('market.activation.submitted'));
        },
        onError: () => toast.error(t('market.activation.submitFailed')),
      }
    );

  if (latest?.status === 'pending') {
    return (
      <div className="flex flex-col gap-2">
        <p className="flex h-10 items-center justify-center gap-1.5 rounded-md bg-warning/10 text-sm font-medium text-warning">
          <Clock3 className="size-4" aria-hidden="true" />
          {t('market.activation.pending')}
        </p>
        <Button
          variant="ghost"
          size="sm"
          disabled={settle.isPending}
          onClick={() =>
            settle.mutate(
              { id: latest.id, action: 'cancel' },
              {
                onSuccess: () => toast.success(t('market.activation.withdrawn')),
                onError: () => toast.error(t('market.activation.settleFailed')),
              }
            )
          }
        >
          {t('market.activation.withdraw')}
        </Button>
      </div>
    );
  }

  const rejected = latest?.status === 'rejected';
  return (
    <div className="flex flex-col gap-2">
      <Button className="w-full" disabled={requests.isPending} onClick={() => setOpen(true)}>
        {rejected ? t('market.activation.requestAgain') : t('market.activation.request')}
      </Button>
      {rejected ? (
        <p role="status" className="text-xs leading-5 text-destructive">
          {latest.review_note
            ? t('market.activation.rejectedWithNote', { note: latest.review_note })
            : t('market.activation.rejected')}
        </p>
      ) : (
        <p className="text-xs leading-5 text-muted-foreground">
          {t('market.activation.memberHint')}
        </p>
      )}
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t('market.activation.requestTitle', { name: title })}</DialogTitle>
            <DialogDescription>{t('market.activation.requestDescription')}</DialogDescription>
          </DialogHeader>
          <DialogBody className="flex flex-col gap-2">
            <Label htmlFor="zgi-market-activation-reason">{t('market.activation.reason')}</Label>
            <Textarea
              id="zgi-market-activation-reason"
              value={reason}
              maxLength={maxReason}
              rows={3}
              placeholder={t('market.activation.reasonPlaceholder')}
              onChange={event => setReason(event.target.value)}
            />
          </DialogBody>
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button disabled={request.isPending} onClick={submit}>
              {t('market.activation.submit')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
