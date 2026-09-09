'use client';

import { useMemo, useState, type CSSProperties } from 'react';
import { Copy, ExternalLink, Globe2, SendHorizontal, Settings2 } from 'lucide-react';
import { toast } from 'sonner';
import { ChatOpeningGuideView } from '@/components/chat/ui/chat-opening-guide-view';
import type { OpeningGuideBrand } from '@/components/chat/utils/opening-guide-brand';
import { IconPreview } from '@/components/common/icon-input/icon-preview';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import OpeningStatementDialog from '@/components/workflow/ui/features-panel/opening-statement-dialog';
import {
  useDigitalHumanAccess,
  useDigitalHumanAccessMutations,
} from '@/hooks/digital-human/use-digital-human-access';
import { useT } from '@/i18n';
import { ICON_BG } from '@/lib/config';
import type {
  DigitalHumanDefinition,
  DigitalHumanPresentationPolicy,
} from '@/services/types/digital-human';
import { getErrorMessage } from '@/utils/error-notifications';

interface DigitalHumanAccessPanelProps {
  id: string;
  definition: DigitalHumanDefinition;
  published: boolean;
  canPublish: boolean;
  canUpdatePresentation: boolean;
  presentation: DigitalHumanPresentationPolicy;
  onPresentationChange: (presentation: DigitalHumanPresentationPolicy) => void;
}

