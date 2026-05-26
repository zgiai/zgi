'use client';

import { useEffect, useMemo } from 'react';
import Chat, { createAgentWebAppTransport, useAIChatController } from '@/components/chat';
import { IconPreview } from '@/components/common/icon-input/icon-preview';
import { useT } from '@/i18n';
import { ICON_BG } from '@/lib/config';
import type { WebAppWorkflowConfig } from '@/services/types/webapp';

interface AgentWebappChatProps {
  webAppId: string;
  config: WebAppWorkflowConfig;
}

function resolveWebAppIcon(config: WebAppWorkflowConfig, fallbackTitle: string) {
  const meta = config.config;
  const title = meta?.title || fallbackTitle;
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
  const t = useT('webapp');
  const agentConfig = config.agent_config;
  const homeTitle = agentConfig?.home_title || t('agentChat.defaultHomeTitle');
  const inputPlaceholder = agentConfig?.input_placeholder || t('chat.enterCommand');
  const iconPreview = useMemo(
    () => resolveWebAppIcon(config, t('agentChat.fallbackTitle')),
    [config, t]
  );
  const transport = useMemo(() => createAgentWebAppTransport(webAppId), [webAppId]);
  const controller = useAIChatController({ transport });
  const initController = controller.init;
  const modelValue = useMemo(() => ({ provider: '', model: '', params: {} }), []);

  useEffect(() => {
    initController(null);
  }, [initController]);

  return (
    <Chat
      mode="aichat"
      controller={controller}
      modelSelectorValue={modelValue}
      onModelChange={() => undefined}
      variant="full"
      showModelSelector={false}
      requireModel={false}
      showMemoryToggle={false}
      enableUpload={Boolean(agentConfig?.file_upload_enabled)}
      showFileLibraryPicker={false}
      suggestions={agentConfig?.suggested_questions ?? []}
      inputPlaceholder={inputPlaceholder}
      embeddedConversationMode="drawer"
      showAssistantModelMeta={false}
      surface="agent-webapp"
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
      homeTitle={homeTitle}
      homeDescription=""
    />
  );
}
