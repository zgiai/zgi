'use client';

import { useState } from 'react';
import { Plus } from 'lucide-react';
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
import { useT } from '@/i18n';
import {
  CUSTOM_TEMPLATE_ID,
  DIGITAL_HUMAN_TEMPLATES,
  type DigitalHumanTemplate,
} from './digital-human-templates';
import {
  DigitalHumanAvatar,
  DigitalHumanCapabilityList,
  DigitalHumanDetailSection,
  DigitalHumanTagList,
} from './digital-human-template-preview';
import { useRoleTextResolver } from './use-role-texts';

/**
 * Template selection step.
 *
 * The card actions are revealed on hover *and* on keyboard focus, and stay
 * visible on small screens where hover does not exist. The detail drawer shows
 * the workspace skills a template declares, which are matched against the
 * workspace's real candidates when the digital human is created.
 */
export function DigitalHumanTemplateGallery({
  onSelectTemplate,
}: {
  onSelectTemplate: (templateId: string) => void;
}) {
  const t = useT('digitalHumans');
  const texts = useRoleTextResolver();
  const [detailTemplate, setDetailTemplate] = useState<DigitalHumanTemplate | null>(null);

  const templateTags = (template: DigitalHumanTemplate) =>
    template.tagKeys.map(key => texts(key));
  const templateSkills = (template: DigitalHumanTemplate) =>
    template.skills.map(skill => texts(skill.labelKey));

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

      {DIGITAL_HUMAN_TEMPLATES.length === 0 ? (
        <div className="flex min-h-40 items-center justify-center rounded-xl border border-dashed px-6 text-center text-sm text-muted-foreground">
          {t('createFlow.gallery.empty')}
        </div>
      ) : (
        <div className="grid grid-cols-[repeat(auto-fill,minmax(15rem,1fr))] gap-4">
          {DIGITAL_HUMAN_TEMPLATES.map(template => (
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
                    role: texts(template.titleKey),
                  })}
                >
                  <DigitalHumanAvatar
                    icon={{
                      type: 'text',
                      icon: texts(template.titleKey).trim().slice(0, 2).toUpperCase(),
                      iconBackground: template.iconBackground,
                    }}
                    fallbackName={texts(template.titleKey)}
                    size="md"
                  />
                  <div className="w-full min-w-0">
                    <h2 className="truncate text-sm font-semibold">{texts(template.titleKey)}</h2>
                    <p className="mt-1 line-clamp-2 min-h-10 text-xs leading-5 text-muted-foreground">
                      {texts(template.shortDescriptionKey)}
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
                    <DigitalHumanTagList tags={templateTags(template)} />
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
      )}

      <Sheet
        open={Boolean(detailTemplate)}
        onOpenChange={open => !open && setDetailTemplate(null)}
      >
        <SheetContent side="right" className="w-full overflow-y-auto sm:max-w-md">
          {detailTemplate ? (
            <>
              <SheetHeader>
                <div className="flex items-center gap-3">
                  <DigitalHumanAvatar
                    icon={{
                      type: 'text',
                      icon: texts(detailTemplate.titleKey).trim().slice(0, 2).toUpperCase(),
                      iconBackground: detailTemplate.iconBackground,
                    }}
                    fallbackName={texts(detailTemplate.titleKey)}
                    size="md"
                  />
                  <div className="min-w-0">
                    <SheetTitle className="truncate">{texts(detailTemplate.titleKey)}</SheetTitle>
                    <SheetDescription className="mt-1 line-clamp-2 text-xs">
                      {texts(detailTemplate.shortDescriptionKey)}
                    </SheetDescription>
                  </div>
                </div>
              </SheetHeader>

              <div className="mt-5 space-y-5 px-4 pb-4">
                <DigitalHumanDetailSection title={t('createFlow.detail.responsibility')}>
                  <p className="text-sm leading-6">{texts(detailTemplate.responsibilityKey)}</p>
                </DigitalHumanDetailSection>

                <DigitalHumanDetailSection title={t('createFlow.detail.goals')}>
                  <ul className="space-y-1.5">
                    {detailTemplate.goalKeys.map(key => (
                      <li key={key} className="flex gap-2 text-sm leading-6">
                        <span className="text-muted-foreground">·</span>
                        <span>{texts(key)}</span>
                      </li>
                    ))}
                  </ul>
                </DigitalHumanDetailSection>

                <DigitalHumanDetailSection title={t('createFlow.detail.rules')}>
                  <ul className="space-y-1.5">
                    {detailTemplate.ruleKeys.map(key => (
                      <li key={key} className="flex gap-2 text-sm leading-6">
                        <span className="text-muted-foreground">·</span>
                        <span>{texts(key)}</span>
                      </li>
                    ))}
                  </ul>
                </DigitalHumanDetailSection>

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
