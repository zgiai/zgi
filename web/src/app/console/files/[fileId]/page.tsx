import { FileDetailShell } from '@/components/files/detail/file-detail-shell';

export default async function FileDetailPage({
  params,
}: {
  params: Promise<{ fileId: string }>;
}) {
  const { fileId } = await params;

  return <FileDetailShell fileId={fileId} />;
}
