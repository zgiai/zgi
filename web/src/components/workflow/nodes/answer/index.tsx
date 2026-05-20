import React from 'react';
import type { AnswerNodeData } from './config';
import ValueBadge from '@/components/workflow/ui/value-badge';
import { useT } from '@/i18n';

export interface AnswerContentProps {
  nodeId: string;
  data: AnswerNodeData;
}

const AnswerContent: React.FC<AnswerContentProps> = ({ nodeId, data }) => {
  const preview = data.answer || '';
  const t = useT('nodes');

  const renderWithBadges = (input: string) => {
    if (!input) return <span className="text-destructive">{t('answer.noAnswer')}</span>;
    const pattern = /\{\{#[^#]+?#\}\}/g;
    const parts: Array<string | React.ReactNode> = [];
    let lastIndex = 0;
    let match: RegExpExecArray | null;
    while ((match = pattern.exec(input)) !== null) {
      const [full] = match;
      const start = match.index;
      const end = start + full.length;
      if (start > lastIndex) {
        parts.push(input.slice(lastIndex, start));
      }
      parts.push(
        <ValueBadge
          key={`vb-${start}`}
          template={full}
          className="mx-0.5 align-baseline"
          currentNodeId={nodeId}
        />
      );
      lastIndex = end;
    }
    if (lastIndex < input.length) {
      parts.push(input.slice(lastIndex));
    }
    return parts.length > 0 ? (
      parts
    ) : (
      <span className="text-destructive">{t('answer.noAnswer')}</span>
    );
  };

  return (
    <div className="mt-1">
      <div
        className="nowheel mt-1 min-h-8 max-h-[320px] overflow-y-auto rounded-md bg-muted/70 p-1.5 text-xs leading-relaxed text-secondary-foreground break-words whitespace-pre-wrap scrollbar-thin"
        onWheelCapture={event => event.stopPropagation()}
      >
        {renderWithBadges(preview)}
      </div>
    </div>
  );
};

export default AnswerContent;
