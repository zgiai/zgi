'use client';

import { useMemo, useState } from 'react';
import { Loader2, Plus } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { useDigitalHumanSkillCandidates } from '@/hooks/digital-human/use-digital-humans';
import { useT } from '@/i18n';
import { ICON_BG } from '@/lib/config';
import { CUSTOM_TEMPLATE_ID, type DigitalHumanTemplate } from './digital-human-templates';
import {
  DigitalHumanAvatar,
  DigitalHumanCapabilityList,
  DigitalHumanDetailSection,
  DigitalHumanTagList,
} from './digital-human-template-preview';

/**
 * Template selection step.
 *
 * Built-in templates come first, then the workspace's saved ones (ADR 0022).
 * The card actions are revealed on hover *and* on keyboard focus, and stay
 * visible on small screens where hover does not exist. The detail drawer shows
 * the workspace skills a template declares, which are matched against the
 * workspace's real candidates when the digital human is created.
 */
export function DigitalHumanTemplateGallery({
  templates,
  loading,
  error,
  onRetry,
  onSelectTemplate,
}: {
  templates: readonly DigitalHumanTemplate[];
  loading: boolean;
  error: boolean;
  onRetry: () => void;
  onSelectTemplate: (templateId: string) => void;
}) {
  const t = useT('digitalHumans');
  const [detailTemplate, setDetailTemplate] = useState<DigitalHumanTemplate | null>(null);
  const skillCandidates = useDigitalHumanSkillCandidates().data?.data?.items;
  const skillNames = useMemo(
    () => new Map((skillCandidates ?? []).map(item => [item.resource_id, item.display_name])),
    [skillCandidates]
  );
  const sections = [
    {
      key: 'builtin',
      title: t('createFlow.gallery.builtinTitle'),
      items: templates.filter(template => template.source === 'builtin'),
    },
    {
      key: 'workspace',
      title: t('createFlow.gallery.workspaceTitle'),
      items: templates.filter(template => template.source === 'workspace'),
    },
  ].filter(section => section.items.length > 0);

  const templateSkills = (template: DigitalHumanTemplate) =>
    template.skills.map(skill => {
      const name = skillNames.get(skill.id) || skill.id;
      return skill.required ? t('createFlow.detail.requiredSkill', { name }) : name;
    });

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
        <div className="min-w-0">
          <h1 className="text-xl font-semibold sm:text-2xl">{t('createFlow.title')}</h1>
          <p className="mt-1 text-sm text-muted-foreground">{t('createFlow.subtitle')}</p>
        </div>
        <div className="flex w-full flex-col gap-2 lg:w-auto lg:items-end">
          <Button
            className="w-full shrink-0 sm:w-auto"
            onClick={() => onSelectTemplate(CUSTOM_TEMPLATE_ID)}
          >
            <Plus className="size-4" />
            {t('createFlow.gallery.customEntry')}
          </Button>
          <p className="text-xs text-muted-foreground lg:max-w-56 lg:text-right">
            {t('createFlow.gallery.customHint')}
          </p>
        </div>
      </div>

      {loading ? (
        <div className="flex min-h-40 items-center justify-center">
          <Loader2 className="size-6 animate-spin text-muted-foreground" />
        </div>
      ) : error ? (
        <div className="flex min-h-40 flex-col items-center justify-center gap-3 rounded-xl border border-dashed px-6 text-center text-sm text-muted-foreground">
          {t('createFlow.gallery.loadFailed')}
          <Button variant="outline" size="sm" onClick={onRetry}>
            {t('createFlow.gallery.retry')}
          </Button>
        </div>
      ) : sections.length === 0 ? (
        <div className="flex min-h-40 items-center justify-center rounded-xl border border-dashed px-6 text-center text-sm text-muted-foreground">
          {t('createFlow.gallery.empty')}
        </div>
      ) : (
        sections.map(section => (
          <section key={section.key} className="space-y-3">
            {sections.length > 1 ? (
              <h2 className="text-sm font-medium text-muted-foreground">{section.title}</h2>
            ) : null}
            <div className="grid grid-cols-[repeat(auto-fill,minmax(15rem,1fr))] gap-4">
              {section.items.map(template => (
                <Card
                  key={template.id}
                  className="group flex flex-col border border-border/80 shadow-sm transition-colors motion-reduce:transition-none focus-within:border-border hover:border-border hover:bg-muted/20"
                >
                  <CardContent className="flex min-w-0 flex-1 flex-col p-4">
                    <button
                      type="button"
                      onClick={() => onSelectTemplate(template.id)}
                      className="flex min-w-0 flex-1 flex-col items-start gap-3 rounded-lg text-left outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                      aria-label={t('createFlow.gallery.createWithRole', {
                        role: template.title,
                      })}
                    >
                      <DigitalHumanAvatar
                        icon={{
                          type: 'text',
                          icon: template.title.trim().slice(0, 2).toUpperCase(),
                          iconBackground: template.icon_background || ICON_BG,
                        }}
                        fallbackName={template.title}
                        size="md"
                      />
                      <div className="w-full min-w-0">
                        <h3 className="truncate text-sm font-semibold">{template.title}</h3>
                        <p className="mt-1 line-clamp-2 min-h-10 text-xs leading-5 text-muted-foreground">
                          {template.description || template.persona}
                        </p>
                      </div>
                    </button>

                    {/* One fixed-height slot: tags by default, actions once the card
                    is hovered or holds focus. Swapping in place keeps every card
                    the same height and stops the buttons from reserving space.
                    Small screens have no hover, so there the actions simply take
                    the slot (the tags stay available in the detail drawer). */}
                    <div className="relative mt-3 h-8">
                      <div className="hidden h-full items-center overflow-hidden sm:flex sm:transition-opacity motion-reduce:transition-none sm:group-hover:opacity-0 sm:group-focus-within:opacity-0">
                        <DigitalHumanTagList tags={template.tags} limit={2} />
                      </div>
                      <div className="absolute inset-0 flex items-center gap-2 opacity-100 transition-opacity motion-reduce:transition-none sm:opacity-0 sm:group-hover:opacity-100 sm:group-focus-within:opacity-100">
                        <Button
                          variant="ghost"
                          size="sm"
                          className="flex-1"
                          onClick={() => setDetailTemplate(template)}
                        >
                          {t('createFlow.gallery.details')}
                        </Button>
                        <Button
                          size="sm"
                          className="flex-1"
                          onClick={() => onSelectTemplate(template.id)}
                        >
                          {t('createFlow.gallery.create')}
                        </Button>
                      </div>
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>
          </section>
        ))
      )}

      <Sheet open={Boolean(detailTemplate)} onOpenChange={open => !open && setDetailTemplate(null)}>
        <SheetContent side="right" className="w-full overflow-y-auto sm:max-w-md">
          {detailTemplate ? (
            <>
              <SheetHeader>
                <div className="flex items-center gap-3">
                  <DigitalHumanAvatar
                    icon={{
                      type: 'text',
                      icon: detailTemplate.title.trim().slice(0, 2).toUpperCase(),
                      iconBackground: detailTemplate.icon_background || ICON_BG,
                    }}
                    fallbackName={detailTemplate.title}
                    size="md"
                  />
                  <div className="min-w-0">
                    <SheetTitle className="truncate">{detailTemplate.title}</SheetTitle>
                    <SheetDescription className="mt-1 line-clamp-2 text-xs">
                      {detailTemplate.description}
                    </SheetDescription>
                  </div>
                </div>
              </SheetHeader>

              <div className="mt-5 space-y-5 px-4 pb-4">
                {detailTemplate.responsibility ? (
                  <DigitalHumanDetailSection title={t('createFlow.detail.responsibility')}>
                    <p className="text-sm leading-6">{detailTemplate.responsibility}</p>
                  </DigitalHumanDetailSection>
                ) : null}

                {detailTemplate.goals.length > 0 ? (
                  <DigitalHumanDetailSection title={t('createFlow.detail.goals')}>
                    <ul className="space-y-1.5">
                      {detailTemplate.goals.map(goal => (
                        <li key={goal} className="flex gap-2 text-sm leading-6">
                          <span className="text-muted-foreground">·</span>
                          <span>{goal}</span>
                        </li>
                      ))}
                    </ul>
                  </DigitalHumanDetailSection>
                ) : null}

                {detailTemplate.rules.length > 0 ? (
                  <DigitalHumanDetailSection title={t('createFlow.detail.rules')}>
                    <ul className="space-y-1.5">
                      {detailTemplate.rules.map(rule => (
                        <li key={rule} className="flex gap-2 text-sm leading-6">
                          <span className="text-muted-foreground">·</span>
                          <span>{rule}</span>
                        </li>
                      ))}
                    </ul>
                  </DigitalHumanDetailSection>
                ) : null}

                {detailTemplate.memory_instructions ? (
                  <DigitalHumanDetailSection title={t('createFlow.detail.memory')}>
                    <p className="text-sm leading-6">{detailTemplate.memory_instructions}</p>
                  </DigitalHumanDetailSection>
                ) : null}

                {detailTemplate.requires_connector ? (
                  <DigitalHumanDetailSection title={t('createFlow.detail.connector')}>
                    <p className="text-xs leading-5 text-muted-foreground">
                      {t('createFlow.form.connectorRequired')}
                    </p>
                  </DigitalHumanDetailSection>
                ) : null}

                {detailTemplate.skills.length > 0 ? (
                  <DigitalHumanDetailSection title={t('createFlow.detail.skills')}>
                    <DigitalHumanCapabilityList items={templateSkills(detailTemplate)} />
                    <p className="text-xs leading-5 text-muted-foreground">
                      {t('createFlow.form.skillsPending')}
                    </p>
                  </DigitalHumanDetailSection>
                ) : null}

                <DigitalHumanDetailSection title={t('createFlow.detail.defaults')}>
                  <p className="text-xs leading-5 text-muted-foreground">
                    {t('createFlow.detail.defaultsValue')}
                  </p>
                </DigitalHumanDetailSection>
              </div>

              <SheetFooter className="border-t">
                <Button className="w-full" onClick={() => onSelectTemplate(detailTemplate.id)}>
                  {t('createFlow.gallery.use')}
                </Button>
              </SheetFooter>
            </>
          ) : null}
        </SheetContent>
      </Sheet>
    </div>
  );
}
