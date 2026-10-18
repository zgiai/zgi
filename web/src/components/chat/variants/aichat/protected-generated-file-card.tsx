'use client';

import { useEffect, useId, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { AlertCircle, Download, Eye, FileText, Loader2 } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { UniversalFilePreviewDialog } from '@/components/files/universal-file-preview-dialog';
import { useT } from '@/i18n/translations';
import type { AIChatGeneratedFile, ExecutionFileReader } from '@/services/types/aichat';

interface Props {
  file: AIChatGeneratedFile;
  conversationId: string;
  readFile?: ExecutionFileReader;
}

// Protected artifacts never use URLs from model output or message metadata.
// Preview blobs exist only while the dialog is mounted; downloads always make
// a fresh authenticated request, including after a preview succeeded.
export function ProtectedGeneratedFileCard({ file, conversationId, readFile }: Props) {
  const t = useT('webapp');
  const [preview, setPreview] = useState(false);
  const [downloading, setDownloading] = useState(false);
  const unavailable =
    file.availability === 'expired' ||
    (file.expires_at ? file.expires_at * 1000 <= Date.now() : false);
  const canRead = Boolean(readFile && conversationId && file.file_id) && !unavailable;
  const download = async () => {
    if (!readFile || !canRead || downloading) return;
    setDownloading(true);
    try {
      const blob = await readFile(conversationId, file.file_id);
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement('a');
      anchor.href = url;
      anchor.download = file.filename;
      anchor.click();
      window.setTimeout(() => URL.revokeObjectURL(url), 0);
    } catch {
      toast.error(t('consoleChat.attachments.protectedFileReadFailed'));
    } finally {
      setDownloading(false);
    }
  };
  return (
    <>
      <div
        className="flex min-w-0 max-w-sm items-center gap-3 rounded-md border bg-background px-3 py-2 text-sm shadow-sm"
        title={file.filename}
      >
        <div className="flex size-9 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
          <FileText className="size-4" />
        </div>
        <div className="min-w-0 flex-1">
          <div className="truncate font-medium">{file.filename}</div>
          <div className="text-xs text-muted-foreground">
            {t(
              unavailable
                ? 'consoleChat.attachments.expiredGeneratedFile'
                : 'consoleChat.attachments.generatedFile'
            )}
          </div>
        </div>
        <Button
          type="button"
          isIcon
          variant="ghost"
          className="size-8 shrink-0"
          disabled={!canRead}
          aria-label={t('consoleChat.attachments.previewGeneratedFile')}
          onClick={() => setPreview(true)}
        >
          <Eye className="size-4" />
        </Button>
        <Button
          type="button"
          isIcon
          variant="ghost"
          className="size-8 shrink-0"
          disabled={!canRead || downloading}
          aria-label={t('consoleChat.attachments.downloadGeneratedFile')}
          onClick={() => void download()}
        >
          {downloading ? (
            <Loader2 className="size-4 animate-spin" />
          ) : (
            <Download className="size-4" />
          )}
        </Button>
      </div>
      {preview && readFile && canRead ? (
        <ProtectedPreview
          file={file}
          conversationId={conversationId}
          readFile={readFile}
          close={() => setPreview(false)}
          download={() => void download()}
          downloading={downloading}
        />
      ) : null}
    </>
  );
}

function ProtectedPreview({
  file,
  conversationId,
  readFile,
  close,
  download,
  downloading,
}: Props & {
  readFile: ExecutionFileReader;
  close: () => void;
  download: () => void;
  downloading: boolean;
}) {
  const t = useT('webapp');
  const [url, setURL] = useState('');
  const previewInstance = useId();
  const query = useQuery({
    queryKey: ['execution-file-preview', previewInstance, conversationId, file.file_id],
    queryFn: ({ signal }) => readFile(conversationId, file.file_id, signal),
    gcTime: 0,
    staleTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
  });
  useEffect(() => {
    if (!query.data) return;
    const objectURL = URL.createObjectURL(query.data);
    setURL(objectURL);
    return () => URL.revokeObjectURL(objectURL);
  }, [query.data]);
  if (!url || query.isError) {
    return (
      <Dialog
        open
        onOpenChange={open => {
          if (!open) close();
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{file.filename}</DialogTitle>
          </DialogHeader>
          {query.isError ? (
            <div role="alert" className="flex items-center gap-2 text-sm">
              <AlertCircle className="size-4" />
              {t('consoleChat.attachments.protectedFileReadFailed')}
            </div>
          ) : (
            <Loader2 className="size-5 animate-spin" />
          )}
        </DialogContent>
      </Dialog>
    );
  }
  return (
    <UniversalFilePreviewDialog
      open
      allowExternalHtmlOpen={false}
      onDownload={download}
      isDownloading={downloading}
      onOpenChange={open => {
        if (!open) close();
      }}
      file={{
        id: file.file_id,
        name: file.filename,
        extension: file.extension,
        mimeType: file.mime_type,
        size: file.size,
        previewUrl: url,
        downloadUrl: '',
      }}
    />
  );
}
