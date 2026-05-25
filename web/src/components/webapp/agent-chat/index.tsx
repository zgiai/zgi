'use client';

import { useEffect, useMemo, useState } from 'react';
import Chat, { createAgentWebAppTransport, useAIChatController } from '@/components/chat';
import { IconPreview } from '@/components/common/icon-input/icon-preview';
import type { ModelSelectorValue } from '@/components/common/model-selector';
import { ICON_BG } from '@/lib/config';
import type { WebAppWorkflowConfig } from '@/services/types/webapp';

interface AgentWebappChatProps {
  webAppId: string;
  config: WebAppWorkflowConfig;
}

function toModelParams(
  params: Record<string, unknown> | undefined
): Record<string, number | string | boolean> {
  const next: Record<string, number | string | boolean> = {};
  for (const [key, value] of Object.entries(params ?? {})) {
    if (typeof value === 'number' || typeof value === 'string' || typeof value === 'boolean') {
      next[key] = value;
    }
  }
  return next;
}

function resolveWebAppIcon(config: WebAppWorkflowConfig) {
  const meta = config.config;
  const title = meta?.title || 'Agent';
  let iconType: 'image' | 'text' = meta?.icon_type === 'image' ? 'image' : 'text';
  let src = '';
  let icon = title.slice(0, 2).toUpperCase();
  let iconBackground = ICON_BG;

  if (meta?.icon_type === 'image') {
    src = meta.icon_url || meta.icon || '';
  } else if (meta?.icon) {
    try {
      const parsed = JSON.parse(meta.icon) as { icon?: string; icon_background?: string };
      icon = parsed.icon || icon;
      iconBackground = parsed.icon_background || iconBackground;
    } catch {
      iconType = 'text';
    }
  }

  return { iconType, src, icon, iconBackground };
}

export default function AgentWebappChat({ webAppId, config }: AgentWebappChatProps) {
  const title = config.config?.title || 'Agent';
  const agentConfig = config.agent_config;
  const iconPreview = useMemo(() => resolveWebAppIcon(config), [config]);
  const transport = useMemo(() => createAgentWebAppTransport(webAppId), [webAppId]);
  const controller = useAIChatController({ transport });
  const initController = controller.init;
  const [modelValue, setModelValue] = useState({
    provider: agentConfig?.model_provider ?? '',
    model: agentConfig?.model ?? '',
    params: toModelParams(agentConfig?.model_parameters),
  });

  useEffect(() => {
    initController(null);
  }, [initController]);

  useEffect(() => {
    setModelValue({
      provider: agentConfig?.model_provider ?? '',
      model: agentConfig?.model ?? '',
      params: toModelParams(agentConfig?.model_parameters),
    });
  }, [agentConfig?.model, agentConfig?.model_parameters, agentConfig?.model_provider]);

  const handleModelChange = (value: ModelSelectorValue) => {
    setModelValue(current => ({
      provider: value.provider,
      model: value.model,
      params: current.params,
    }));
  };

  return (
    <Chat
      mode="aichat"
      controller={controller}
      modelSelectorValue={modelValue}
      onModelChange={handleModelChange}
      variant="full"
      showModelSelector={false}
      showMemoryToggle={false}
      forcedUseMemory={Boolean(agentConfig?.use_memory)}
      enableUpload={Boolean(agentConfig?.file_upload_enabled)}
      showFileLibraryPicker={false}
      suggestions={agentConfig?.suggested_questions ?? []}
      homeBrand={
        <div className="flex size-16 items-center justify-center rounded-2xl border bg-background shadow-sm">
          <IconPreview
            iconType={iconPreview.iconType}
            src={iconPreview.src}
            icon={iconPreview.icon}
            iconBackground={iconPreview.iconBackground}
            editable={false}
            size="lg"
          />
        </div>
      }
      homeTitle={title}
      homeDescription="开始新的 Agent 对话"
    />
  );
}
