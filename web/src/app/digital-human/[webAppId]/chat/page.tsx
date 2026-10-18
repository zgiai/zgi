import { redirect } from 'next/navigation';

interface DigitalHumanWebAppChatRedirectProps {
  params: Promise<{ webAppId: string }> | { webAppId: string };
  searchParams?:
    | Promise<Record<string, string | string[] | undefined>>
    | Record<string, string | string[] | undefined>;
}

export default async function DigitalHumanWebAppChatRedirect({
  params,
  searchParams,
}: DigitalHumanWebAppChatRedirectProps) {
  const { webAppId } = await params;
  const resolvedSearch = searchParams ? await searchParams : {};
  const query = new URLSearchParams();
  Object.entries(resolvedSearch).forEach(([key, value]) => {
    if (Array.isArray(value)) value.forEach(item => query.append(key, item));
    else if (value !== undefined) query.set(key, value);
  });
  const suffix = query.toString();
  redirect(`/digital-human/${encodeURIComponent(webAppId)}${suffix ? `?${suffix}` : ''}`);
}
