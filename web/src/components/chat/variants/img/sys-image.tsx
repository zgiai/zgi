'use client';

import * as React from 'react';
import Chat from '../..';
import { Skeleton } from '@/components/ui/skeleton';
import { useT } from '@/i18n/translations';
import type { ModelSelectorValue } from '@/components/common/model-selector';
import type { SingleChatController } from '@/components/chat/controllers/single-chat-controller';
import type { ImageRuntimeModel } from '@/services/types/image-runtime';

export interface SysImageProps {
  controller: SingleChatController | null;
  isLoading: boolean;
  modelSelectorValue: ModelSelectorValue;
  onModelChange: (value: ModelSelectorValue) => void;
  inputTopNotice?: React.ReactNode;
  conversationSearchKey?: readonly unknown[];
  imageRuntimeModels?: ImageRuntimeModel[];
}

export function SysImage({
  controller,
  isLoading,
  modelSelectorValue,
  onModelChange,
  inputTopNotice,
  conversationSearchKey,
  imageRuntimeModels,
}: SysImageProps) {
  const t = useT('webapp');

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
      <div className="flex h-full w-full items-center justify-center p-4">
        <div className="text-sm text-muted-foreground">
          {t('chat.imageGenNotConfigured')}
        </div>
      </div>
    );
  }

  return (
    <Chat
      mode="img"
      controller={controller}
      modelSelectorValue={modelSelectorValue}
      onModelChange={onModelChange}
      inputTopNotice={inputTopNotice}
      conversationSearchKey={conversationSearchKey}
      imageRuntimeModels={imageRuntimeModels}
    />
  );
}
