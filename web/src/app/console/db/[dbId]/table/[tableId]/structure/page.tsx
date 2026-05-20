'use client';

import { use } from 'react';
import TableColumns from '@/components/db/table-columns';

interface PageProps {
  params: Promise<{ dbId: string; tableId: string }>;
}

export default function DbTableStructurePage({ params }: PageProps) {
  const { dbId, tableId } = use(params);
  return (
    <div className="p-6 h-full flex flex-col w-full overflow-hidden">
      <TableColumns dbId={dbId} tableId={tableId} />
    </div>
  );
}
