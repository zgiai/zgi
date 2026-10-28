'use client';

import { useMemo, useState } from 'react';
import Link from 'next/link';
import { AlertTriangle, Cpu, Plus, Store } from 'lucide-react';
import { toast } from 'sonner';
import {
  getAIChatSkillDisplayInfo,
  isSkillUserSelectable,
} from '@/components/chat/variants/aichat/skill-display';
import { Button } from '@/components/ui/button';
import {
  useAIChatSkillConfig,
  useAIChatSkills,
  useUpdateAIChatSkillConfig,
} from '@/hooks/aichat/use-aichat-skills';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import { isPermissionError } from './skill-availability';

export const SKILLS_PAGE_HREF = '/console/skills';
export const SKILL_MARKET_HREF = '/console/market/catalog?kind=skill';

export interface SkillCodeExecution {
  /** Whether the agent or digital human runs code in an execution environment. */
  enabled: boolean;
  /** Turns the environment on in place, where the host can. */
  onEnable?: () => void;
  /** Where the environment is turned on, when it cannot be done here. */
  settingsHref?: string;
  /** Said instead of a link, e.g. before a digital human exists. */
  settingsHint?: string;
}

/**
 * Closes the loop of binding a skill (Agent and DigitalHuman pickers, and the
 * template create form):
 *
 * - skills that exist but are not enabled for the organization, with a quick
 *   enable button, or the names to ask an administrator for when enabling is
 *   refused;
 * - skills a template expects that were deleted;
 * - where to add more (the skills page and the market);
 * - custom skills with scripts that need the code execution environment.
 */
