import React, { useCallback, useMemo, useState } from 'react';

import { Button } from '@/components/ui/button';
import { CodeEditor, type EditorLanguage } from '@/components/ui/code-editor';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { useT } from '@/i18n';
import { ensureUniqueIdentifier, sanitizeIdentifier } from '@/utils/validation';

import { useLocalNodeData } from '../../../hooks';
import {
  selectorsEqual,
  VariableBindingEditor,
  type VariableBindingEditorAdapter,
} from '../../../common/variable-binding-editor';
import SortableListSection from '../../../common/sortable-list/sortable-list-section';
import type { CodeNodeData } from '../config';
import OutputRow from './output-row';
import { useCodeLanguage, type EditorLanguageType } from './hooks/use-code-language';
import { useCodeOutputs } from './hooks/use-code-outputs';

interface CodeManagerProps {
  id: string;
  readOnly?: boolean;
}

interface LanguageChangeConfirmState {
  open: boolean;
  fromLang: EditorLanguageType;
  toLang: EditorLanguageType;
  resolve: ((confirmed: boolean) => void) | null;
}

type CodeInputRow = CodeNodeData['variables'][number];

const CODE_INPUT_ADAPTER: VariableBindingEditorAdapter<CodeInputRow> = {
  createRow: rows => {
    const base = `var${rows.length + 1}`;
    return {
      variable: ensureUniqueIdentifier(
        sanitizeIdentifier(base),
        rows.map(item => item.variable || '')
      ),
      value_selector: [],
      value_type: 'string',
    };
  },
  isRowEqual: (left, right) =>
    left.variable === right.variable &&
    left.value_type === right.value_type &&
    selectorsEqual(left.value_selector, right.value_selector),
  getName: row => row.variable || '',
  setName: (row, name) => (row.variable === name ? row : { ...row, variable: name }),
  getSelector: row =>
    Array.isArray(row.value_selector) &&
    row.value_selector.length >= 2 &&
    typeof row.value_selector[0] === 'string' &&
    typeof row.value_selector[1] === 'string'
      ? row.value_selector
      : undefined,
  normalizeRowOnBlur: ({ row, rows, index }) => {
    const normalized = ensureUniqueIdentifier(
      sanitizeIdentifier(row.variable || ''),
      rows.filter((_, rowIndex) => rowIndex !== index).map(item => item.variable || '')
    );

    if (normalized === row.variable) {
      return row;
    }

    return { ...row, variable: normalized };
  },
  applySelectorChange: ({ row, payload }) => {
    const nextSelector = payload.valuePath;
    const nextType = (payload.type ?? row.value_type) as CodeInputRow['value_type'];

    if (selectorsEqual(row.value_selector, nextSelector) && row.value_type === nextType) {
      return row;
    }

    return {
      ...row,
      value_selector: nextSelector,
      value_type: nextType,
    };
  },
};

/**
 * @component CodeManager
 * @category Feature
 * @status Stable
 * @description Code-node editor that manages inputs, code content, and declared outputs.
 * @usage Use in the node panel when configuring workflow code execution nodes.
 * @example
 * <CodeManager id={nodeId} readOnly={false} />
 */
