import { ApprovalFormPageClient } from './page-client';

interface ApprovalFormPageProps {
  params: Promise<{ token: string }> | { token: string };
}

export default async function ApprovalFormPage({ params }: ApprovalFormPageProps) {
  const resolvedParams = await params;
  return <ApprovalFormPageClient token={resolvedParams.token} />;
}
