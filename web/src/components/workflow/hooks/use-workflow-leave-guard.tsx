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
import { useWorkflowStore } from '../store';
import { flushWorkflowPendingEdits } from './pending-edits';

interface WorkflowLeaveGuardOptions {
  enabled: boolean;
  shouldGuard: boolean;
  confirmOnAnyNavigation?: boolean;
  isValid: boolean;
  isSaving: boolean;
  onSave: (options?: { silent?: boolean }) => Promise<void> | void;
}

interface PendingNavigation {
  type: 'link' | 'popstate';
  href?: string;
  hasUnsavedChanges: boolean;
  reason: 'unsaved' | 'saving' | 'confirm';
  retry: () => void;
}

interface WorkflowLeaveGuardDialogProps {
  open: boolean;
  hasUnsavedChanges: boolean;
  isValid: boolean;
  isSaving: boolean;
  onOpenChange: (open: boolean) => void;
  onSaveAndLeave: () => void;
  onDiscardAndLeave: () => void;
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

/**
 * @component WorkflowLeaveGuardDialog
 * @category Feature
 * @status Stable
 * @description Confirmation dialog shown before leaving a workflow with unsaved changes
 * @usage Render from useWorkflowLeaveGuard inside the workflow editor
 * @example
 * <WorkflowLeaveGuardDialog open={open} isValid={isValid} isSaving={isSaving} />
 */
function WorkflowLeaveGuardDialog({
  open,
  hasUnsavedChanges,
  isValid,
  isSaving,
  onOpenChange,
  onSaveAndLeave,
  onDiscardAndLeave,
}: WorkflowLeaveGuardDialogProps) {
  const t = useT('agents');

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="sm" className="overflow-hidden p-0" showCloseButton={!isSaving}>
        <DialogHeader className="pb-3">
          <div className="flex items-center gap-3">
            <div className="flex size-9 shrink-0 items-center justify-center rounded-full bg-amber-500/10 text-amber-600">
              <AlertTriangle className="size-5" />
            </div>
            <DialogTitle className="text-base">
              {hasUnsavedChanges
                ? isValid
                  ? t('workflow.leaveGuard.title')
                  : t('workflow.leaveGuard.invalidTitle')
                : t('workflow.leaveGuard.confirmTitle')}
            </DialogTitle>
          </div>
        </DialogHeader>
        <DialogBody className="pt-0">
          <DialogDescription className="text-sm leading-6">
            {hasUnsavedChanges
              ? isValid
                ? t('workflow.leaveGuard.description')
                : t('workflow.leaveGuard.invalidDescription')
              : t('workflow.leaveGuard.confirmDescription')}
          </DialogDescription>
        </DialogBody>
        <DialogFooter className="border-none px-6 pb-6 pt-3">
          {hasUnsavedChanges ? (
            <>
              <Button variant="ghost" disabled={isSaving} onClick={onDiscardAndLeave}>
                {t('workflow.leaveGuard.discard')}
              </Button>
              {isValid ? (
                <Button disabled={isSaving} onClick={onSaveAndLeave}>
                  {isSaving ? <Loader2 className="size-4 animate-spin" /> : null}
                  {t('workflow.leaveGuard.saveAndLeave')}
                </Button>
              ) : (
                <Button disabled={isSaving} onClick={() => onOpenChange(false)}>
                  {t('workflow.leaveGuard.continueEditing')}
                </Button>
              )}
            </>
          ) : (
            <>
              <Button variant="ghost" disabled={isSaving} onClick={() => onOpenChange(false)}>
                {t('workflow.leaveGuard.continueEditing')}
              </Button>
              <Button disabled={isSaving} onClick={onDiscardAndLeave}>
                {t('workflow.leaveGuard.leave')}
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * @hook useWorkflowLeaveGuard
 * @description Guards browser and in-app navigation when the workflow editor has unsaved changes
 */
export function useWorkflowLeaveGuard({
  enabled,
  shouldGuard,
  confirmOnAnyNavigation = false,
  isValid,
  isSaving,
  onSave,
}: WorkflowLeaveGuardOptions): React.ReactNode {
  const router = useRouter();
  const [pendingNavigation, setPendingNavigation] = React.useState<PendingNavigation | null>(null);
  const pendingNavigationRef = React.useRef<PendingNavigation | null>(null);
  const ignoreNextPopRef = React.useRef(false);
  const enabledRef = React.useRef(enabled);
  const shouldGuardRef = React.useRef(shouldGuard);
  const confirmOnAnyNavigationRef = React.useRef(confirmOnAnyNavigation);
  const isSavingRef = React.useRef(isSaving);

  React.useEffect(() => {
    enabledRef.current = enabled;
  }, [enabled]);

  React.useEffect(() => {
    shouldGuardRef.current = shouldGuard;
  }, [shouldGuard]);

  React.useEffect(() => {
    confirmOnAnyNavigationRef.current = confirmOnAnyNavigation;
  }, [confirmOnAnyNavigation]);

  React.useEffect(() => {
    isSavingRef.current = isSaving;
  }, [isSaving]);

  const getHasUnsavedChanges = React.useCallback(() => {
    if (!enabledRef.current || !shouldGuardRef.current) return false;
    flushWorkflowPendingEdits();
    const state = useWorkflowStore.getState();
    return Boolean(state.isDirty || state.hasLayoutChanges);
  }, []);

  const getNavigationBlockState = React.useCallback(() => {
    const hasUnsavedChanges = getHasUnsavedChanges();
    if (hasUnsavedChanges) {
      return { shouldBlock: true, hasUnsavedChanges, reason: 'unsaved' as const };
    }
    if (enabledRef.current && shouldGuardRef.current && isSavingRef.current) {
      return { shouldBlock: true, hasUnsavedChanges: false, reason: 'saving' as const };
    }
    if (enabledRef.current && confirmOnAnyNavigationRef.current) {
      return { shouldBlock: true, hasUnsavedChanges: false, reason: 'confirm' as const };
    }
    return {
      shouldBlock: false,
      hasUnsavedChanges: false,
      reason: 'confirm' as const,
    };
  }, [getHasUnsavedChanges]);

  const setPending = React.useCallback((navigation: PendingNavigation | null) => {
    pendingNavigationRef.current = navigation;
    setPendingNavigation(navigation);
  }, []);

  const closeGuard = React.useCallback(() => {
    if (isSaving) return;
    setPending(null);
  }, [isSaving, setPending]);

  const runPendingNavigation = React.useCallback(() => {
    const navigation = pendingNavigationRef.current;
    if (!navigation) return;
    setPending(null);
    navigation.retry();
  }, [setPending]);

  React.useEffect(() => {
    const navigation = pendingNavigationRef.current;
    if (!navigation || navigation.reason !== 'saving' || isSaving) return;

    flushWorkflowPendingEdits();
    const state = useWorkflowStore.getState();

    if (state.isDirty || state.hasLayoutChanges) {
      setPending({
        ...navigation,
        hasUnsavedChanges: true,
        reason: 'unsaved',
      });
      return;
    }

    runPendingNavigation();
  }, [isSaving, runPendingNavigation, setPending]);

  const handleDiscardAndLeave = React.useCallback(() => {
    runPendingNavigation();
  }, [runPendingNavigation]);

  const handleSaveAndLeave = React.useCallback(async () => {
    const navigation = pendingNavigationRef.current;
    if (!navigation) return;

    flushWorkflowPendingEdits();
    await onSave({ silent: true });

    const state = useWorkflowStore.getState();
    if (state.isDirty || state.hasLayoutChanges) return;

    runPendingNavigation();
  }, [onSave, runPendingNavigation]);

  React.useEffect(() => {
    if (!enabled) return;

    const handleBeforeUnload = (event: BeforeUnloadEvent) => {
      if (!getHasUnsavedChanges() && !isSavingRef.current) return;
      event.preventDefault();
      event.returnValue = '';
    };

    window.addEventListener('beforeunload', handleBeforeUnload);

    return () => {
      window.removeEventListener('beforeunload', handleBeforeUnload);
    };
  }, [enabled, getHasUnsavedChanges]);

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
      const blockState = getNavigationBlockState();
      if (!blockState.shouldBlock) return;

      event.preventDefault();
      event.stopPropagation();

      const href = `${url.pathname}${url.search}${url.hash}`;
      setPending({
        type: 'link',
        href,
        hasUnsavedChanges: blockState.hasUnsavedChanges,
        reason: blockState.reason,
        retry: () => router.push(href),
      });
    };

    document.addEventListener('click', handleClick, true);

    return () => {
      document.removeEventListener('click', handleClick, true);
    };
  }, [enabled, getNavigationBlockState, router, setPending]);

  React.useEffect(() => {
    if (!enabled) return;

    const sentinelId = `${Date.now()}-${Math.random().toString(36).slice(2)}`;
    const initialState =
      typeof window.history.state === 'object' && window.history.state !== null
        ? window.history.state
        : {};

    window.history.replaceState(
      { ...initialState, __workflowLeaveGuardBase: sentinelId },
      '',
      window.location.href
    );
    window.history.pushState(
      { ...initialState, __workflowLeaveGuard: sentinelId },
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

      const blockState = getNavigationBlockState();
      if (!blockState.shouldBlock) {
        continueBack();
        return;
      }

      window.history.pushState(
        { ...(window.history.state ?? {}), __workflowLeaveGuard: sentinelId },
        '',
        window.location.href
      );
      setPending({
        type: 'popstate',
        hasUnsavedChanges: blockState.hasUnsavedChanges,
        reason: blockState.reason,
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
  }, [enabled, getNavigationBlockState, setPending]);

  return (
    <WorkflowLeaveGuardDialog
      open={Boolean(pendingNavigation)}
      hasUnsavedChanges={Boolean(pendingNavigation?.hasUnsavedChanges)}
      isValid={isValid}
      isSaving={isSaving}
      onOpenChange={open => {
        if (!open) closeGuard();
      }}
      onSaveAndLeave={handleSaveAndLeave}
      onDiscardAndLeave={handleDiscardAndLeave}
    />
  );
}