const CodeManager: React.FC<CodeManagerProps> = ({ id, readOnly = false }) => {
  const t = useT('nodes');

  const [confirmState, setConfirmState] = useState<LanguageChangeConfirmState>({
    open: false,
    fromLang: 'python3',
    toLang: 'js',
    resolve: null,
  });

  const handleLanguageChangeConfirm = useCallback(
    (
      fromLang: EditorLanguageType,
      toLang: EditorLanguageType,
      _currentCode: string,
      _newDefaultCode: string
    ): Promise<boolean> => {
      return new Promise(resolve => {
        setConfirmState({
          open: true,
          fromLang,
          toLang,
          resolve,
        });
      });
    },
    []
  );

  const handleConfirmOverwrite = useCallback(() => {
    if (confirmState.resolve) {
      confirmState.resolve(true);
    }
    setConfirmState(prev => ({ ...prev, open: false, resolve: null }));
  }, [confirmState]);

  const handleCancelOverwrite = useCallback(() => {
    if (confirmState.resolve) {
      confirmState.resolve(false);
    }
    setConfirmState(prev => ({ ...prev, open: false, resolve: null }));
  }, [confirmState]);

  const { codeValue, setCodeLocal, editorLanguage, handleLangChange } = useCodeLanguage(
    id,
    readOnly,
    {
      onLanguageChangeConfirm: handleLanguageChangeConfirm,
    }
  );

  const { localData: codeInputsRaw, setLocalData: setCodeInputs } = useLocalNodeData<CodeInputRow[]>(
    id,
    {
      path: 'variables',
      delay: 400,
    }
  );
  const codeInputs = useMemo(() => codeInputsRaw || [], [codeInputsRaw]);

  const {
    rows,
    items,
    sensors,
    handleAddOutput,
    handleRemoveOutput,
    handleOutputKeyChangeAtIndex,
    handleOutputTypeChange,
    handleDragEnd,
  } = useCodeOutputs(id);

  const getLanguageDisplayName = (lang: EditorLanguageType) => {
    return lang === 'js' ? 'JavaScript' : 'Python 3';
  };

  return (
    <div className="space-y-6">
      <VariableBindingEditor
        rows={codeInputs}
        onChange={setCodeInputs}
        labels={{
          title: t('code.inputs'),
          addLabel: t('code.addVariable'),
          emptyText: t('code.noInputs'),
          namePlaceholder: t('code.varNamePlaceholder'),
          removeLabel: () => t('common.remove'),
        }}
        adapter={CODE_INPUT_ADAPTER}
        nodeId={id}
        readOnly={readOnly}
      />

      <div className="space-y-2">
        <div className="text-sm font-medium">{t('code.editor')}</div>
        <CodeEditor
          value={codeValue}
          onChange={setCodeLocal}
          language={editorLanguage}
          onLanguageChange={handleLangChange as (language: EditorLanguage) => void}
          allowLanguages={['python3', 'js']}
          readOnly={readOnly}
          height={320}
          resizable
          minHeight={200}
          maxHeight={900}
        />
      </div>

      <SortableListSection
        title={t('end.outputs.title')}
        addLabel={t('end.outputs.addVariable')}
        emptyText={t('code.noOutputs')}
        isReadOnly={readOnly}
        items={items}
        sensors={sensors}
        onDragEnd={handleDragEnd}
        onAdd={handleAddOutput}
        renderRow={(index: number) => (
          <OutputRow
            idx={index}
            committedKey={rows[index]?.key || ''}
            onKeyChange={handleOutputKeyChangeAtIndex}
            typeValue={rows[index]?.type || 'string'}
            onTypeChange={handleOutputTypeChange}
            onRemove={handleRemoveOutput}
            isReadOnly={readOnly}
          />
        )}
      />

      <Dialog open={confirmState.open} onOpenChange={open => !open && handleCancelOverwrite()}>
        <DialogContent className="max-w-[400px] p-0 overflow-hidden">
          <DialogHeader className="pb-2">
            <DialogTitle className="text-xl font-black tracking-tight flex items-center gap-3">
              <div className="h-8 w-8 bg-amber-100 text-amber-500 flex items-center justify-center rounded-lg">
                <span className="text-lg">!</span>
              </div>
              {t('code.languageChange.title')}
            </DialogTitle>
          </DialogHeader>

          <DialogBody className="py-6 space-y-4">
            <div className="bg-amber-50/50 p-4 rounded-2xl border border-amber-100 text-sm font-medium leading-relaxed text-neutral-600">
              {t('code.languageChange.description', {
                from: getLanguageDisplayName(confirmState.fromLang),
                to: getLanguageDisplayName(confirmState.toLang),
              })}
            </div>
          </DialogBody>

          <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t font-medium">
            <Button variant="ghost" className="font-semibold" onClick={handleCancelOverwrite}>
              {t('code.languageChange.keepCode')}
            </Button>
            <Button onClick={handleConfirmOverwrite} size="lg" className="px-10 font-bold shadow-sm">
              {t('code.languageChange.useTemplate')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
};

export default React.memo(CodeManager);
