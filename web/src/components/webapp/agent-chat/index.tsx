'use client';

import { useEffect, useMemo } from 'react';
import { LogIn } from 'lucide-react';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import Chat, { createAgentWebAppTransport, useAIChatController } from '@/components/chat';
import { IconPreview } from '@/components/common/icon-input/icon-preview';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n';
import { ICON_BG } from '@/lib/config';
import type { WebAppWorkflowConfig } from '@/services/types/webapp';
import { useAuthStore } from '@/store/auth-store';
import { WEBAPP_USER_MIGRATED_EVENT } from '@/hooks/webapp/use-maybe-migrate-user';

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
  const pathname = usePathname();
  const router = useRouter();
  const searchParams = useSearchParams();
  const isAuthenticated = useAuthStore.use.isAuthenticated();
  const isAuthLoading = useAuthStore.use.isLoading();
  const isAuthInitialized = useAuthStore.use.isInitialized();
  const agentConfig = config.agent_config;
  const memoryEnabled = Boolean(agentConfig?.agent_memory_enabled);
  const supportsVision = Boolean(agentConfig?.supports_vision);
  const requiresLoginForMemory =
    memoryEnabled && isAuthInitialized && !isAuthLoading && !isAuthenticated;
  const canUseFiles = Boolean(agentConfig?.file_upload_enabled && isAuthenticated);
  const homeTitle = agentConfig?.home_title || t('agentChat.defaultHomeTitle');
  const inputPlaceholder = agentConfig?.input_placeholder || t('chat.enterCommand');
  const iconPreview = useMemo(
    () => resolveWebAppIcon(config, t('agentChat.fallbackTitle')),
    [config, t]
  );
  const transport = useMemo(() => createAgentWebAppTransport(webAppId), [webAppId]);
  const uploadScope = useMemo(() => ({ type: 'webapp' as const, webAppId }), [webAppId]);
  const controller = useAIChatController({ transport, requireModel: false });
  const initController = controller.init;
  const modelValue = useMemo(() => ({ provider: '', model: '', params: {} }), []);

  useEffect(() => {
    if (requiresLoginForMemory) return;
    initController(null);
  }, [initController, requiresLoginForMemory]);

  useEffect(() => {
    if (!isAuthenticated) return;
    const refreshMigratedConversations = () => {
      void controller.refreshList();
    };
    window.addEventListener(WEBAPP_USER_MIGRATED_EVENT, refreshMigratedConversations);
    return () => {
      window.removeEventListener(WEBAPP_USER_MIGRATED_EVENT, refreshMigratedConversations);
    };
  }, [controller, isAuthenticated]);

  const handleLogin = () => {
    const search = searchParams.toString();
    const currentUrl = search ? `${pathname}?${search}` : pathname;
    router.push(`/login?redirect=${encodeURIComponent(currentUrl)}`);
  };

  if (requiresLoginForMemory) {
    return (
      <div className="flex h-full w-full items-center justify-center px-4">
        <div className="flex max-w-md flex-col items-center text-center">
          <div className="mb-4 flex size-16 items-center justify-center rounded-2xl border bg-background shadow-sm">
            <IconPreview
              iconType={iconPreview.iconType}
              src={iconPreview.src}
              icon={iconPreview.icon}
              iconBackground={iconPreview.iconBackground}
              editable={false}
              size="lg"
            />
          </div>
          <h1 className="text-lg font-semibold">{t('agentChat.memoryLoginRequiredTitle')}</h1>
          <p className="mt-2 text-sm leading-6 text-muted-foreground">
            {t('agentChat.memoryLoginRequiredDescription')}
          </p>
          <Button className="mt-5 gap-2" onClick={handleLogin}>
            <LogIn className="size-4" />
            {t('header.login')}
          </Button>
        </div>
      </div>
    );
  }

  return (
    <Chat
      mode="aichat"
      controller={controller}
      modelSelectorValue={modelValue}
      onModelChange={() => undefined}
      variant="full"
      showModelSelector={false}
      requireModel={false}
      supportsVisionOverride={supportsVision}
      showMemoryToggle={false}
      enableUpload={canUseFiles}
      uploadScope={uploadScope}
      showFileLibraryPicker={canUseFiles}
      allowWorkspaceSwitch
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
