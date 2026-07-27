import type { ReactNode } from 'react';
import ModelChannelWorkspaceNav from '@/components/providers/model-channel-workspace-nav';

export default function ChannelLayout({ children }: { children: ReactNode }): JSX.Element {
  return (
    <div className="flex h-full min-h-0 flex-col">
      <ModelChannelWorkspaceNav />
      <div className="min-h-0 flex-1">{children}</div>
    </div>
  );
}
