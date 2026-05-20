'use client';

import React, { forwardRef, useCallback, useImperativeHandle, useRef } from 'react';
import dynamic from 'next/dynamic';
import type { EditorProps } from '@monaco-editor/react';
import type * as MonacoEditor from 'monaco-editor';
import { cn } from '@/lib/utils';

export interface SqlMonacoEditorHandle {
  focus: () => void;
  insertText: (snippet: string) => void;
}

interface SqlMonacoEditorProps {
  value: string;
  onChange: (next: string) => void;
  className?: string;
  height?: number | string;
  readOnly?: boolean;
  placeholder?: string;
}

const MonacoComponent = dynamic<EditorProps>(() => import('@monaco-editor/react'), { ssr: false });

const DEFAULT_HEIGHT = 220;

const SqlMonacoEditor = forwardRef<SqlMonacoEditorHandle, SqlMonacoEditorProps>(
  ({ value, onChange, className, height = DEFAULT_HEIGHT, readOnly = false }, ref) => {
    const editorRef = useRef<MonacoEditor.editor.IStandaloneCodeEditor | null>(null);

    useImperativeHandle(
      ref,
      () => ({
        focus: () => {
          editorRef.current?.focus();
        },
        insertText: snippet => {
          const editor = editorRef.current;
          if (!editor) return;
          const selection = editor.getSelection();
          if (!selection) return;

          editor.executeEdits('insert-text', [
            {
              range: selection,
              text: snippet,
              forceMoveMarkers: true,
            },
          ]);
          editor.focus();
        },
      }),
      []
    );

    const handleMount = useCallback<NonNullable<EditorProps['onMount']>>((editor, monaco) => {
      editorRef.current = editor;
      monaco.editor.defineTheme('workflow-sql-theme', {
        base: 'vs',
        inherit: true,
        rules: [],
        colors: {
          'editor.background': '#ffffff',
        },
      });
      monaco.editor.setTheme('workflow-sql-theme');

      // Highlight {{#...#}} variables with decorations
      const collection = editor.createDecorationsCollection();
      const placeholderRegex = /\{\{#([a-zA-Z0-9_-]+)(?:\.([a-zA-Z0-9_.-]+))?#\}\}/g;

      const computeDecorations = (): Parameters<typeof collection.set>[0] => {
        const model = editor.getModel();
        if (!model) return [];
        const decorations: MonacoEditor.editor.IModelDeltaDecoration[] = [];
        const lineCount = model.getLineCount();
        for (let lineNumber = 1; lineNumber <= lineCount; lineNumber++) {
          const line = model.getLineContent(lineNumber);
          placeholderRegex.lastIndex = 0;
          let match: RegExpExecArray | null;
          while ((match = placeholderRegex.exec(line))) {
            const startColumn = match.index + 1; // monaco columns are 1-based
            const endColumn = startColumn + match[0].length;
            const isSys = match[1] === 'sys';
            decorations.push({
              range: new monaco.Range(lineNumber, startColumn, lineNumber, endColumn),
              options: {
                inlineClassName: isSys ? 'sql-var-token sql-var-token-sys' : 'sql-var-token',
                stickiness: monaco.editor.TrackedRangeStickiness.NeverGrowsWhenTypingAtEdges,
              },
            });
          }
        }
        return decorations;
      };

      const apply = () => collection.set(computeDecorations());
      apply();

      const _d1 = editor.onDidChangeModelContent(apply);
      const _d2 = editor.onDidChangeModel(apply);
      const _d3 = editor.onDidDispose(() => collection.clear());
      // Ensure listeners are cleaned up when model is disposed as well
      const model = editor.getModel();
      const _d4 = model?.onWillDispose?.(() => collection.clear());

      // No explicit return here; relying on editor disposal
    }, []);

    const handleChange = useCallback<NonNullable<EditorProps['onChange']>>(
      next => {
        if (typeof next === 'string') {
          onChange(next);
        }
      },
      [onChange]
    );

    return (
      <div className={cn('rounded-md border bg-background overflow-hidden', className)}>
        {/* Inline styles for Monaco decorations */}
        <style jsx global>{`
          .monaco-editor .sql-var-token {
            background-color: rgba(59, 130, 246, 0.12);
            border-radius: 4px;
            box-shadow: inset 0 0 0 1px rgba(59, 130, 246, 0.35);
          }
          .monaco-editor .sql-var-token-sys {
            background-color: rgba(16, 185, 129, 0.15);
            box-shadow: inset 0 0 0 1px rgba(16, 185, 129, 0.45);
          }
        `}</style>
        <MonacoComponent
          language="sql"
          theme="workflow-sql-theme"
          value={value}
          onChange={handleChange}
          onMount={handleMount}
          height={height}
          options={{
            readOnly,
            minimap: { enabled: false },
            fontSize: 13,
            automaticLayout: true,
            wordWrap: 'on',
            scrollBeyondLastLine: false,
            renderValidationDecorations: 'on',
            quickSuggestions: false,
            suggestOnTriggerCharacters: false,
            tabSize: 2,
            padding: { top: 8, bottom: 8 },
            contextmenu: false,
          }}
        />
      </div>
    );
  }
);

SqlMonacoEditor.displayName = 'SqlMonacoEditor';

export default SqlMonacoEditor;
