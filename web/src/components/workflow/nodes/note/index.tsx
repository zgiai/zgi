import React, { memo, useState, useCallback, useEffect } from 'react';
import { cn } from '@/lib/utils';
import { useWorkflowStore } from '../../store/store';
import ManualResizeHandle from '../custom/node-resize-handle';
import type { NodePropsCompat } from '../../store/type';
import { useT } from '@/i18n/translations';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';

export interface NoteNodeData {
  type: 'note';
  text: string;
  theme?: 'yellow' | 'blue' | 'red' | 'purple' | 'green';
  title?: string;
  desc?: string;
  isInIteration?: boolean;
  isInLoop?: boolean;
}

const NoteNode: React.FC<NodePropsCompat<NoteNodeData>> = ({ id, data, selected }) => {
  const t = useT('nodes');
  const updateNodeData = useWorkflowStore.use.updateNodeData();
  const mode = useWorkflowStore.use.mode();
  const canEdit = useWorkflowStore.use.canEdit();
  const isReadOnly = mode === 'history' || !canEdit;
  const [text, setText] = useState(data.text || '');

  // Sync local state with data prop
  useEffect(() => {
    setText(data.text || '');
  }, [data.text]);

  // Auto-resize textarea logic
  const textareaRef = React.useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto';
      textareaRef.current.style.height = `${textareaRef.current.scrollHeight}px`;
    }
  }, [text]);

  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLTextAreaElement>) => {
      if (isReadOnly) return;
      setText(e.target.value);
    },
    [isReadOnly]
  );

  const handleBlur = useCallback(() => {
    if (isReadOnly) return;
    if (text !== data.text) {
      updateNodeData(id as string, { text });
    }
  }, [id, isReadOnly, text, data.text, updateNodeData]);

  const handleThemeChange = useCallback(
    (theme: NoteNodeData['theme']) => {
      if (isReadOnly) return;
      updateNodeData(id as string, { theme });
    },
    [id, isReadOnly, updateNodeData]
  );

  const themeClasses: Record<string, string> = {
    yellow: 'bg-yellow-50 dark:bg-yellow-900/10 border-yellow-200 dark:border-yellow-800/30',
    blue: 'bg-blue-50 dark:bg-blue-900/10 border-blue-200 dark:border-blue-800/30',
    red: 'bg-red-50 dark:bg-red-900/10 border-red-200 dark:border-red-800/30',
    purple: 'bg-purple-50 dark:bg-purple-900/10 border-purple-200 dark:border-purple-800/30',
    green: 'bg-green-50 dark:bg-green-900/10 border-green-200 dark:border-green-800/30',
  };

  const headerClasses: Record<string, string> = {
    yellow: 'bg-yellow-100 dark:bg-yellow-800/20 border-b border-yellow-200 dark:border-yellow-800',
    blue: 'bg-blue-100 dark:bg-blue-800/20 border-b border-blue-200 dark:border-blue-800',
    red: 'bg-red-100 dark:bg-red-800/20 border-b border-red-200 dark:border-red-800',
    purple: 'bg-purple-100 dark:bg-purple-800/20 border-b border-purple-200 dark:border-purple-800',
    green: 'bg-green-100 dark:bg-green-800/20 border-b border-green-200 dark:border-green-800',
  };

  const themeColors: Record<string, string> = {
    yellow: 'bg-yellow-400',
    blue: 'bg-blue-400',
    red: 'bg-red-400',
    purple: 'bg-purple-400',
    green: 'bg-green-400',
  };

  const currentTheme = data.theme || 'yellow';

  return (
    <div
      className={cn(
        'relative h-full w-full rounded-xl shadow-sm border transition-all flex flex-col overflow-hidden group',
        themeClasses[currentTheme] || themeClasses.yellow,
        selected ? 'ring-2 ring-blue-500/40 shadow-md' : 'hover:shadow-md'
      )}
    >
      <div
        className={cn(
          'h-4 w-full shrink-0 transition-colors cursor-grab active:cursor-grabbing',
          headerClasses[currentTheme] || headerClasses.yellow
        )}
      />

      <div className="flex-1 w-full h-full px-4 pb-4 pt-2 overflow-hidden flex flex-col cursor-grab active:cursor-grabbing">
        <textarea
          ref={textareaRef}
          className="nodrag w-full resize-none bg-transparent border-none focus:outline-none text-sm text-foreground/90 font-medium leading-relaxed scrollbar-hidden shrink-0 cursor-text overflow-y-auto"
          value={text}
          onChange={handleChange}
          onBlur={handleBlur}
          placeholder={t('catalog.note.placeholder')}
          onKeyDown={e => e.stopPropagation()}
          onPointerDown={e => e.stopPropagation()}
          disabled={isReadOnly}
          rows={1}
          style={{ height: 'auto', minHeight: '24px', maxHeight: '100%' }}
        />
        {/* Remaining space is draggable because the parent container handles drag */}
      </div>

      {/* Controls Overlay - Only visible on hover or select */}
      {!isReadOnly && (
        <div className="absolute top-1 right-1 flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity z-10">
          {/* Color Picker */}
          <Popover>
            <PopoverTrigger asChild>
              <div className="w-5 h-5 rounded-full bg-black/5 dark:bg-white/10 hover:bg-black/10 dark:hover:bg-white/20 cursor-pointer flex items-center justify-center backdrop-blur-sm border border-black/5">
                <div className={cn('w-2.5 h-2.5 rounded-full', themeColors[currentTheme])} />
              </div>
            </PopoverTrigger>
            <PopoverContent className="w-auto p-2" side="top" align="end">
              <div className="flex gap-2">
                {Object.keys(themeClasses).map(theme => (
                  <button
                    key={theme}
                    onClick={() => handleThemeChange(theme as NoteNodeData['theme'])}
                    className={cn(
                      'w-5 h-5 rounded-full hover:scale-110 transition-transform border border-black/10',
                      themeColors[theme],
                      currentTheme === theme && 'ring-2 ring-offset-2 ring-black/20'
                    )}
                  />
                ))}
              </div>
            </PopoverContent>
          </Popover>

          {/* Drag Handle Removed - using whole node drag */}
        </div>
      )}

      {/* Custom Resize Handle */}
      {selected && !isReadOnly && <ManualResizeHandle minWidth={160} minHeight={100} />}
    </div>
  );
};

export default memo(NoteNode);