export function DigitalHumanAccessPanel({
  id,
  definition,
  published,
  canPublish,
  canUpdatePresentation,
  presentation,
  onPresentationChange,
}: DigitalHumanAccessPanelProps) {
  const t = useT('digitalHumans.access');
  const access = useDigitalHumanAccess(id, canPublish);
  const mutations = useDigitalHumanAccessMutations(id);
  const [openingDialogOpen, setOpeningDialogOpen] = useState(false);
  const landingURL = useMemo(
    () =>
      typeof window === 'undefined'
        ? `/digital-human/${definition.web_app_id}`
        : `${window.location.origin}/digital-human/${definition.web_app_id}`,
    [definition.web_app_id]
  );
  const settings = access.data?.data;
  const openingGuideBrand = useMemo(() => resolveOpeningGuideBrand(definition), [definition]);

  const copy = async (value: string) => {
    await navigator.clipboard.writeText(value);
    toast.success(t('linkCopied'));
  };

  const updateWebAppSurface = async (enabled: boolean) => {
    try {
      await mutations.updateAccess.mutateAsync({ webapp_enabled: enabled });
      toast.success(t('surfaceUpdated'));
    } catch (error) {
      toast.error(getErrorMessage(error) || t('surfaceUpdateFailed'));
    }
  };

  const updatePresentation = <K extends keyof DigitalHumanPresentationPolicy>(
    field: K,
    value: DigitalHumanPresentationPolicy[K]
  ) => onPresentationChange({ ...presentation, [field]: value });

  return (
    <div className="space-y-6 p-4 md:p-6">
      <section className="space-y-3">
        <div>
          <h3 className="text-sm font-semibold">{t('webAppTitle')}</h3>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">{t('webAppDescription')}</p>
        </div>
        {!published ? (
          <div className="rounded-lg border border-warning/30 bg-warning/5 px-3 py-2 text-xs text-warning-foreground">
            {t('publishFirst')}
          </div>
        ) : null}
        <div className="rounded-lg border px-4">
          <AccessSwitch
            title={t('webAppTitle')}
            description={t('surfacesDescription')}
            checked={settings?.webapp_enabled ?? false}
            disabled={
              !canPublish || !published || access.isLoading || mutations.updateAccess.isPending
            }
            onCheckedChange={enabled => void updateWebAppSurface(enabled)}
          />
        </div>
        <div className="flex flex-wrap items-center gap-2 rounded-lg border bg-muted/20 p-3">
          <Input
            value={landingURL}
            readOnly
            className="min-w-0 flex-1"
            aria-label={t('landingURL')}
          />
          <Button type="button" variant="outline" size="sm" onClick={() => void copy(landingURL)}>
            <Copy className="mr-2 size-4" />
            {t('copy')}
          </Button>
          <Button type="button" variant="outline" size="sm" asChild>
            <a href={landingURL} target="_blank" rel="noreferrer">
              <ExternalLink className="mr-2 size-4" />
              {t('open')}
            </a>
          </Button>
        </div>
      </section>

      <section className="space-y-4 border-t pt-6">
        <div>
          <h3 className="text-sm font-semibold">{t('presentationTitle')}</h3>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            {t('presentationDescription')}
          </p>
        </div>

        <div className="grid items-start gap-5 xl:grid-cols-[minmax(0,440px)_minmax(420px,1fr)]">
          <div className="space-y-5 rounded-xl border bg-background p-4">
            <div className="space-y-3">
              <div className="flex items-start justify-between gap-3">
                <div className="space-y-1">
                  <div className="text-sm font-semibold">{t('openingGuideTitle')}</div>
                  <p className="text-xs leading-5 text-muted-foreground">
                    {t('openingGuideDescription')}
                  </p>
                </div>
                <Button
                  type="button"
                  variant="outline"
                  size="xs"
                  className="shrink-0 gap-1.5"
                  disabled={!canUpdatePresentation}
                  onClick={() => setOpeningDialogOpen(true)}
                >
                  <Settings2 className="size-3.5" />
                  {t('configureOpeningGuide')}
                </Button>
              </div>
              <div className="max-h-80 overflow-y-auto rounded-lg border border-border/70 bg-muted/20 px-3 py-4">
                <ChatOpeningGuideView
                  title={presentation.home_title.trim() || definition.name}
                  message={presentation.opening_statement}
                  iconType={openingGuideBrand.iconType}
                  icon={openingGuideBrand.icon}
                  iconBackground={openingGuideBrand.iconBackground}
                  iconSrc={openingGuideBrand.iconSrc}
                  suggestions={presentation.suggested_questions}
                />
              </div>
            </div>

            <AccessField
              label={t('inputPlaceholder')}
              description={t('inputPlaceholderDescription')}
            >
              <Input
                value={presentation.input_placeholder}
                onChange={event => updatePresentation('input_placeholder', event.target.value)}
                disabled={!canUpdatePresentation}
                maxLength={120}
                showCharacterCount
              />
            </AccessField>

            <AccessField label={t('themeColor')} description={t('themeColorDescription')}>
              <div className="flex items-center gap-2">
                <input
                  type="color"
                  value={resolvedThemeColor(presentation.theme_color)}
                  onChange={event => updatePresentation('theme_color', event.target.value)}
                  disabled={!canUpdatePresentation}
                  className="size-9 cursor-pointer rounded-md border bg-background p-1 disabled:cursor-not-allowed"
                  aria-label={t('themeColor')}
                />
                <Input
                  value={presentation.theme_color}
                  onChange={event => updatePresentation('theme_color', event.target.value)}
                  disabled={!canUpdatePresentation}
                  placeholder="#6366f1"
                  maxLength={32}
                />
              </div>
            </AccessField>
          </div>

          <div className="space-y-2 xl:sticky xl:top-4">
            <div>
              <div className="text-sm font-semibold">{t('livePreview')}</div>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                {t('livePreviewDescription')}
              </p>
            </div>
            <DigitalHumanLandingPreview
              definition={definition}
              presentation={presentation}
              openingGuideBrand={openingGuideBrand}
            />
          </div>
        </div>
      </section>

      <OpeningStatementDialog
        open={openingDialogOpen}
        onOpenChange={setOpeningDialogOpen}
        value={{
          title: presentation.home_title,
          message: presentation.opening_statement,
          suggestedQuestions: presentation.suggested_questions,
        }}
        previewBrand={openingGuideBrand}
        onSave={value =>
          onPresentationChange({
            ...presentation,
            home_title: value.title,
            opening_statement: value.message,
            suggested_questions: value.suggestedQuestions,
          })
        }
      />
    </div>
  );
}

