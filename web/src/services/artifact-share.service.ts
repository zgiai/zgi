import { API_URL } from '@/lib/config';

/** What a share link shows: no owner, workspace, or application. */
export interface SharedArtifact {
  title: string;
  kind: 'document' | 'file' | 'image' | 'table';
  version: number;
  file_name?: string;
  mime_type: string;
  size: number;
  /** A document's Markdown. */
  content?: string;
  updated_at: string;
  expires_at?: string;
}

function shareUrl(token: string) {
  return `${API_URL}/console/api/artifact-shares/${encodeURIComponent(token)}`;
}

// A share is opened by people who are not signed in, so it is read without
// the console's client and its session handling.
export const artifactShareService = {
  async getShare(token: string): Promise<SharedArtifact> {
    const response = await fetch(shareUrl(token), { credentials: 'omit', cache: 'no-store' });
    const body = (await response.json().catch(() => null)) as { code?: string | number; data?: SharedArtifact } | null;
    if (!response.ok || !body || String(body.code) !== '0' || !body.data) {
      throw new Error('share unavailable');
    }
    return body.data;
  },

  /** The shared file itself: shown in the page, or downloaded. */
  contentUrl(token: string, download = false) {
    return `${shareUrl(token)}/content${download ? '?download=1' : ''}`;
  },
};
