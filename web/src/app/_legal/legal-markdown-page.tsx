import { readFile } from 'node:fs/promises';
import path from 'node:path';
import ReactMarkdown from 'react-markdown';
import type { Components } from 'react-markdown';
import remarkGfm from 'remark-gfm';

type LegalDocument = 'privacy' | 'terms';

const documentFiles: Record<LegalDocument, string> = {
  privacy: 'privacy.md',
  terms: 'terms.md',
};

const documentUrlEnvNames: Record<LegalDocument, string> = {
  privacy: 'LEGAL_PRIVACY_MD_URL',
  terms: 'LEGAL_TERMS_MD_URL',
};

function getEnv(name: string): string | undefined {
  const value = process.env[name]?.trim();
  return value || undefined;
}

function joinUrl(baseUrl: string, filename: string): string {
  return `${baseUrl.replace(/\/+$/, '')}/${filename}`;
}

function getRemoteMarkdownUrl(document: LegalDocument): string | undefined {
  const explicitUrl = getEnv(documentUrlEnvNames[document]);
  if (explicitUrl) return explicitUrl;

  const baseUrl = getEnv('LEGAL_DOCS_BASE_URL') ?? getEnv('NEXT_PUBLIC_LEGAL_DOCS_BASE_URL');
  return baseUrl ? joinUrl(baseUrl, documentFiles[document]) : undefined;
}

async function fetchRemoteMarkdown(url: string): Promise<string | undefined> {
  try {
    const response = await fetch(url, { cache: 'no-store' });
    if (!response.ok) return undefined;
    return response.text();
  } catch {
    return undefined;
  }
}

async function readLocalMarkdown(document: LegalDocument): Promise<string> {
  const filePath = path.join(process.cwd(), 'public', 'legal', documentFiles[document]);
  return readFile(filePath, 'utf8');
}

async function loadMarkdown(document: LegalDocument): Promise<string> {
  const remoteUrl = getRemoteMarkdownUrl(document);
  if (remoteUrl) {
    const remoteMarkdown = await fetchRemoteMarkdown(remoteUrl);
    if (remoteMarkdown) return remoteMarkdown;
  }

  return readLocalMarkdown(document);
}

const markdownComponents: Components = {
  a({ href, children, ...props }) {
    const isExternal = href?.startsWith('http://') || href?.startsWith('https://');
    return (
      <a
        href={href}
        target={isExternal ? '_blank' : undefined}
        rel={isExternal ? 'noopener noreferrer' : undefined}
        {...props}
      >
        {children}
      </a>
    );
  },
};

interface LegalMarkdownPageProps {
  document: LegalDocument;
}

export default async function LegalMarkdownPage({ document }: LegalMarkdownPageProps) {
  const markdown = await loadMarkdown(document);

  return (
    <main className="md-page">
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
        {markdown}
      </ReactMarkdown>
    </main>
  );
}
