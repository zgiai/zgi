'use client';

import React from 'react';
import { Check } from 'lucide-react';
import { cn } from '@/lib/utils';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';

export type StepStatus = 'completed' | 'current' | 'pending';

interface ProgressStep {
  key: number;
  status: StepStatus;
}

export interface StepInfo {
  title: React.ReactNode;
  description?: React.ReactNode;
}

export interface StepsProgressBarProps {
  /** Current step number (1-based) */
  currentStep: number;
  /** Total number of steps */
  totalSteps: number;
  /** Provide step title/description */
  getStepInfo: (stepNumber: number) => StepInfo;
  /** Click handler for step navigation */
  onStepClick?: (step: number) => void;
  /** Whether to allow clicking on completed steps */
  allowStepNavigation?: boolean;
  /** Additional CSS classes */
  className?: string;
}

/**
 * Generic steps progress bar with tooltips
 * - Pure presentational component; callers supply i18n strings via getStepInfo
 * - Style aligned with dataset creation progress bar
 */
export function StepsProgressBar({
  currentStep,
  totalSteps,
  getStepInfo,
  onStepClick,
  allowStepNavigation = true,
  className,
}: StepsProgressBarProps) {
  // Generate steps based on current step and total steps
  const steps: ProgressStep[] = Array.from({ length: totalSteps }, (_, index) => {
    const stepNumber = index + 1;
    let status: StepStatus;
    if (stepNumber < currentStep) status = 'completed';
    else if (stepNumber === currentStep) status = 'current';
    else status = 'pending';
    return { key: stepNumber, status };
  });

  const handleStepClick = (stepNumber: number) => {
    if (allowStepNavigation && stepNumber <= currentStep && onStepClick) {
      onStepClick(stepNumber);
    }
  };

  const progressWidth =
    totalSteps > 1 ? `calc(${((currentStep - 1) / (totalSteps - 1)) * 100}% - 1.5rem)` : '0%';

  return (
    <div
      className={cn('w-full bg-muted rounded-lg p-2 flex items-center justify-center', className)}
    >
      {/* Progress Steps */}
      <div className="relative w-full max-w-2xl">
        {/* Progress Line Background */}
        <div className="absolute top-1/2 -translate-y-1/2 left-3 right-3 h-1 bg-muted-foreground rounded-full" />

        {/* Active Progress Line */}
        <div
          className="absolute top-1/2 -translate-y-1/2 h-1 bg-highlight rounded-full transition-all duration-500 ease-out"
          style={{ width: progressWidth }}
        />

        {/* Steps Container */}

        <div className="relative flex justify-between">
          {steps.map(step => {
            const stepInfo = getStepInfo(step.key);
            const isClickable = allowStepNavigation && step.key <= currentStep;

            return (
              <Tooltip key={step.key}>
                <TooltipTrigger asChild>
                  <div
                    className={cn('bg-muted')}
                    onClick={() => isClickable && handleStepClick(step.key)}
                  >
                    <div
                      className={cn(
                        'relative flex items-center gap-2 px-2 py-1 rounded-lg transition-all duration-200',
                        isClickable ? 'cursor-pointer' : 'cursor-default'
                      )}
                    >
                      {/* Step Circle */}
                      <div
                        className={cn(
                          'flex h-6 w-6 items-center justify-center rounded-full text-base font-normal transition-all duration-200 z-10',
                          step.status === 'completed' && ['bg-highlight text-white'],
                          step.status === 'current' && ['bg-highlight text-white'],
                          step.status === 'pending' && ['bg-muted-foreground text-white']
                        )}
                      >
                        {step.status === 'completed' ? (
                          <Check size={16} />
                        ) : (
                          <span>{step.key}</span>
                        )}
                      </div>

                      {/* Step Title */}
                      <span
                        className={cn(
                          'text-base font-normal',
                          step.status === 'completed' && 'text-highlight',
                          step.status === 'current' && 'text-highlight',
                          step.status === 'pending' && 'text-muted-foreground'
                        )}
                      >
                        {stepInfo.title}
                      </span>
                    </div>
                  </div>
                </TooltipTrigger>
                {stepInfo.description && (
                  <TooltipContent className="z-50">
                    <p className="max-w-xs">{stepInfo.description}</p>
                  </TooltipContent>
                )}
              </Tooltip>
            );
          })}
        </div>
      </div>
    </div>
  );
}
