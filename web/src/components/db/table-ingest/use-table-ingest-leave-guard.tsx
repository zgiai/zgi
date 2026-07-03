'use client';

import React from 'react';
import { useRouter } from 'next/navigation';
import { AlertTriangle } from 'lucide-react';
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
import { useT } from '@/i18n';

type LeaveGuardReason = 'processing' | 'unsaved';

interface UseTableIngestLeaveGuardOptions {
  enabled: boolean;
  reason: LeaveGuardReason | null;
}

interface PendingNavigation {
  retry: () => void;
  reason: LeaveGuardReason;
}

function isPlainLeftClick(event: MouseEvent): boolean {
  return event.button === 0 && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey;
}

function isSameDocumentNavigation(url: URL): boolean {
  return (
    url.origin === window.location.origin &&
    url.pathname === window.location.pathname &&
    url.search === window.location.search
  );
}

function shouldIgnoreAnchor(anchor: HTMLAnchorElement): boolean {
  const href = anchor.getAttribute('href');
  if (!href || href.startsWith('#')) return true;
  if (anchor.target && anchor.target !== '_self') return true;
  if (anchor.hasAttribute('download')) return true;
  return false;
}

export function useTableIngestLeaveGuard({
  enabled,
  reason,
}: UseTableIngestLeaveGuardOptions): {
  leaveGuardDialog: React.ReactNode;
  confirmNavigation: (retry: () => void) => void;
} {
  const t = useT('dbs');
  const router = useRouter();
  const [pendingNavigation, setPendingNavigation] = React.useState<PendingNavigation | null>(null);
  const pendingNavigationRef = React.useRef<PendingNavigation | null>(null);
  const ignoreNextPopRef = React.useRef(false);
  const enabledRef = React.useRef(enabled);
  const reasonRef = React.useRef(reason);

  React.useEffect(() => {
    enabledRef.current = enabled;
  }, [enabled]);

  React.useEffect(() => {
    reasonRef.current = reason;
  }, [reason]);

  const setPending = React.useCallback((navigation: PendingNavigation | null) => {
    pendingNavigationRef.current = navigation;
    setPendingNavigation(navigation);
  }, []);

  const getBlockReason = React.useCallback((): LeaveGuardReason | null => {
    if (!enabledRef.current) return null;
    return reasonRef.current;
  }, []);

  const runPendingNavigation = React.useCallback(() => {
    const navigation = pendingNavigationRef.current;
    if (!navigation) return;
    setPending(null);
    navigation.retry();
  }, [setPending]);

  const confirmNavigation = React.useCallback(
    (retry: () => void) => {
      const blockReason = getBlockReason();
      if (!blockReason) {
        retry();
        return;
      }
      setPending({ retry, reason: blockReason });
    },
    [getBlockReason, setPending]
  );

  React.useEffect(() => {
    if (!enabled) {
      setPending(null);
      return;
    }

    const handleBeforeUnload = (event: BeforeUnloadEvent) => {
      if (!getBlockReason()) return;
      event.preventDefault();
      event.returnValue = '';
    };

    window.addEventListener('beforeunload', handleBeforeUnload);
    return () => window.removeEventListener('beforeunload', handleBeforeUnload);
  }, [enabled, getBlockReason, setPending]);

  React.useEffect(() => {
    if (!enabled) return;

    const handleClick = (event: MouseEvent) => {
      if (event.defaultPrevented || !isPlainLeftClick(event)) return;
      const target = event.target;
      if (!(target instanceof Element)) return;

      const anchor = target.closest('a[href]');
      if (!(anchor instanceof HTMLAnchorElement) || shouldIgnoreAnchor(anchor)) return;

      const url = new URL(anchor.href, window.location.href);
      if (url.origin !== window.location.origin || isSameDocumentNavigation(url)) return;

      const blockReason = getBlockReason();
      if (!blockReason) return;

      event.preventDefault();
      event.stopPropagation();

      const href = `${url.pathname}${url.search}${url.hash}`;
      setPending({
        reason: blockReason,
        retry: () => router.push(href),
      });
    };

    document.addEventListener('click', handleClick, true);
    return () => document.removeEventListener('click', handleClick, true);
  }, [enabled, getBlockReason, router, setPending]);

  React.useEffect(() => {
    if (!enabled) return;

    const sentinelId = `${Date.now()}-${Math.random().toString(36).slice(2)}`;
    const initialState =
      typeof window.history.state === 'object' && window.history.state !== null
        ? window.history.state
        : {};

    window.history.replaceState(
      { ...initialState, __tableIngestLeaveGuardBase: sentinelId },
      '',
      window.location.href
    );
    window.history.pushState(
      { ...initialState, __tableIngestLeaveGuard: sentinelId },
      '',
      window.location.href
    );

    const continueBack = () => {
      window.setTimeout(() => {
        ignoreNextPopRef.current = true;
        window.history.back();
      }, 0);
    };

    const handlePopState = () => {
      if (ignoreNextPopRef.current) {
        ignoreNextPopRef.current = false;
        return;
      }

      const blockReason = getBlockReason();
      if (!blockReason) {
        continueBack();
        return;
      }

      window.history.pushState(
        { ...(window.history.state ?? {}), __tableIngestLeaveGuard: sentinelId },
        '',
        window.location.href
      );
      setPending({
        reason: blockReason,
        retry: () => {
          ignoreNextPopRef.current = true;
          window.history.go(-2);
        },
      });
    };

    window.addEventListener('popstate', handlePopState);
    return () => window.removeEventListener('popstate', handlePopState);
  }, [enabled, getBlockReason, setPending]);

  const dialogReason = pendingNavigation?.reason ?? reason ?? 'unsaved';

  const leaveGuardDialog = (
    <Dialog open={Boolean(pendingNavigation)} onOpenChange={open => !open && setPending(null)}>
      <DialogContent size="sm" className="overflow-hidden p-0">
        <DialogHeader className="pb-3">
          <div className="flex items-center gap-3">
            <div className="flex size-9 shrink-0 items-center justify-center rounded-full bg-amber-500/10 text-amber-600">
              <AlertTriangle className="size-5" />
            </div>
            <DialogTitle className="text-base">
              {dialogReason === 'processing'
                ? t('dataIngestPage.leaveGuard.processingTitle')
                : t('dataIngestPage.leaveGuard.unsavedTitle')}
            </DialogTitle>
          </div>
        </DialogHeader>
        <DialogBody className="pt-0">
          <DialogDescription className="text-sm leading-6">
            {dialogReason === 'processing'
              ? t('dataIngestPage.leaveGuard.processingDescription')
              : t('dataIngestPage.leaveGuard.unsavedDescription')}
          </DialogDescription>
        </DialogBody>
        <DialogFooter className="border-none px-6 pb-6 pt-3">
          <Button variant="ghost" onClick={() => setPending(null)}>
            {t('dataIngestPage.leaveGuard.continueReview')}
          </Button>
          <Button variant="destructive" onClick={runPendingNavigation}>
            {t('dataIngestPage.leaveGuard.leaveAndDiscard')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );

  return { leaveGuardDialog, confirmNavigation };
}
