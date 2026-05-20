import React from 'react';
import { ChevronRight, Info, Sparkles } from 'lucide-react';
import { useT } from '@/i18n';
import type { FormInputs } from '@/components/workflow/common/workflow-input-form';

export interface DebugSampleInput {
  title: string;
  description: string;
  values: FormInputs;
  previewItems: Array<{ label: string; value: string }>;
}

interface DebugInputGuideProps {
  sample?: DebugSampleInput | null;
  setupHints?: string[];
  onApplySample: () => void;
}

const DebugInputGuide: React.FC<DebugInputGuideProps> = ({
  sample,
  setupHints = [],
  onApplySample,
}) => {
  const t = useT();

  if (!sample && setupHints.length === 0) return null;

  return (
    <div className="mb-3 space-y-2 rounded-lg border border-border bg-muted/20 p-3">
      <div className="flex items-start gap-2">
        <div className="mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
          <Sparkles className="size-3.5" />
        </div>
        <div className="min-w-0">
          <div className="text-sm font-semibold text-foreground">
            {t('agents.workflow.debugGuide.title')}
          </div>
          <p className="mt-0.5 text-xs leading-5 text-muted-foreground">
            {t('agents.workflow.debugGuide.description')}
          </p>
        </div>
      </div>

      {sample ? (
        <button
          type="button"
          className="group w-full rounded-md border border-border bg-background p-2.5 text-left transition-colors hover:border-primary/40 hover:bg-primary/5"
          onClick={onApplySample}
        >
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="text-sm font-medium text-foreground">{sample.title}</div>
              <p className="mt-0.5 text-xs leading-5 text-muted-foreground">
                {sample.description}
              </p>
            </div>
            <ChevronRight className="mt-0.5 size-4 shrink-0 text-muted-foreground transition-colors group-hover:text-primary" />
          </div>
          {sample.previewItems.length > 0 ? (
            <div className="mt-2 space-y-1">
              {sample.previewItems.map(item => (
                <div key={item.label} className="min-w-0 text-xs leading-5 text-muted-foreground">
                  <span className="font-medium text-foreground/80">{item.label}: </span>
                  <span>{item.value}</span>
                </div>
              ))}
            </div>
          ) : null}
        </button>
      ) : null}

      {setupHints.length > 0 ? (
        <div className="space-y-1.5 pt-1">
          {setupHints.map(hint => (
            <div key={hint} className="flex items-start gap-2 text-xs leading-5 text-muted-foreground">
              <Info className="mt-0.5 size-3.5 shrink-0 text-muted-foreground" />
              <span>{hint}</span>
            </div>
          ))}
        </div>
      ) : null}
    </div>
  );
};

export default DebugInputGuide;
