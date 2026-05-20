'use client';

import React from 'react';
import dynamic from 'next/dynamic';
import type { EditorProps } from '@monaco-editor/react';

import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectTrigger,
  SelectContent,
  SelectItem,
  SelectValue,
} from '@/components/ui/select';
import { Check, Copy } from 'lucide-react';

export type EditorLanguage = 'js' | 'python3' | 'json';

interface CodeEditorProps {
  value: string;
  onChange: (value: string) => void;
  language: EditorLanguage;
  onLanguageChange?: (language: EditorLanguage) => void;
  allowLanguages?: EditorLanguage[];
  readOnly?: boolean;
  height?: number | string;
  resizable?: boolean;
  minHeight?: number;
  maxHeight?: number;
  className?: string;
  name?: string;
  // Optional UI controls visibility
  showLanguageSelector?: boolean;
  showCopyButton?: boolean;
  // Disable suggestions/IntelliSense (quickSuggestions, parameter hints, inline suggest)
  disableSuggest?: boolean;
}

const languageToMonaco: Record<EditorLanguage, 'javascript' | 'python' | 'json'> = {
  js: 'javascript',
  python3: 'python',
  json: 'json',
};

const MonacoEditor = dynamic<EditorProps>(
  () => import('@monaco-editor/react').then(m => m.default),
  { ssr: false }
);

export function CodeEditor(props: CodeEditorProps) {
  const {
    value,
    onChange,
    language,
    onLanguageChange,
    allowLanguages = ['js', 'python3'],
    readOnly = false,
    height = 320,
    resizable = false,
    minHeight = 200,
    maxHeight = 900,
    className,
    name,
    showLanguageSelector = true,
    showCopyButton = true,
  } = props;

  const [copied, setCopied] = React.useState(false);

  const monacoLanguage = languageToMonaco[language];

  const handleChange = React.useCallback<NonNullable<EditorProps['onChange']>>(
    next => {
      if (typeof next === 'string') onChange(next);
    },
    [onChange]
  );

  const handleCopy = React.useCallback(async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1200);
    } catch {
      // Ignore clipboard errors
    }
  }, [value]);

  const handleLangChange = React.useCallback(
    (val: string) => {
      if (onLanguageChange) onLanguageChange(val as EditorLanguage);
    },
    [onLanguageChange]
  );

  const [editorHeight, setEditorHeight] = React.useState<number | string>(height);
  React.useEffect(() => {
    setEditorHeight(height);
  }, [height]);

  const dragDataRef = React.useRef<{ startY: number; startH: number } | null>(null);

  const onMouseDownResize = React.useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      if (!resizable || readOnly) return;
      e.preventDefault();
      const startY = e.clientY;
      const startH =
        typeof editorHeight === 'number' ? editorHeight : parseInt(String(editorHeight), 10) || 320;
      dragDataRef.current = { startY, startH };
      const onMove = (ev: MouseEvent) => {
        const info = dragDataRef.current;
        if (!info) return;
        const delta = ev.clientY - info.startY;
        let next = info.startH + delta;
        if (typeof next !== 'number' || Number.isNaN(next)) next = minHeight;
        next = Math.max(minHeight, Math.min(maxHeight, next));
        setEditorHeight(next);
      };
      const onUp = () => {
        dragDataRef.current = null;
        window.removeEventListener('mousemove', onMove);
        window.removeEventListener('mouseup', onUp);
      };
      window.addEventListener('mousemove', onMove);
      window.addEventListener('mouseup', onUp);
    },
    [editorHeight, minHeight, maxHeight, resizable, readOnly]
  );

  return (
    <div className={cn('flex w-full flex-col gap-2', className)}>
      {(showLanguageSelector || showCopyButton) && (
        <div className="flex items-center justify-between gap-2">
          <div className="min-w-0">
            {showLanguageSelector ? (
              <Select value={language} onValueChange={handleLangChange}>
                <SelectTrigger className="w-40">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {allowLanguages.includes('js') && <SelectItem value="js">JavaScript</SelectItem>}
                  {allowLanguages.includes('python3') && (
                    <SelectItem value="python3">Python 3</SelectItem>
                  )}
                  {allowLanguages.includes('json') && <SelectItem value="json">JSON</SelectItem>}
                </SelectContent>
              </Select>
            ) : null}
          </div>
          {showCopyButton ? (
            <Button variant="ghost" isIcon onClick={handleCopy} aria-label="Copy code">
              {copied ? <Check className="w-4 h-4" /> : <Copy className="w-4 h-4" />}
            </Button>
          ) : null}
        </div>
      )}

      <div className="rounded-md border overflow-hidden">
        <MonacoEditor
          value={value}
          language={monacoLanguage}
          onChange={handleChange}
          options={{
            minimap: { enabled: false },
            wordWrap: 'on',
            scrollBeyondLastLine: false,
            automaticLayout: true,
            readOnly,
            tabSize: 2,
            contextmenu: false,
            // Suggestion controls
            quickSuggestions: props.disableSuggest ? false : true,
            parameterHints: { enabled: props.disableSuggest ? false : true },
            suggestOnTriggerCharacters: props.disableSuggest ? false : true,
            acceptSuggestionOnEnter: props.disableSuggest ? 'off' : 'on',
            inlineSuggest: { enabled: props.disableSuggest ? false : true },
          }}
          height={editorHeight}
        />
      </div>

      {resizable ? (
        <div
          role="separator"
          aria-label="Resize code editor"
          aria-orientation="horizontal"
          className={cn(
            'h-2 w-full cursor-row-resize select-none rounded-sm bg-muted hover:bg-muted/80 active:bg-muted/60'
          )}
          onMouseDown={onMouseDownResize}
        />
      ) : null}

      {name ? (
        <>
          <input type="hidden" name={name} value={value} />
          <input type="hidden" name={`${name}__language`} value={language} />
        </>
      ) : null}
    </div>
  );
}

export default CodeEditor;
