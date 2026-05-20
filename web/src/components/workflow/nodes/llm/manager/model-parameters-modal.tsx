'use client';

import React from 'react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogBody,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { Input } from '@/components/ui/input';
import type { LLMNodeData } from '../../../store/type';

interface ModelParametersModalProps {
  isOpen: boolean;
  onClose: () => void;
  completionParams: LLMNodeData['model']['completion_params'];
  onUpdateCompletionParams: (patch: Partial<LLMNodeData['model']['completion_params']>) => void;
}

const ModelParametersModal: React.FC<ModelParametersModalProps> = ({
  isOpen,
  onClose,
  completionParams,
  onUpdateCompletionParams,
}) => {
  const [tempInput, setTempInput] = React.useState<number>(completionParams.temperature as number);

  React.useEffect(() => {
    setTempInput(completionParams.temperature as number);
  }, [completionParams.temperature]);

  const handleSave = () => {
    onUpdateCompletionParams({ temperature: tempInput });
    onClose();
  };

  return (
    <Dialog open={isOpen} onOpenChange={onClose}>
      <DialogContent className="max-w-[440px] p-0 overflow-hidden">
        <DialogHeader className="pb-2">
          <DialogTitle className="text-xl font-bold tracking-tight">Model Parameters</DialogTitle>
        </DialogHeader>

        <DialogBody className="py-6">
          <div className="space-y-5">
            <div className="flex flex-col gap-2.5">
              <Label className="text-sm font-bold px-0.5">Temperature</Label>
              <Input
                type="number"
                step="0.1"
                min={0}
                max={2}
                className="h-11 shadow-sm font-medium rounded-xl border-neutral-200 focus-visible:ring-primary/20"
                value={Number.isFinite(tempInput) ? tempInput : 0}
                onChange={e => setTempInput(Number(e.target.value) || 0)}
              />
              <p className="text-[10px] text-muted-foreground font-medium px-1 uppercase tracking-wider">
                控制生成文本的随机性 (0-2)
              </p>
            </div>
          </div>
        </DialogBody>

        <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t">
          <Button variant="ghost" onClick={onClose} className="font-semibold">
            Cancel
          </Button>
          <Button onClick={handleSave} size="lg" className="px-10 font-bold shadow-sm">
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export default ModelParametersModal;
