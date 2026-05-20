'use client';

import { use } from 'react';
import TableData from '@/components/db/table-data';

interface PageProps {
  params: Promise<{ dbId: string; tableId: string }>;
}

export default function DbTableDetailPage({ params }: PageProps) {
  const { dbId, tableId } = use(params);
  return (
    <div className="p-6 h-full flex flex-col w-full overflow-y-auto">
      <TableData dbId={dbId} tableId={tableId} />
    </div>
  );
}
