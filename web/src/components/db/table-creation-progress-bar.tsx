'use client';

import { StepsProgressBar, type StepInfo } from '@/components/common/steps-progress-bar';
import { useT } from '@/i18n';

export interface TableCreationProgressBarProps {
  currentStep: number;
  totalSteps: number;
  onStepClick?: (step: number) => void;
  allowStepNavigation?: boolean;
  className?: string;
}

// Wrapper for DB table creation steps using the generic StepsProgressBar
export function TableCreationProgressBar({
  currentStep,
  totalSteps,
  onStepClick,
  allowStepNavigation = true,
  className,
}: TableCreationProgressBarProps) {
  const t = useT();

  const getStepInfo = (stepNumber: number): StepInfo => {
    const titles = ['tableCreate.steps.generateStructure', 'tableCreate.steps.mergeEdit'];
    const descs = ['tableCreate.steps.generateStructureDesc', 'tableCreate.steps.mergeEditDesc'];

    const titleKey = titles[stepNumber - 1] || titles[0];
    const descKey = descs[stepNumber - 1] || descs[0];

    return {
      title: t(`dbs.${titleKey}` as any, {
        defaultMessage: stepNumber === 1 ? '生成表结构' : '合并并编辑',
      }),
      description: t(`dbs.${descKey}` as any, {
        defaultMessage: stepNumber === 1 ? 'AI生成初始表结构' : '合并AI与现有字段并编辑',
      }),
    };
  };

  return (
    <StepsProgressBar
      currentStep={currentStep}
      totalSteps={totalSteps}
      getStepInfo={getStepInfo}
      onStepClick={onStepClick}
      allowStepNavigation={allowStepNavigation}
      className={className}
    />
  );
}
