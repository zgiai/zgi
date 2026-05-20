'use client';

import { useT, DbSuffix } from '@/i18n';
import { StepsProgressBar, type StepInfo } from '@/components/common/steps-progress-bar';

export interface TableIngestProgressBarProps {
  /** Current step number (1-based) */
  currentStep: number;
  /** Total number of steps, defaults to 2 for ingest flow */
  totalSteps?: number;
  /** Click handler for step navigation */
  onStepClick?: (step: number) => void;
  /** Whether to allow clicking on completed steps */
  allowStepNavigation?: boolean;
  /** Additional CSS classes */
  className?: string;
}

/**
 * Page-specific progress bar for table data ingestion.
 * - Step 1: Select Files
 * - Step 2: Recognition & Preview
 * Internationalized via `dbs.tableIngest.progress.*` keys.
 */
export function TableIngestProgressBar({
  currentStep,
  totalSteps = 2,
  onStepClick,
  allowStepNavigation = true,
  className,
}: TableIngestProgressBarProps) {
  const t = useT();

  const getStepInfo = (stepNumber: number): StepInfo => {
    const titles: DbSuffix[] = [
      'tableIngest.progress.step1',
      'tableIngest.progress.step2',
      'tableIngest.progress.step3',
      'tableIngest.progress.step4',
    ];
    const descs: DbSuffix[] = [
      'tableIngest.progress.step1Desc',
      'tableIngest.progress.step2Desc',
      'tableIngest.progress.step3Desc',
      'tableIngest.progress.step4Desc',
    ];

    const titleKey = titles[stepNumber - 1] || titles[0];
    const descKey = descs[stepNumber - 1] || descs[0];

    return {
      title: t(`dbs.${titleKey}`),
      description: t(`dbs.${descKey}`),
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

export default TableIngestProgressBar;
