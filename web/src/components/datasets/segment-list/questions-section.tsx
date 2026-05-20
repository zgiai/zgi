'use client';

import React from 'react';
import { useT } from '@/i18n';
import { ChevronDown, ChevronUp, Edit, Trash2, Loader2 } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';
import type { QuestionsSectionProps } from './types';

/**
 * Questions section component - displays questions list with actions
 */
export const QuestionsSection = React.memo(function QuestionsSection({
  segment,
  isExpanded,
  questionsLoading,
  questionsData,
  generatingQuestionsSegmentId,
  onToggleExpand,
  onPrefetchQuestions,
  onAddQuestion,
  onGenerateQuestions,
  onBatchImportQuestions,
  onEditQuestion,
  onDeleteQuestion,
}: QuestionsSectionProps) {
  const t = useT('datasets');

  return (
    <div>
      <div className="flex items-center gap-2">
        <Badge
          variant="warning"
          className="cursor-pointer"
          onClick={() => onToggleExpand(segment.id)}
          onMouseEnter={() => onPrefetchQuestions(segment.id)}
        >
          {t('segments.questions')}
          <span>
            {isExpanded ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}
          </span>
        </Badge>
        <Badge variant="warning" className="cursor-pointer" onClick={() => onAddQuestion(segment)}>
          {t('segments.add')}
        </Badge>
        <Badge
          variant="warning"
          className={cn(
            'cursor-pointer',
            generatingQuestionsSegmentId === segment.id && 'pointer-events-none opacity-50'
          )}
          onClick={() => onGenerateQuestions(segment.id)}
        >
          {generatingQuestionsSegmentId === segment.id && (
            <Loader2 className="h-3 w-3 animate-spin" />
          )}
          {t('segments.batchGenerateQuestions')}
        </Badge>
        <Badge
          variant="warning"
          className="cursor-pointer"
          onClick={() => onBatchImportQuestions(segment.id)}
        >
          {t('segments.batchImportQuestions')}
        </Badge>
      </div>

      {isExpanded && (
        <div className="bg-[#F9FAFB] p-4 mt-4">
          <Badge variant="warning" className="cursor-pointer">
            {t('segments.questions')}
          </Badge>

          {questionsLoading ? (
            <div className="flex justify-center my-4">
              <Skeleton className="h-6 w-24" />
            </div>
          ) : questionsData && questionsData.length > 0 ? (
            <div className="space-y-2 mt-4">
              {questionsData.map((question, index) => (
                <div
                  key={question.id || index}
                  className="p-2 border-b border-[#D8D8D8] group relative"
                >
                  <div className="flex justify-between items-center">
                    <p className="text-sm pr-16">
                      {index + 1}、{question.question}
                    </p>
                    <div className="absolute right-2 flex space-x-2">
                      <Edit
                        className="h-4 w-4 mr-2 cursor-pointer"
                        onClick={() => onEditQuestion(segment.id, question.id, question.question)}
                      />
                      <Trash2
                        className="h-4 w-4 mr-2 cursor-pointer"
                        onClick={() => onDeleteQuestion(segment.id, question.id)}
                      />
                    </div>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className="text-sm text-muted-foreground text-center my-4">
              {t('segments.noData')}
            </div>
          )}
        </div>
      )}
    </div>
  );
});