export function SkillAvailabilityNotice({
  offeredIds,
  expectedIds = [],
  selectedIds = [],
  codeExecution,
  onEnabled,
  showDisabledList = true,
}: {
  /** Skills the picker can bind right now. */
  offeredIds: readonly string[];
  /** Skills a template asks for. */
  expectedIds?: readonly string[];
  /** Skills currently selected, checked for script execution. */
  selectedIds?: readonly string[];
  codeExecution?: SkillCodeExecution;
  /** Refresh the picker's candidates after a skill was enabled. */
  onEnabled?: (skillId: string) => void | Promise<unknown>;
  /** List every skill not yet enabled, not only the expected ones. */
  showDisabledList?: boolean;
}) {
  const t = useT('dashboard');
  const { locale } = useLocale();
  const skillsQuery = useAIChatSkills();
  const configQuery = useAIChatSkillConfig();
  const updateConfig = useUpdateAIChatSkillConfig();
  const [enabling, setEnabling] = useState<string | null>(null);
  const [refused, setRefused] = useState(false);
  const [expanded, setExpanded] = useState(false);

  const catalog = useMemo(
    () => (skillsQuery.data ?? []).filter(skill => isSkillUserSelectable(skill)),
    [skillsQuery.data]
  );
  const byId = useMemo(() => new Map(catalog.map(skill => [skill.skill_id, skill])), [catalog]);
  const enabledIds = configQuery.data?.enabled_skill_ids ?? [];
  const offered = new Set(offeredIds);
  const label = (id: string) => {
    const skill = byId.get(id);
    return skill ? getAIChatSkillDisplayInfo(skill, locale).label : id;
  };

  const disabled = catalog.filter(
    skill => !offered.has(skill.skill_id) && !enabledIds.includes(skill.skill_id)
  );
  const expectedMissing = expectedIds.filter(id => !offered.has(id));
  const expectedDisabled = expectedMissing.filter(id => byId.has(id));
  const expectedDeleted = skillsQuery.isSuccess ? expectedMissing.filter(id => !byId.has(id)) : [];
  const scriptSkills = selectedIds.filter(id => {
    const skill = byId.get(id);
    return skill?.source === 'custom' && skill.has_scripts;
  });

  const enable = async (id: string) => {
    if (enabling) return;
    setEnabling(id);
    try {
      const response = await updateConfig.mutateAsync({
        payload: { enabled_skill_ids: [...new Set([...enabledIds, id])] },
      });
      if (response.data?.status === 'confirmation_required') {
        toast.error(t('skillAvailability.needsReview'));
        return;
      }
      await Promise.all([configQuery.refetch(), onEnabled?.(id)]);
    } catch (error) {
      if (isPermissionError(error)) setRefused(true);
    } finally {
      setEnabling(null);
    }
  };

  const enableButton = (id: string) =>
    refused ? null : (
      <Button
        type="button"
        size="sm"
        variant="outline"
        className="h-7 shrink-0"
        disabled={enabling !== null}
        onClick={() => void enable(id)}
      >
        {enabling === id ? t('skillAvailability.enabling') : t('skillAvailability.enable')}
      </Button>
    );

  const showDisabled = showDisabledList && disabled.length > 0;
  const hasContent =
    expectedDeleted.length > 0 ||
    expectedDisabled.length > 0 ||
    showDisabled ||
    (scriptSkills.length > 0 && codeExecution && !codeExecution.enabled);

  return (
    <div className="space-y-2 text-xs leading-5">
      {expectedDeleted.length > 0 ? (
        <p role="alert" className="flex gap-2 rounded-md border border-warning/40 bg-warning/5 p-2">
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-warning" />
          <span>
            {t('skillAvailability.deleted', {
              skills: expectedDeleted.join('、'),
            })}
          </span>
        </p>
      ) : null}

      {expectedDisabled.length > 0 ? (
        <div className="space-y-1.5 rounded-md border border-primary/30 bg-primary/[0.03] p-2">
          <p>{t('skillAvailability.expectedDisabled')}</p>
          {expectedDisabled.map(id => (
            <div key={id} className="flex items-center justify-between gap-2">
              <span className="truncate font-medium">{label(id)}</span>
              {enableButton(id)}
            </div>
          ))}
        </div>
      ) : null}

      {showDisabled ? (
        <div className="rounded-md border bg-muted/20 p-2">
          <button
            type="button"
            className="w-full text-left font-medium"
            aria-expanded={expanded}
            onClick={() => setExpanded(value => !value)}
          >
            {t('skillAvailability.disabledCount', { count: disabled.length })}
          </button>
          {expanded ? (
            <div className="mt-2 max-h-48 space-y-1.5 overflow-y-auto">
              {disabled.map(skill => (
                <div key={skill.skill_id} className="flex items-center justify-between gap-2">
                  <span className="min-w-0">
                    <span className="block truncate font-medium">{label(skill.skill_id)}</span>
                    <span className="block truncate text-muted-foreground">
                      {getAIChatSkillDisplayInfo(skill, locale).description}
                    </span>
                  </span>
                  {enableButton(skill.skill_id)}
                </div>
              ))}
            </div>
          ) : null}
        </div>
      ) : null}

      {refused && (expectedDisabled.length > 0 || disabled.length > 0) ? (
        <p role="alert" className="rounded-md border border-warning/40 bg-warning/5 p-2">
          {t('skillAvailability.noPermission', {
            skills: [...new Set([...expectedDisabled, ...disabled.map(skill => skill.skill_id)])]
              .map(label)
              .join('、'),
          })}
        </p>
      ) : null}

      {scriptSkills.length > 0 && codeExecution && !codeExecution.enabled ? (
        <div className="flex flex-wrap items-center gap-2 rounded-md border border-warning/40 bg-warning/5 p-2">
          <Cpu className="size-3.5 shrink-0 text-warning" />
          <span className="min-w-0 flex-1">
            {t('skillAvailability.needsCodeExecution', {
              skills: scriptSkills.map(label).join('、'),
            })}
            {codeExecution.settingsHint ? ` ${codeExecution.settingsHint}` : null}
          </span>
          {codeExecution.onEnable ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="h-7"
              onClick={codeExecution.onEnable}
            >
              {t('skillAvailability.enableCodeExecution')}
            </Button>
          ) : codeExecution.settingsHref ? (
            <Button asChild size="sm" variant="outline" className="h-7">
              <Link href={codeExecution.settingsHref}>
                {t('skillAvailability.openCodeExecution')}
              </Link>
            </Button>
          ) : null}
        </div>
      ) : null}

      <div className={`flex flex-wrap gap-x-3 gap-y-1 ${hasContent ? '' : 'pt-0.5'}`}>
        <Link
          href={SKILLS_PAGE_HREF}
          className="inline-flex items-center gap-1 text-primary hover:underline"
        >
          <Plus className="size-3.5" />
          {t('skillAvailability.manage')}
        </Link>
        <Link
          href={SKILL_MARKET_HREF}
          className="inline-flex items-center gap-1 text-primary hover:underline"
        >
          <Store className="size-3.5" />
          {t('skillAvailability.market')}
        </Link>
      </div>
    </div>
  );
}
