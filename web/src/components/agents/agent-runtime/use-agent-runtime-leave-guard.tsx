'use client';

import React from 'react';
import { useRouter } from 'next/navigation';
import { AlertTriangle, Loader2 } from 'lucide-react';
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

interface UseAgentRuntimeLeaveGuardOptions {
  enabled: boolean;
  hasUnsavedChanges: boolean;
  isSaving: boolean;
  onSave: () => Promise<boolean> | boolean;
}

interface PendingNavigation {
  type: 'link' | 'popstate';
  reason: 'unsaved' | 'saving';
  retry: () => void;
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

interface AgentRuntimeLeaveGuardDialogProps {
  open: boolean;
  isSaving: boolean;
  onOpenChange: (open: boolean) => void;
  onSaveAndLeave: () => void;
  onDiscardAndLeave: () => void;
}

function AgentRuntimeLeaveGuardDialog({
  open,
  isSaving,
  onOpenChange,
  onSaveAndLeave,
  onDiscardAndLeave,
}: AgentRuntimeLeaveGuardDialogProps) {
  const t = useT('agents.agentRuntime');

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="sm" className="overflow-hidden p-0" showCloseButton={!isSaving}>
        <DialogHeader className="pb-3">
          <div className="flex items-center gap-3">
            <div className="flex size-9 shrink-0 items-center justify-center rounded-full bg-amber-500/10 text-amber-600">
              <AlertTriangle className="size-5" />
            </div>
            <DialogTitle className="text-base">{t('leaveGuard.title')}</DialogTitle>
          </div>
        </DialogHeader>
        <DialogBody className="pt-0">
          <DialogDescription className="text-sm leading-6">
            {t('leaveGuard.description')}
          </DialogDescription>
        </DialogBody>
        <DialogFooter className="border-none px-6 pb-6 pt-3">
          <Button variant="ghost" disabled={isSaving} onClick={onDiscardAndLeave}>
            {t('leaveGuard.discardAndLeave')}
          </Button>
          <Button variant="outline" disabled={isSaving} onClick={() => onOpenChange(false)}>
            {t('leaveGuard.continueEditing')}
          </Button>
          <Button disabled={isSaving} onClick={onSaveAndLeave}>
            {isSaving ? <Loader2 className="size-4 animate-spin" /> : null}
            {t('leaveGuard.saveAndLeave')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function useAgentRuntimeLeaveGuard({
  enabled,
  hasUnsavedChanges,
  isSaving,
  onSave,
}: UseAgentRuntimeLeaveGuardOptions): React.ReactNode {
  const router = useRouter();
  const [pendingNavigation, setPendingNavigation] = React.useState<PendingNavigation | null>(null);
  const pendingNavigationRef = React.useRef<PendingNavigation | null>(null);
  const ignoreNextPopRef = React.useRef(false);
  const enabledRef = React.useRef(enabled);
  const hasUnsavedChangesRef = React.useRef(hasUnsavedChanges);
  const isSavingRef = React.useRef(isSaving);
  const onSaveRef = React.useRef(onSave);

  React.useEffect(() => {
    enabledRef.current = enabled;
  }, [enabled]);

  React.useEffect(() => {
    hasUnsavedChangesRef.current = hasUnsavedChanges;
  }, [hasUnsavedChanges]);

  React.useEffect(() => {
    isSavingRef.current = isSaving;
  }, [isSaving]);

  React.useEffect(() => {
    onSaveRef.current = onSave;
  }, [onSave]);

  const setPending = React.useCallback((navigation: PendingNavigation | null) => {
    pendingNavigationRef.current = navigation;
    setPendingNavigation(navigation);
  }, []);

  const runPendingNavigation = React.useCallback(() => {
    const navigation = pendingNavigationRef.current;
    if (!navigation) return;
    setPending(null);
    navigation.retry();
  }, [setPending]);

  const getBlockReason = React.useCallback((): PendingNavigation['reason'] | null => {
    if (!enabledRef.current) return null;
    if (hasUnsavedChangesRef.current) return 'unsaved';
    if (isSavingRef.current) return 'saving';
    return null;
  }, []);

  React.useEffect(() => {
    const navigation = pendingNavigationRef.current;
    if (!navigation || navigation.reason !== 'saving' || isSaving) return;
    if (hasUnsavedChangesRef.current) {
      setPending({ ...navigation, reason: 'unsaved' });
      return;
    }
    runPendingNavigation();
  }, [isSaving, runPendingNavigation, setPending]);

  const handleSaveAndLeave = React.useCallback(async () => {
    const saved = await onSaveRef.current();
    if (!saved) return;
    runPendingNavigation();
  }, [runPendingNavigation]);

  React.useEffect(() => {
    if (!enabled) return;

    const handleBeforeUnload = (event: BeforeUnloadEvent) => {
      if (!hasUnsavedChangesRef.current && !isSavingRef.current) return;
      event.preventDefault();
      event.returnValue = '';
    };

    window.addEventListener('beforeunload', handleBeforeUnload);
    return () => {
      window.removeEventListener('beforeunload', handleBeforeUnload);
    };
  }, [enabled]);

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

      const reason = getBlockReason();
      if (!reason) return;

      event.preventDefault();
      event.stopPropagation();

      const href = `${url.pathname}${url.search}${url.hash}`;
      setPending({
        type: 'link',
        reason,
        retry: () => router.push(href),
      });
    };

    document.addEventListener('click', handleClick, true);
    return () => {
      document.removeEventListener('click', handleClick, true);
    };
  }, [enabled, getBlockReason, router, setPending]);

  React.useEffect(() => {
    if (!enabled) return;

    const sentinelId = `${Date.now()}-${Math.random().toString(36).slice(2)}`;
    const initialState =
      typeof window.history.state === 'object' && window.history.state !== null
        ? window.history.state
        : {};

    window.history.replaceState(
      { ...initialState, __agentRuntimeLeaveGuardBase: sentinelId },
      '',
      window.location.href
    );
    window.history.pushState(
      { ...initialState, __agentRuntimeLeaveGuard: sentinelId },
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

      const reason = getBlockReason();
      if (!reason) {
        continueBack();
        return;
      }

      window.history.pushState(
        { ...(window.history.state ?? {}), __agentRuntimeLeaveGuard: sentinelId },
        '',
        window.location.href
      );
      setPending({
        type: 'popstate',
        reason,
        retry: () => {
          ignoreNextPopRef.current = true;
          window.history.go(-2);
        },
      });
    };

    window.addEventListener('popstate', handlePopState);
    return () => {
      window.removeEventListener('popstate', handlePopState);
    };
  }, [enabled, getBlockReason, setPending]);

  return (
    <AgentRuntimeLeaveGuardDialog
      open={Boolean(pendingNavigation)}
      isSaving={isSaving}
      onOpenChange={open => {
        if (!open && !isSaving) setPending(null);
      }}
      onSaveAndLeave={handleSaveAndLeave}
      onDiscardAndLeave={runPendingNavigation}
    />
  );
}
