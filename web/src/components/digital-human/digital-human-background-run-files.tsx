'use client';

import { ProtectedGeneratedFileCard } from '@/components/chat/variants/aichat/protected-generated-file-card';
import { digitalHumanService } from '@/services/digital-human.service';
import { useT } from '@/i18n';
import type { DigitalHumanRun } from '@/services/types/digital-human';

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export function DigitalHumanBackgroundRunFiles({
  digitalHumanId,
  run,
}: {
  digitalHumanId: string;
  run: DigitalHumanRun | null;
}) {
  const t = useT('webapp');
  if (!run || !uuid.test(digitalHumanId) || !uuid.test(run.id)) return null;
  const origin = run.invocation_event.origin;
  if (!origin || typeof origin !== 'object' || !('kind' in origin)) return null;
  if (origin.kind !== 'schedule' && origin.kind !== 'channel') return null;
  const entries = run.execution_trace.generated_files;
  if (!Array.isArray(entries)) return null;
  const seen = new Set<string>();
  const files = entries
    .filter(
      (
        entry
      ): entry is {
        file_id: string;
        filename?: string;
        mime_type?: string;
        size?: number;
        expires_at?: number;
        availability?: string;
      } => {
        if (!entry || typeof entry !== 'object' || typeof entry.file_id !== 'string') return false;
        if (!uuid.test(entry.file_id) || seen.has(entry.file_id)) return false;
        seen.add(entry.file_id);
        return true;
      }
    )
    .slice(0, 8);
  if (!files.length) return null;
  return (
    <div className="flex max-h-40 flex-col gap-2 overflow-y-auto">
      {files.map(file => {
        const filename =
          typeof file.filename === 'string' && file.filename
            ? file.filename
            : t('consoleChat.attachments.generatedFile');
        return (
          <ProtectedGeneratedFileCard
            key={file.file_id}
            conversationId={run.id}
            readFile={(_, id, signal) =>
              digitalHumanService.downloadBackgroundFile(digitalHumanId, run.id, id, signal)
            }
            file={{
              file_id: file.file_id,
              filename,
              mime_type:
                typeof file.mime_type === 'string' ? file.mime_type : 'application/octet-stream',
              size: typeof file.size === 'number' ? file.size : 0,
              expires_at: typeof file.expires_at === 'number' ? file.expires_at : undefined,
              availability: typeof file.availability === 'string' ? file.availability : undefined,
              artifact_type: 'file',
              access_mode: 'authenticated',
              skill_id: '',
              tool_name: '',
              extension: filename.includes('.') ? `.${filename.split('.').pop()}` : '',
              url: '',
              transfer_method: 'tool_file',
              created_at: 0,
            }}
          />
        );
      })}
    </div>
  );
}
