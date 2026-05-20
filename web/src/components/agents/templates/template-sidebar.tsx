'use client';

import type { LucideIcon } from 'lucide-react';
import {
  Building2,
  Cable,
  BookOpenCheck,
  Database,
  FileText,
  Gauge,
  GitBranch,
  ListChecks,
  ShieldCheck,
  Sparkles,
  ThumbsUp,
} from 'lucide-react';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { TEMPLATE_CATEGORY_SECTIONS } from './template-manifest';
import { getCategoryLabel, type TemplateTranslator } from './template-labels';
import type { AgentTemplateCategoryId } from './types';

const CATEGORY_ICON: Record<AgentTemplateCategoryId, LucideIcon> = {
  recommended: ThumbsUp,
  starter: Gauge,
  standard: ListChecks,
  advanced: GitBranch,
  enterprise: Building2,
  'document-intake': FileText,
  'knowledge-service': BookOpenCheck,
  'data-systems': Database,
  'integration-automation': Cable,
  governance: ShieldCheck,
};

interface TemplateSidebarProps {
  activeCategory: AgentTemplateCategoryId;
  disabled: boolean;
  onCategoryChange: (category: AgentTemplateCategoryId) => void;
}

export function TemplateSidebar({
  activeCategory,
  disabled,
  onCategoryChange,
}: TemplateSidebarProps) {
  const t = useT();
  const templateT = t as TemplateTranslator;

  const renderCategoryButton = (categoryId: AgentTemplateCategoryId) => {
    const Icon = CATEGORY_ICON[categoryId] ?? Sparkles;
    const isActive = activeCategory === categoryId;

    return (
      <button
        key={categoryId}
        type="button"
        disabled={disabled}
        onClick={() => onCategoryChange(categoryId)}
        className={cn(
          'flex shrink-0 items-center gap-2 rounded-lg px-3 py-2 text-left text-[13px] font-medium transition-colors md:w-full md:gap-2.5',
          isActive
            ? 'border border-border bg-background text-foreground shadow-sm'
            : 'text-muted-foreground hover:bg-background/70 hover:text-foreground',
          disabled && 'cursor-not-allowed opacity-60'
        )}
      >
        <Icon className={cn('size-4 shrink-0', isActive && 'text-primary')} />
        <span className="min-w-0 whitespace-nowrap md:truncate">
          {getCategoryLabel(templateT, categoryId)}
        </span>
      </button>
    );
  };

  return (
    <aside className="shrink-0 border-b bg-muted/20 md:flex md:min-h-0 md:w-56 md:flex-col md:border-b-0 md:border-r">
      <div className="flex gap-2 overflow-x-auto p-3 md:min-h-0 md:flex-1 md:flex-col md:gap-0 md:overflow-y-auto">
        {TEMPLATE_CATEGORY_SECTIONS.map(section => (
          <div key={section.id} className="flex gap-2 md:block md:space-y-1">
            {section.labelKey ? (
              <div className="hidden px-3 pb-2 pt-4 text-[11px] font-semibold uppercase tracking-normal text-muted-foreground md:block">
                {templateT(section.labelKey)}
              </div>
            ) : null}
            {section.categories.map(category => renderCategoryButton(category.id))}
          </div>
        ))}
      </div>
    </aside>
  );
}
