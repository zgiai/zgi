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
    <div className="flex flex-col gap-2 border-b border-border/70 pb-3 lg:flex-row lg:items-center">
      <SearchInput
        value={searchQuery}
        onChange={event => onSearchQueryChange(event.target.value)}
        placeholder={t('organization.aichatSkills.filters.searchPlaceholder')}
        aria-label={t('organization.aichatSkills.filters.searchAria')}
        className="h-8 rounded-md lg:min-w-[220px] lg:flex-1"
      />

      <div className="grid gap-2 sm:grid-cols-2 lg:flex lg:shrink-0">
        <Select
          value={scenario}
          onValueChange={value => onScenarioChange(value as SkillScenarioFilter)}
        >
          <SelectTrigger
            className="h-8 rounded-md bg-background lg:w-32"
            aria-label={t('organization.aichatSkills.filters.scenarioAria')}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">
              {t('organization.aichatSkills.filters.allScenarios')}
            </SelectItem>
            {availableScenarios.map(item => (
              <SelectItem key={item} value={item}>
                {getSkillScenarioLabel(item, locale)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select
          value={capability}
          onValueChange={value => onCapabilityChange(value as SkillCapabilityFilter)}
        >
          <SelectTrigger
            className="h-8 rounded-md bg-background lg:w-32"
            aria-label={t('organization.aichatSkills.filters.capabilityAria')}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">
              {t('organization.aichatSkills.filters.allCapabilities')}
            </SelectItem>
            {availableCapabilities.map(item => (
              <SelectItem key={item} value={item}>
                {getSkillCapabilityLabel(item, locale)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select value={source} onValueChange={value => onSourceChange(value as SkillSourceFilter)}>
          <SelectTrigger
            className="h-8 rounded-md bg-background lg:w-28"
            aria-label={t('organization.aichatSkills.filters.sourceAria')}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t('organization.aichatSkills.filters.allSources')}</SelectItem>
            <SelectItem value="system">{t('organization.aichatSkills.source.system')}</SelectItem>
            <SelectItem value="custom">{t('organization.aichatSkills.source.custom')}</SelectItem>
          </SelectContent>
        </Select>

        <Select value={status} onValueChange={value => onStatusChange(value as SkillStatusFilter)}>
          <SelectTrigger
            className="h-8 rounded-md bg-background lg:w-28"
            aria-label={t('organization.aichatSkills.filters.statusAria')}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t('organization.aichatSkills.filters.allStatus')}</SelectItem>
            <SelectItem value="enabled">{t('organization.aichatSkills.status.enabled')}</SelectItem>
            <SelectItem value="disabled">
              {t('organization.aichatSkills.status.disabled')}
            </SelectItem>
            <SelectItem value="invalid">{t('organization.aichatSkills.status.invalid')}</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {hasActiveFilters ? (
        <Button variant="ghost" size="sm" className="h-8 shrink-0" onClick={onClearFilters}>
          {t('organization.aichatSkills.actions.clearFilters')}
        </Button>
      ) : null}
    </div>
  );
}
