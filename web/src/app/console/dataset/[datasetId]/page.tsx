import { redirect } from 'next/navigation';

export default async function DatasetPage({ params }: { params: Promise<{ datasetId: string }> }) {
  const { datasetId } = await params;
  redirect(`/console/dataset/${datasetId}/documents`);
}
