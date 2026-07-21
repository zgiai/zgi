'use client';

import * as React from 'react';
import { SysImage } from '@/components/chat/variants/img/sys-image';
import { SingleChatController } from '@/components/chat/controllers/single-chat-controller';
import { useImageRuntimeModels, IMAGE_RUNTIME_KEYS } from '@/hooks/image-runtime/use-image-runtime-models';
import { useImageRuntimeTransport } from '@/hooks/image-runtime/use-image-runtime-transport';
import { useCurrentUser } from '@/store/auth-store';
import { getLastSelectedAiModel, saveLastSelectedAiModel } from '@/utils/ui-local';
import type { ModelSelectorParameterValue } from '@/components/common/model-selector/model-selector-parameter';
import { useRouter, useSearchParams } from 'next/navigation';
import { useStore } from 'zustand';

import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import { AlertCircle, RefreshCw } from 'lucide-react';
import { useT } from '@/i18n/translations';

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
    isLoading,
    isFetching,
    error: modelsError,
    refetch: refetchModels,
    models: imageRuntimeModels,
  } = useImageRuntimeModels();
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

  React.useEffect(() => {
    if (isLoading || modelsError) {
      return;
    }

    const currentModelStillAvailable = imageRuntimeModels.some(
      item => item.provider === modelSelectorValue.provider && item.model === modelSelectorValue.model
    );
    if (currentModelStillAvailable && modelSelectorValue.model) return;

    const fallback = imageRuntimeModels[0];
    if (!fallback) return;

    setModelSelectorValue(prev => ({
      ...prev,
      provider: fallback.provider,
      model: fallback.model,
    }));

    if (user?.id) {
      saveLastSelectedAiModel(user.id, 'imageGenChat', {
        provider: fallback.provider,
        model: fallback.model,
      });
    }
  }, [
    imageRuntimeModels,
    isLoading,
    modelsError,
    modelSelectorValue.model,
    modelSelectorValue.provider,
    user?.id,
  ]);

  const { transport } = useImageRuntimeTransport({ models: imageRuntimeModels });

  // Use a ref to store the controller instance to prevent unnecessary re-instantiation
  const controllerRef = React.useRef<SingleChatController | null>(null);

  const controller = React.useMemo(() => {
    if (modelsError || imageRuntimeModels.length === 0) return null;

    if (controllerRef.current) {
      return controllerRef.current;
    }

    const ctrl = new SingleChatController(transport);
    controllerRef.current = ctrl;
    return ctrl;
  }, [imageRuntimeModels.length, modelsError, transport]);

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
        type={modelsError ? 'load-failed' : 'config-missing'}
        isRetrying={isFetching}
        onRetry={() => {
          void refetchModels();
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
        conversationSearchKey={IMAGE_RUNTIME_KEYS.search}
        imageRuntimeModels={imageRuntimeModels}
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
