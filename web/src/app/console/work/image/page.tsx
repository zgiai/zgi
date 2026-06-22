'use client';

import * as React from 'react';
import { SysImage } from '@/components/chat/variants/img/sys-image';
import { SingleChatController } from '@/components/chat/controllers/single-chat-controller';
import { useBuiltInWorkflows } from '@/hooks/workflow/use-built-in-workflows';
import { useWebappConversationTransport } from '@/hooks/webapp/use-webapp-transport';
import { useInitializeDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import { useCurrentUser } from '@/store/auth-store';
import { getLastSelectedAiModel, saveLastSelectedAiModel } from '@/utils/ui-local';
import type { ModelSelectorParameterValue } from '@/components/common/model-selector/model-selector-parameter';
import { useRouter, useSearchParams } from 'next/navigation';
import { useStore } from 'zustand';

import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import { AlertCircle, RefreshCw } from 'lucide-react';
import { useT } from '@/i18n/translations';
import { WorkflowPrecheckWarningBanner } from '@/components/workflow/common/workflow-precheck-warning';

interface ImageInitializationErrorProps {
  type: 'load-failed' | 'config-missing';
  isRetrying: boolean;
  onRetry: () => void;
}

function ImageInitializationError({ type, isRetrying, onRetry }: ImageInitializationErrorProps) {
  const t = useT('webapp');
  const isLoadFailed = type === 'load-failed';

  return (
    <div className="flex h-full w-full items-center justify-center bg-background p-6">
      <div
        role="alert"
        className="flex w-full max-w-[560px] gap-4 rounded-[4px] border border-border bg-card p-5 shadow-premium"
      >
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-muted">
          <AlertCircle className="h-5 w-5 text-muted-foreground" />
        </div>
        <div className="min-w-0 flex-1">
          <h2 className="text-base font-semibold text-foreground">
            {isLoadFailed
              ? t('chat.imageGenLoadFailed.title')
              : t('chat.imageGenConfigMissing.title')}
          </h2>
          <p className="mt-2 text-sm leading-6 text-muted-foreground">
            {isLoadFailed
              ? t('chat.imageGenLoadFailed.description')
              : t('chat.imageGenConfigMissing.description')}
          </p>
          <p className="mt-3 rounded-[4px] bg-muted px-3 py-2 text-xs leading-5 text-muted-foreground">
            {isLoadFailed
              ? t('chat.imageGenLoadFailed.detail')
              : t('chat.imageGenConfigMissing.detail')}
          </p>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="mt-4 gap-2"
            onClick={onRetry}
            disabled={isRetrying}
          >
            <RefreshCw className={isRetrying ? 'h-4 w-4 animate-spin' : 'h-4 w-4'} />
            {t('chat.retry')}
          </Button>
        </div>
      </div>
    </div>
  );
}

function ImagePageContent() {
  const t = useT('webapp');
  const {
    imageGenChatWorkflow,
    isLoading,
    isFetching,
    error: workflowError,
    refetch: refetchBuiltInWorkflows,
  } = useBuiltInWorkflows();
  const webAppId = imageGenChatWorkflow?.web_app_id;
  const user = useCurrentUser();
  const router = useRouter();
  const searchParams = useSearchParams();

  // Model config state - initialize from saved preference
  const [modelSelectorValue, setModelSelectorValue] = React.useState<ModelSelectorParameterValue>(
    () => {
      if (!user?.id) return { provider: '', model: '', params: {} };
      const saved = getLastSelectedAiModel(user.id, 'imageGenChat');
      return saved
        ? { provider: saved.provider, model: saved.model, params: {} }
        : { provider: '', model: '', params: {} };
    }
  );

  // Apply default model when loaded (only if no saved preference)
  useInitializeDefaultModelByUseCase({
    useCase: 'image-gen',
    currentModel: modelSelectorValue,
    enabled: Boolean(user?.id && !getLastSelectedAiModel(user.id, 'imageGenChat')),
    onInitialize: v => {
      setModelSelectorValue({
        provider: v.provider,
        model: v.model,
        params: v.params,
      });
    },
  });

  const handleModelChange = React.useCallback(
    (value: { provider: string; model: string }) => {
      setModelSelectorValue(prev => ({
        ...prev,
        provider: value.provider,
        model: value.model,
      }));
      // Persist selection for this user
      if (user?.id) {
        saveLastSelectedAiModel(user.id, 'imageGenChat', {
          provider: value.provider,
          model: value.model,
        });
      }
    },
    [user?.id]
  );

  const { transport, precheckWarnings } = useWebappConversationTransport(webAppId ?? '', {
    enablePrecheck: true,
  });

  // Use a ref to store the controller instance to prevent unnecessary re-instantiation
  const controllerRef = React.useRef<SingleChatController | null>(null);

  const controller = React.useMemo(() => {
    if (!webAppId) return null;

    if (controllerRef.current) {
      return controllerRef.current;
    }

    const ctrl = new SingleChatController(transport);
    controllerRef.current = ctrl;
    return ctrl;
  }, [webAppId]); // Only recreate if webAppId changes

  // Effect to update transport when it changes
  React.useEffect(() => {
    if (controller && transport) {
      controller.updateTransport(transport);
    }
  }, [controller, transport]);

  React.useEffect(() => {
    if (controller) {
      const convId = searchParams.get('convId');
      if (convId) {
        controller.init(convId);
      } else {
        controller.init();
      }
    }
  }, [controller]);

  // Sync activeId to URL
  const activeId = useStore(
    controller?.store || {
      getState: () => ({ activeId: null }),
      subscribe: () => () => () => {},
      getInitialState: () => ({ activeId: null }),
    },
    s => s.activeId
  );

  React.useEffect(() => {
    if (activeId && !activeId.startsWith('draft-')) {
      const params = new URLSearchParams(searchParams.toString());
      if (params.get('convId') !== activeId) {
        params.set('convId', activeId);
        router.replace(`?${params.toString()}`);
      }
    } else if (!activeId || activeId.startsWith('draft-')) {
      const params = new URLSearchParams(searchParams.toString());
      if (params.has('convId')) {
        params.delete('convId');
        router.replace(`?${params.toString()}`);
      }
    }
  }, [activeId, router, searchParams]);

  if (isLoading) {
    return (
      <div className="flex h-full w-full items-center justify-center p-4">
        <div className="flex flex-col items-center gap-4">
          <Skeleton className="h-12 w-12 rounded-full" />
          <div className="text-sm text-muted-foreground">{t('chat.loadingImageGen')}</div>
        </div>
      </div>
    );
  }

  if (!controller) {
    return (
      <ImageInitializationError
        type={workflowError ? 'load-failed' : 'config-missing'}
        isRetrying={isFetching}
        onRetry={() => {
          void refetchBuiltInWorkflows();
        }}
      />
    );
  }

  return (
    <div className="h-full w-full">
      <SysImage
        controller={controller}
        isLoading={isLoading}
        modelSelectorValue={modelSelectorValue}
        onModelChange={handleModelChange}
        conversationSearchKey={['webapp', 'conversations', webAppId ?? 'image', 'search']}
        inputTopNotice={
          precheckWarnings.length > 0 ? (
            <WorkflowPrecheckWarningBanner
              warnings={precheckWarnings}
              scope="webapp"
              storageKey={`console-image-chat:${webAppId ?? 'image'}`}
            />
          ) : null
        }
      />
    </div>
  );
}

export default function ImagePage() {
  return (
    <React.Suspense fallback={null}>
      <ImagePageContent />
    </React.Suspense>
  );
}
