'use client';

import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { Check, Copy, Link2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Dialog, DialogBody, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { formatTaskTime } from '@/components/digital-human/digital-human-task-presentation';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import { digitalHumanService } from '@/services/digital-human.service';
import type { DigitalHumanArtifactShare, DigitalHumanArtifactShareLifetime } from '@/services/types/digital-human';
import { getErrorMessage } from '@/utils/error-notifications';

const lifetimes: DigitalHumanArtifactShareLifetime[] = ['1d', '7d', '30d', 'never'];

/** The page a share link opens, on this site. */
export function artifactShareUrl(token: string) {
  return `${window.location.origin}/s/${encodeURIComponent(token)}`;
}

/**
 * Makes links that open one version of an artifact for anyone holding them,
 * without signing in. A link is shown once, when it is made; earlier ones
 * can only be revoked.
 */
export function ArtifactShareDialog({
  digitalHumanId,
  artifactId,
  version,
  open,
  onOpenChange,
}: {
  digitalHumanId: string;
  artifactId: string;
  version: number;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const t = useT('digitalHumans');
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  const [lifetime, setLifetime] = useState<DigitalHumanArtifactShareLifetime>('7d');
  const [link, setLink] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const sharesKey = ['digital-human-artifact-shares', digitalHumanId, artifactId];
  const shares = useQuery({
    queryKey: sharesKey,
    queryFn: () => digitalHumanService.listArtifactShares(digitalHumanId, artifactId),
    enabled: open,
  });
  const create = useMutation({
    mutationFn: () => digitalHumanService.createArtifactShare(digitalHumanId, artifactId, { version, expires_in: lifetime }),
    onSuccess: result => {
      setLink(artifactShareUrl(result.data.token));
      setCopied(false);
      void queryClient.invalidateQueries({ queryKey: sharesKey });
    },
    onError: error => toast.error(getErrorMessage(error) || t('artifacts.share.error')),
  });
  const revoke = useMutation({
    mutationFn: (shareId: string) => digitalHumanService.revokeArtifactShare(digitalHumanId, artifactId, shareId),
    onSuccess: () => {
      toast.success(t('artifacts.share.revoked'));
      void queryClient.invalidateQueries({ queryKey: sharesKey });
    },
    onError: error => toast.error(getErrorMessage(error) || t('artifacts.share.error')),
  });
  const copy = async () => {
    if (!link) return;
    try {
      await navigator.clipboard.writeText(link);
      setCopied(true);
      toast.success(t('artifacts.share.copied'));
    } catch {
      toast.error(t('artifacts.share.copyError'));
    }
  };
  const close = (next: boolean) => {
    if (!next) {
      setLink(null);
      setCopied(false);
    }
    onOpenChange(next);
  };
  const items = shares.data?.data.items ?? [];

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('artifacts.share.title')}</DialogTitle>
          <DialogDescription>{t('artifacts.share.description', { number: version })}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          {link ? (
            <div className="space-y-2">
              <p className="text-xs text-muted-foreground">{t('artifacts.share.onlyOnce')}</p>
              <div className="flex gap-2">
                <Input readOnly value={link} onFocus={event => event.currentTarget.select()} className="font-mono text-xs" />
                <Button variant="outline" onClick={() => void copy()}>
                  {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
                  {t('artifacts.share.copy')}
                </Button>
              </div>
            </div>
          ) : (
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-sm">{t('artifacts.share.lifetime')}</span>
              <Select value={lifetime} onValueChange={value => setLifetime(value as DigitalHumanArtifactShareLifetime)}>
                <SelectTrigger className="h-8 w-32 text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {lifetimes.map(item => (
                    <SelectItem key={item} value={item}>
                      {t(`artifacts.share.lifetimes.${item}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button size="sm" className="ml-auto" disabled={create.isPending} onClick={() => create.mutate()}>
                <Link2 className="size-4" />
                {t('artifacts.share.create')}
              </Button>
            </div>
          )}
          {items.length > 0 ? (
            <section>
              <h3 className="text-xs font-medium text-muted-foreground">{t('artifacts.share.existing')}</h3>
              <ul className="mt-2 divide-y rounded-lg border">
                {items.map(item => (
                  <ShareRow key={item.id} share={item} locale={locale} revoking={revoke.isPending} onRevoke={() => revoke.mutate(item.id)} />
                ))}
              </ul>
            </section>
          ) : null}
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" onClick={() => close(false)}>
            {t('artifacts.share.done')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ShareRow({
  share,
  locale,
  revoking,
  onRevoke,
}: {
  share: DigitalHumanArtifactShare;
  locale: string;
  revoking: boolean;
  onRevoke: () => void;
}) {
  const t = useT('digitalHumans');
  const expired = Boolean(share.expires_at && new Date(share.expires_at).getTime() <= Date.now());
  const status = share.revoked_at
    ? t('artifacts.share.status.revoked')
    : expired
      ? t('artifacts.share.status.expired')
      : share.expires_at
        ? t('artifacts.share.status.until', { time: formatTaskTime(share.expires_at, locale) })
        : t('artifacts.share.status.forever');
  return (
    <li className="flex items-center gap-3 px-3 py-2 text-xs">
      <span className="w-8 shrink-0 font-medium tabular-nums">v{share.version}</span>
      <span className="min-w-0 flex-1 truncate">
        {status}
        <span className="text-muted-foreground"> · {t('artifacts.share.opened', { count: share.access_count })}</span>
      </span>
      {!share.revoked_at && !expired ? (
        <Button size="sm" variant="ghost" className="h-7 px-2 text-xs" disabled={revoking} onClick={onRevoke}>
          {t('artifacts.share.revoke')}
        </Button>
      ) : null}
    </li>
  );
}
