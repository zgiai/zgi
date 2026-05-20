import { redirect } from 'next/navigation';

/**
 * Workspace page - redirects to members page
 */
export default function WorkspacePage() {
  redirect('/console/workspace/members');
}
