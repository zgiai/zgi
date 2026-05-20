'use client';

import React, { useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogBody,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';

interface SaveRecordDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSave: (batchName: string) => void;
  isLoading?: boolean;
}

export function SaveRecordDialog({ open, onOpenChange, onSave, isLoading }: SaveRecordDialogProps) {
  const [batchName, setBatchName] = useState('');

  const handleSave = () => {
    if (batchName.trim()) {
      onSave(batchName.trim());
      setBatchName('');
    }
  };

  const handleCancel = () => {
    setBatchName('');
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[440px] p-0 overflow-hidden">
        <DialogHeader>
          <DialogTitle className="text-xl font-bold tracking-tight">保存测试记录</DialogTitle>
        </DialogHeader>

        <DialogBody className="py-6 space-y-6">
          <div className="space-y-4">
            <Label htmlFor="batch-name" className="text-sm font-semibold">
              记录名称
            </Label>
            <Input
              id="batch-name"
              value={batchName}
              onChange={e => setBatchName(e.target.value)}
              className="h-11 shadow-sm"
              placeholder="请输入测试记录名称"
              onKeyDown={e => {
                if (e.key === 'Enter') {
                  handleSave();
                }
              }}
            />
          </div>
        </DialogBody>

        <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t font-medium">
          <Button
            variant="ghost"
            onClick={handleCancel}
            disabled={isLoading}
            className="font-semibold"
          >
            取消
          </Button>
          <Button
            onClick={handleSave}
            disabled={!batchName.trim() || isLoading}
            className="px-8 font-bold"
          >
            {isLoading ? '保存中...' : '确认保存'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default SaveRecordDialog;
