'use client';

import {
  getSkillCapabilityLabel,
  getSkillScenarioLabel,
  type SkillCapabilityCategory,
  type SkillScenario,
} from '@/components/chat/variants/aichat/skill-taxonomy';
import { Button } from '@/components/ui/button';
import { SearchInput } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useT } from '@/i18n/translations';
import type { AIChatSkillSource } from '@/services/types/aichat';

export type SkillScenarioFilter = 'all' | SkillScenario;
export type SkillCapabilityFilter = 'all' | SkillCapabilityCategory;
export type SkillSourceFilter = 'all' | AIChatSkillSource;
export type SkillStatusFilter = 'all' | 'enabled' | 'disabled' | 'invalid';

interface AIChatSkillCatalogFiltersProps {
  locale: string;
  availableScenarios: SkillScenario[];
  availableCapabilities: SkillCapabilityCategory[];
  scenario: SkillScenarioFilter;
  capability: SkillCapabilityFilter;
  source: SkillSourceFilter;
  status: SkillStatusFilter;
  searchQuery: string;
  hasActiveFilters: boolean;
  onScenarioChange: (value: SkillScenarioFilter) => void;
  onCapabilityChange: (value: SkillCapabilityFilter) => void;
  onSourceChange: (value: SkillSourceFilter) => void;
  onStatusChange: (value: SkillStatusFilter) => void;
  onSearchQueryChange: (value: string) => void;
  onClearFilters: () => void;
}

export function AIChatSkillCatalogFilters({
  locale,
  availableScenarios,
  availableCapabilities,
  scenario,
  capability,
  source,
  status,
  searchQuery,
  hasActiveFilters,
  onScenarioChange,
  onCapabilityChange,
  onSourceChange,
  onStatusChange,
  onSearchQueryChange,
  onClearFilters,
}: AIChatSkillCatalogFiltersProps) {
  const t = useT('dashboard');

  return (
    <div className="space-y-3 rounded-lg border bg-card p-3.5 shadow-sm">
      <div className="space-y-2">
        <p className="text-xs font-medium text-muted-foreground">
          {t('organization.aichatSkills.filters.scenarioLabel')}
        </p>
        <div className="flex gap-1.5 overflow-x-auto pb-1">
          <Button
            type="button"
            variant={scenario === 'all' ? 'default' : 'ghost'}
            size="sm"
            className="shrink-0 rounded-md"
            aria-pressed={scenario === 'all'}
            onClick={() => onScenarioChange('all')}
          >
            {t('organization.aichatSkills.filters.allScenarios')}
          </Button>
          {availableScenarios.map(item => (
            <Button
              key={item}
              type="button"
              variant={scenario === item ? 'default' : 'ghost'}
              size="sm"
              className="shrink-0 rounded-md"
              aria-pressed={scenario === item}
              onClick={() => onScenarioChange(item)}
            >
              {getSkillScenarioLabel(item, locale)}
            </Button>
          ))}
        </div>
      </div>

      <div className="space-y-2">
        <p className="text-xs font-medium text-muted-foreground">
          {t('organization.aichatSkills.filters.capabilityLabel')}
        </p>
        <div className="flex flex-wrap gap-1.5">
          <Button
            type="button"
            variant={capability === 'all' ? 'secondary' : 'outline'}
            size="sm"
            className="h-7 rounded-full px-3 text-xs"
            aria-pressed={capability === 'all'}
            onClick={() => onCapabilityChange('all')}
          >
            {t('organization.aichatSkills.filters.allCapabilities')}
          </Button>
          {availableCapabilities.map(item => (
            <Button
              key={item}
              type="button"
              variant={capability === item ? 'secondary' : 'outline'}
              size="sm"
              className="h-7 rounded-full px-3 text-xs"
              aria-pressed={capability === item}
              onClick={() => onCapabilityChange(item)}
            >
              {getSkillCapabilityLabel(item, locale)}
            </Button>
          ))}
        </div>
      </div>

      <div className="flex flex-col gap-2 lg:flex-row lg:items-center">
        <SearchInput
          value={searchQuery}
          onChange={event => onSearchQueryChange(event.target.value)}
          placeholder={t('organization.aichatSkills.filters.searchPlaceholder')}
          aria-label={t('organization.aichatSkills.filters.searchAria')}
          className="rounded-md lg:w-[360px]"
        />
        <div className="grid gap-2 sm:grid-cols-2 lg:flex lg:shrink-0">
          <Select
            value={source}
            onValueChange={value => onSourceChange(value as SkillSourceFilter)}
          >
            <SelectTrigger
              className="rounded-md bg-background lg:w-40"
              aria-label={t('organization.aichatSkills.filters.sourceAria')}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">
                {t('organization.aichatSkills.filters.allSources')}
              </SelectItem>
              <SelectItem value="system">{t('organization.aichatSkills.source.system')}</SelectItem>
              <SelectItem value="custom">{t('organization.aichatSkills.source.custom')}</SelectItem>
            </SelectContent>
          </Select>

          <Select
            value={status}
            onValueChange={value => onStatusChange(value as SkillStatusFilter)}
          >
            <SelectTrigger
              className="rounded-md bg-background lg:w-40"
              aria-label={t('organization.aichatSkills.filters.statusAria')}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">
                {t('organization.aichatSkills.filters.allStatus')}
              </SelectItem>
              <SelectItem value="enabled">
                {t('organization.aichatSkills.status.enabled')}
              </SelectItem>
              <SelectItem value="disabled">
                {t('organization.aichatSkills.status.disabled')}
              </SelectItem>
              <SelectItem value="invalid">
                {t('organization.aichatSkills.status.invalid')}
              </SelectItem>
            </SelectContent>
          </Select>
        </div>
        {hasActiveFilters ? (
          <Button variant="ghost" size="sm" onClick={onClearFilters}>
            {t('organization.aichatSkills.actions.clearFilters')}
          </Button>
        ) : null}
      </div>
    </div>
  );
}
