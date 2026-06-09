'use client';

import { useEffect, useState } from 'react';
import { FileUp } from 'lucide-react';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { contentParseService } from '@/services/content-parse.service';
import { useUploadConfig } from '@/hooks/use-upload';
import type {
  ContentParsePlaygroundProviderStatus,
  ParseProviderKey,
} from '@/services/types/content-parse';
import type {
  FileItem,
  FileParseProviderKey,
  FileUploadProcessingMode,
} from '@/services/types/file';

const FILE_PARSE_PROVIDERS: FileParseProviderKey[] = [
  'auto',
  'mineru',
  'local',
  'reducto',
  'hyperparse_api',
];

interface ReplaceDocumentDialogProps {
  open: boolean;
  file: FileItem | null;
  loading?: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: (
    file: FileItem,
    replacementFile: File,
    processingMode: FileUploadProcessingMode,
    parseProvider: FileParseProviderKey
  ) => void;
}

export function ReplaceDocumentDialog({
  open,
  file,
  loading = false,
  onOpenChange,
  onConfirm,
}: ReplaceDocumentDialogProps) {
  const { files: t, common } = useT();
  const { data: uploadConfig } = useUploadConfig({ enabled: open });
  const [replacementFile, setReplacementFile] = useState<File | null>(null);
  const [processingMode, setProcessingMode] = useState<FileUploadProcessingMode>('process_now');
  const [parseProvider, setParseProvider] = useState<FileParseProviderKey>('auto');
  const [providerStatuses, setProviderStatuses] = useState<
    Partial<Record<ParseProviderKey, ContentParsePlaygroundProviderStatus>>
  >({});
  const [providersLoading, setProvidersLoading] = useState(false);

  const maxSizeMB = uploadConfig?.file_size_limit ?? 15;

  useEffect(() => {
    if (!open) return;
    setReplacementFile(null);
    setProcessingMode('process_now');
    setParseProvider('auto');
  }, [file?.id, open]);

  useEffect(() => {
    if (!open) return;
    let ignore = false;

    const loadProviders = async () => {
      setProvidersLoading(true);
      try {
        const response = await contentParseService.listPlaygroundProviders();
        if (ignore) return;
        const next: Partial<Record<ParseProviderKey, ContentParsePlaygroundProviderStatus>> = {};
        response.data.providers.forEach(provider => {
          next[provider.key] = provider;
        });
        setProviderStatuses(next);
      } catch {
        if (!ignore) {
          setProviderStatuses({});
        }
      } finally {
        if (!ignore) {
          setProvidersLoading(false);
        }
      }
    };

    void loadProviders();

    return () => {
      ignore = true;
    };
  }, [open]);

  const getParseProviderLabel = (provider: FileParseProviderKey) => {
    switch (provider) {
      case 'auto':
        return t('upload.parseProviders.auto');
      case 'mineru':
        return t('upload.parseProviders.mineru');
      case 'local':
        return t('upload.parseProviders.local');
      case 'reducto':
        return t('upload.parseProviders.reducto');
      case 'hyperparse_api':
        return t('upload.parseProviders.hyperparseApi');
      case 'vlm':
        return t('upload.parseProviders.vlm');
      default:
        return provider;
    }
  };

  const handleConfirm = () => {
    if (!file || !replacementFile) return;
    onConfirm(file, replacementFile, processingMode, parseProvider);
  };

  const isFileTooLarge =
    replacementFile !== null && replacementFile.size > maxSizeMB * 1024 * 1024;
  const canSubmit = Boolean(file && replacementFile && !isFileTooLarge && !loading);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[520px] p-0 overflow-hidden">
        <DialogHeader className="border-b pb-4">
          <div className="flex items-start gap-3 pr-8">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
              <FileUp className="h-5 w-5" />
            </div>
            <div className="min-w-0">
              <DialogTitle className="text-lg font-semibold">
                {t('replaceDocument.title')}
              </DialogTitle>
              <DialogDescription className="mt-2 leading-6">
                {t('replaceDocument.description', { name: file?.name ?? '' })}
              </DialogDescription>
            </div>
          </div>
        </DialogHeader>
        <DialogBody className="space-y-5 py-5">
          <div className="space-y-2">
            <Label className="text-sm font-semibold">{t('replaceDocument.newFile')}</Label>
            <Input
              type="file"
              disabled={loading}
              onChange={event => setReplacementFile(event.target.files?.[0] ?? null)}
            />
            {replacementFile ? (
              <p className="text-xs leading-5 text-muted-foreground">
                {t('replaceDocument.selectedFile', { name: replacementFile.name })}
              </p>
            ) : null}
            {isFileTooLarge ? (
              <p className="text-xs leading-5 text-destructive">
                {t('replaceDocument.fileTooLarge', { max: maxSizeMB })}
              </p>
            ) : null}
          </div>

          <div className="space-y-2">
            <Label className="text-sm font-semibold">{t('upload.processingMode')}</Label>
            <Select
              value={processingMode}
              onValueChange={value => setProcessingMode(value as FileUploadProcessingMode)}
              disabled={loading}
            >
              <SelectTrigger className="bg-background">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="process_now">
                  {t('upload.processingModes.processNow.title')}
                </SelectItem>
                <SelectItem value="store_only">
                  {t('upload.processingModes.storeOnly.title')}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-2">
            <Label className="text-sm font-semibold">{t('upload.parseProvider')}</Label>
            <Select
              value={parseProvider}
              onValueChange={value => setParseProvider(value as FileParseProviderKey)}
              disabled={loading || processingMode !== 'process_now'}
            >
              <SelectTrigger className="bg-background" isLoading={providersLoading}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {FILE_PARSE_PROVIDERS.map(provider => {
                  const status = providerStatuses[provider];
                  const hasProviderStatuses = Object.keys(providerStatuses).length > 0;
                  const isUnavailable =
                    provider !== 'auto' && !providersLoading && hasProviderStatuses
                      ? !status?.selectable
                      : false;

                  return (
                    <SelectItem key={provider} value={provider} disabled={isUnavailable}>
                      <span className="flex w-full min-w-0 items-center justify-between gap-3">
                        <span className="truncate">
                          {status?.display_name || getParseProviderLabel(provider)}
                        </span>
                        {provider !== 'auto' && isUnavailable ? (
                          <span className="shrink-0 text-xs text-muted-foreground">
                            {t('upload.parseProviderUnavailable')}
                          </span>
                        ) : null}
                      </span>
                    </SelectItem>
                  );
                })}
              </SelectContent>
            </Select>
            <p className="text-xs leading-5 text-muted-foreground">
              {processingMode === 'process_now'
                ? t('replaceDocument.processingHint')
                : t('replaceDocument.storeOnlyHint')}
            </p>
          </div>
        </DialogBody>
        <DialogFooter className="border-t bg-muted/30 px-6 py-4">
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            {common('cancel')}
          </Button>
          <Button type="button" loading={loading} disabled={!canSubmit} onClick={handleConfirm}>
            {t('replaceDocument.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
