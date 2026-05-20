'use client';

// Create page under [tableId]:
// Step 1 – call AI to generate table structure (left: prompt + file selection, right: result)
// Step 2 – merge AI columns with existing columns, allow editing, then save

import { use } from 'react';
import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { ChevronRight, ChevronLeft, ArrowLeft } from 'lucide-react';
import { TableCreationProgressBar } from '@/components/db/table-creation-progress-bar';
import { useT } from '@/i18n';
import { StepOne, StepTwo } from '@/components/db/table-create';
import type { DbTableColumn } from '@/services/types/db';
import Link from 'next/link';

interface PageProps {
  params: Promise<{ dbId: string; tableId: string }>;
}

type Step = 1 | 2;

export default function DbTableCreatePage({ params }: PageProps) {
  const { dbId, tableId } = use(params);
  const t = useT('dbs');

  // Step control
  const [step, setStep] = useState<Step>(1);

  // AI result from StepOne
  const [aiColumns, setAiColumns] = useState<DbTableColumn[]>([]);

  // StepOne handles its own inputs, preview and analysis

  // Reset to step 1 when going back
  const onPrevStep = () => {
    setStep(1);
  };

  return (
    <div className="p-6 h-full flex flex-col w-full overflow-hidden">
      {/* Progress bar for the two-step creation flow */}
      <div className="mb-4">
        <TableCreationProgressBar
          currentStep={step}
          totalSteps={2}
          allowStepNavigation
          onStepClick={s => setStep(s as Step)}
        />
      </div>
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-1">
          <Link
            href={`/console/db/${dbId}/table/${tableId}`}
            className="hover:bg-muted flex justify-center items-center w-9 h-9 rounded-md"
          >
            <ArrowLeft className="h-5 w-5" />
          </Link>
          <span className="font-semibold text-lg">{t('createPage.headerTitle')}</span>
        </div>
        <div className="flex items-center gap-2">
          {step === 2 ? (
            <Button variant="outline" onClick={onPrevStep} className="gap-1">
              <ChevronLeft className="h-4 w-4" /> {t('createPage.prevStep')}
            </Button>
          ) : (
            <Button onClick={() => setStep(2)} disabled={aiColumns.length === 0} className="gap-1">
              {t('createPage.nextStep')} <ChevronRight className="h-4 w-4" />
            </Button>
          )}
        </div>
      </div>

      {step === 1 && (
        <StepOne dataSourceId={dbId} onAnalyzeDone={setAiColumns} initialAiColumns={aiColumns} />
      )}

      {step === 2 && <StepTwo dbId={dbId} tableId={tableId} aiColumns={aiColumns} />}
    </div>
  );
}
