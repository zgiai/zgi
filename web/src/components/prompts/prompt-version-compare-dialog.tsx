'use client';

import { useEffect, useMemo, useState } from 'react';
import { GitCompare } from 'lucide-react';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import type { PromptVersion } from '@/services/types/prompt';

interface PromptVersionCompareDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  versions: PromptVersion[];
}

function toPromptText(version?: PromptVersion): string {
  if (!version) return '';
  return typeof version.content === 'string' ? version.content : JSON.stringify(version.content, null, 2);
}

export function PromptVersionCompareDialog({
  open,
  onOpenChange,
  versions,
}: PromptVersionCompareDialogProps) {
  const t = useT('prompts');
  const [leftVersionId, setLeftVersionId] = useState('');
  const [rightVersionId, setRightVersionId] = useState('');

  useEffect(() => {
    if (!open) return;
    const latest = versions[0];
    const previous = versions[1] ?? versions[0];
    setLeftVersionId(latest?.id ?? '');
    setRightVersionId(previous?.id ?? '');
  }, [open, versions]);

  const leftVersion = useMemo(
    () => versions.find(version => version.id === leftVersionId) ?? versions[0],
    [leftVersionId, versions]
  );
  const rightVersion = useMemo(
    () => versions.find(version => version.id === rightVersionId) ?? versions[1] ?? versions[0],
    [rightVersionId, versions]
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-6xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <GitCompare className="h-5 w-5" />
            {t('compare.title')}
          </DialogTitle>
        </DialogHeader>
        <DialogBody className="space-y-5">
          <div className="text-sm text-muted-foreground">{t('compare.description')}</div>
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-5">
            <div className="space-y-3">
              <div className="space-y-2">
                <Label>{t('compare.leftVersion')}</Label>
                <Select value={leftVersionId} onValueChange={setLeftVersionId}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {versions.map(version => (
                      <SelectItem key={version.id} value={version.id}>
                        v{version.version}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              {leftVersion ? (
                <div className="rounded-lg border p-3 space-y-2">
                  <div className="text-xs text-muted-foreground">
                    v{leftVersion.version} · {new Date(leftVersion.updated_at).toLocaleString()}
                  </div>
                  <Textarea value={toPromptText(leftVersion)} readOnly className="min-h-[320px] font-mono text-xs" />
                </div>
              ) : null}
            </div>

            <div className="space-y-3">
              <div className="space-y-2">
                <Label>{t('compare.rightVersion')}</Label>
                <Select value={rightVersionId} onValueChange={setRightVersionId}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {versions.map(version => (
                      <SelectItem key={version.id} value={version.id}>
                        v{version.version}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              {rightVersion ? (
                <div className="rounded-lg border p-3 space-y-2">
                  <div className="text-xs text-muted-foreground">
                    v{rightVersion.version} · {new Date(rightVersion.updated_at).toLocaleString()}
                  </div>
                  <Textarea value={toPromptText(rightVersion)} readOnly className="min-h-[320px] font-mono text-xs" />
                </div>
              ) : null}
            </div>
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default PromptVersionCompareDialog;
