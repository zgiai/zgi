import { notFound, redirect } from 'next/navigation';
import {
  isValidShortToken,
  normalizeShortToken,
  resolveShortLinkTargetPath,
} from '@/services/shortlink.service';

interface ShortLinkPageProps {
  params: Promise<{ shortToken: string }> | { shortToken: string };
}

export default async function ShortLinkPage({ params }: ShortLinkPageProps) {
  const resolvedParams = await params;
  const shortToken = normalizeShortToken(resolvedParams.shortToken);
  if (!isValidShortToken(shortToken)) {
    notFound();
  }

  const targetPath = await resolveShortLinkTargetPath(shortToken);
  if (!targetPath) {
    notFound();
  }

  redirect(targetPath);
}
