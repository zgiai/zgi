'use client';

import React, { forwardRef } from 'react';
import {
  AutoFileUpload,
  type AutoFileUploadProps,
  type AutoFileUploadRef,
} from './auto-file-upload';
import {
  ManualFileUpload,
  type ManualFileUploadProps,
  type ManualFileUploadRef,
} from './manual-file-upload';
import type { UploadedFile } from '@/services/types/dataset';
import type { FileUploadProcessingMode } from '@/services/types/file';

export interface FileUploadProps
  extends Omit<AutoFileUploadProps, 'autoUpload'>,
    Omit<ManualFileUploadProps, 'autoUpload'> {
  autoUpload?: boolean;
  folderId?: string;
  workspaceId?: string;
  processingMode?: FileUploadProcessingMode;
  showSystemSelect?: boolean;
  isTemporary?: boolean;
  allowWorkspaceSwitch?: boolean;
}

export type FileUploadRef = AutoFileUploadRef & ManualFileUploadRef;

export const FileUpload = forwardRef<FileUploadRef, FileUploadProps>(
  ({ autoUpload = true, ...props }, ref) => {
    if (autoUpload) {
      // Auto upload mode: use AutoFileUpload component
      return (
        <AutoFileUpload
          ref={ref as React.Ref<AutoFileUploadRef>}
          {...(props as AutoFileUploadProps)}
        />
      );
    } else {
      // Manual upload mode: use ManualFileUpload component
      return (
        <ManualFileUpload
          ref={ref as React.Ref<ManualFileUploadRef>}
          {...(props as ManualFileUploadProps)}
        />
      );
    }
  }
);

FileUpload.displayName = 'FileUpload';

export { AutoFileUpload, ManualFileUpload };
export type { AutoFileUploadProps, AutoFileUploadRef, ManualFileUploadProps, ManualFileUploadRef };
export type { UploadedFile };
