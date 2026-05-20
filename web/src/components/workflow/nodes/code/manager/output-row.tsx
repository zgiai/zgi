'use client';

import React from 'react';
import { Trash } from 'lucide-react';

import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { WorkflowIdentifierInput } from '@/components/workflow/common/variable-binding-editor/identifier-input';
import type { OutputVariable } from '../../../store/type';
import { useT } from '@/i18n';
import { isValidIdentifier } from '@/utils/validation';

const OUTPUT_TYPES: Array<OutputVariable['type']> = [
  'string',
  'number',
  'boolean',
  'object',
  'file',
  'array[string]',
  'array[number]',
  'array[boolean]',
  'array[object]',
  'array[file]',
];

function isDeleteShortcut(event: React.KeyboardEvent<HTMLElement>): boolean {
  return event.key === 'Delete' || event.key === 'Backspace';
}

function isClipboardShortcut(event: React.KeyboardEvent<HTMLElement>): boolean {
  if (!event.ctrlKey && !event.metaKey) return false;
  const key = event.key.toLowerCase();
  return key === 'c' || key === 'v' || key === 'x';
}

export interface OutputRowProps {
  idx: number;
  committedKey: string;
  onKeyChange: (index: number, newKey: string) => void;
  typeValue: OutputVariable['type'];
  onTypeChange: (committedKey: string, type: OutputVariable['type']) => void;
  onRemove: (committedKey: string) => void;
  isReadOnly: boolean;
}

/**
 * @component OutputRow
 * @category Feature
 * @status Stable
 * @description Editable row for code-node declared outputs with stable identifier input behavior.
 * @usage Use inside the code-node outputs list.
 * @example
 * <OutputRow idx={0} committedKey="result" onKeyChange={setKey} typeValue="string" onTypeChange={setType} onRemove={remove} isReadOnly={false} />
 */
const OutputRow: React.FC<OutputRowProps> = ({
  idx,
  committedKey,
  onKeyChange,
  typeValue,
  onTypeChange,
  onRemove,
  isReadOnly,
}) => {
  const t = useT('nodes');
  const invalid = committedKey.trim().length > 0 ? !isValidIdentifier(committedKey) : false;

  return (
    <div
      className="flex items-center gap-1 min-w-0"
      onKeyDownCapture={event => {
        if (isDeleteShortcut(event) || isClipboardShortcut(event)) {
          event.stopPropagation();
        }
      }}
    >
      <div className="flex-1 min-w-0">
        <WorkflowIdentifierInput
          initial={committedKey}
          onCommit={value => onKeyChange(idx, value)}
          placeholder={t('code.varNamePlaceholder')}
          invalid={invalid}
          disabled={isReadOnly}
        />
      </div>

      <div className="shrink-0 w-28">
        <Select
          value={typeValue}
          onValueChange={value => onTypeChange(committedKey, value as OutputVariable['type'])}
          disabled={isReadOnly}
        >
          <SelectTrigger className="w-full shrink-0">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {OUTPUT_TYPES.map(type => (
              <SelectItem key={type} value={type}>
                {type}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <Button
        variant="ghost"
        isIcon
        onClick={() => onRemove(committedKey)}
        disabled={isReadOnly}
        className="shrink-0"
        aria-label={t('common.remove')}
        title={t('common.remove')}
      >
        <Trash className="w-4 h-4" />
      </Button>
    </div>
  );
};

export default React.memo(OutputRow);
