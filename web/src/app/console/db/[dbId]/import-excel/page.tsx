'use client';

import { use } from 'react';
import { ExcelImportShell } from '@/components/db/excel-import';

interface PageProps {
  params: Promise<{ dbId: string }>;
}

export default function DbExcelImportPage({ params }: PageProps) {
  const { dbId } = use(params);
  return <ExcelImportShell dbId={dbId} />;
}