function DigitalHumanLandingPreview({
  definition,
  presentation,
  openingGuideBrand,
}: {
  definition: DigitalHumanDefinition;
  presentation: DigitalHumanPresentationPolicy;
  openingGuideBrand: OpeningGuideBrand;
}) {
  const t = useT('digitalHumans.access');
  const themeColor = resolvedThemeColor(presentation.theme_color);
  const style = { '--digital-human-preview-theme': themeColor } as CSSProperties;

  return (
    <div
      style={style}
      className="flex min-h-[520px] flex-col overflow-hidden rounded-xl border bg-muted/20 shadow-sm"
    >
      <div className="flex h-14 shrink-0 items-center gap-2.5 border-b bg-background px-4">
        <IconPreview
          iconType={openingGuideBrand.iconType === 'image' ? 'image' : 'text'}
          src={openingGuideBrand.iconSrc}
          icon={openingGuideBrand.icon}
          iconBackground={openingGuideBrand.iconBackground}
          alt={definition.name}
          size="xs"
          editable={false}
        />
        <div className="min-w-0">
          <div className="truncate text-sm font-semibold">{definition.name}</div>
          {definition.description ? (
            <div className="truncate text-xs text-muted-foreground">{definition.description}</div>
          ) : null}
        </div>
      </div>
      <div className="flex min-h-0 flex-1 flex-col bg-background p-4">
        <div className="flex flex-1 items-center justify-center overflow-y-auto px-2 py-6">
          <ChatOpeningGuideView
            title={presentation.home_title.trim() || definition.name}
            message={presentation.opening_statement}
            iconType={openingGuideBrand.iconType}
            icon={openingGuideBrand.icon}
            iconBackground={openingGuideBrand.iconBackground}
            iconSrc={openingGuideBrand.iconSrc}
            suggestions={presentation.suggested_questions}
          />
        </div>
        <div className="mx-auto flex w-full max-w-2xl items-center gap-2 rounded-xl border bg-background p-2 shadow-sm">
          <div className="min-w-0 flex-1 truncate px-2 text-sm text-muted-foreground">
            {presentation.input_placeholder.trim() || t('previewInputPlaceholder')}
          </div>
          <span
            className="flex size-8 shrink-0 items-center justify-center rounded-lg text-white"
            style={{ backgroundColor: 'var(--digital-human-preview-theme)' }}
          >
            <SendHorizontal className="size-4" />
          </span>
        </div>
      </div>
      <div className="flex items-center gap-2 border-t bg-muted/30 px-3 py-2 text-[11px] text-muted-foreground">
        <Globe2 className="size-3.5" />
        {t('previewFooter')}
      </div>
    </div>
  );
}

function AccessSwitch({
  title,
  description,
  checked,
  disabled,
  onCheckedChange,
}: {
  title: string;
  description: string;
  checked: boolean;
  disabled: boolean;
  onCheckedChange: (checked: boolean) => void;
}) {
  return (
    <div className="flex items-start justify-between gap-5 py-4">
      <div>
        <div className="text-sm font-medium">{title}</div>
        <p className="mt-1 text-xs leading-5 text-muted-foreground">{description}</p>
      </div>
      <Switch checked={checked} onCheckedChange={onCheckedChange} disabled={disabled} />
    </div>
  );
}

function AccessField({
  label,
  description,
  children,
}: {
  label: string;
  description: string;
  children: React.ReactNode;
}) {
  return (
    <label className="grid gap-2">
      <span>
        <span className="block text-sm font-medium">{label}</span>
        <span className="mt-1 block text-xs leading-5 text-muted-foreground">{description}</span>
      </span>
      {children}
    </label>
  );
}

function resolveOpeningGuideBrand(definition: DigitalHumanDefinition): OpeningGuideBrand {
  let icon = definition.name.slice(0, 2).toUpperCase();
  let iconBackground = ICON_BG;
  if (definition.icon_type === 'text' && definition.icon) {
    try {
      const parsed = JSON.parse(definition.icon) as {
        icon?: string;
        icon_background?: string;
      };
      icon = parsed.icon || icon;
      iconBackground = parsed.icon_background || iconBackground;
    } catch {
      icon = definition.icon.slice(0, 2) || icon;
    }
  }
  return {
    title: definition.name,
    iconType: definition.icon_type,
    icon,
    iconBackground,
    iconSrc:
      definition.icon_type === 'image'
        ? definition.icon_url || definition.avatar || definition.icon
        : undefined,
  };
}

function resolvedThemeColor(value: string): string {
  return /^#[0-9a-f]{6}$/i.test(value.trim()) ? value.trim() : '#6366f1';
}
