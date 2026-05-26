import type { Metadata } from 'next';
import LegalMarkdownPage from '../_legal/legal-markdown-page';

export const dynamic = 'force-dynamic';

export const metadata: Metadata = {
  title: 'ZGI隐私政策',
};

export default function PrivacyPage() {
  return <LegalMarkdownPage document="privacy" />;
}
