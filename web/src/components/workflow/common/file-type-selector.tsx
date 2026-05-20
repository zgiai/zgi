import React, { useCallback, useMemo } from 'react';
import { Label } from '@/components/ui/label';
import { cn } from '@/lib/utils';
import { Check, Image, Music, Video, FileText, Settings2 } from 'lucide-react';
import { FILE_TYPE_EXTENSIONS } from '@/utils/file-helpers';
import { useT } from '@/i18n';

// File type options
const FILE_TYPES = ['document', 'image', 'audio', 'video', 'custom'] as const;
export type FileType = (typeof FILE_TYPES)[number];

// Icons for each file type
const FILE_TYPE_ICONS: Record<FileType, React.ElementType> = {
  image: Image,
  video: Video,
  audio: Music,
  document: FileText,
  custom: Settings2,
};

// Colors for each file type (used for the icon and border when selected)
const FILE_TYPE_COLORS: Record<FileType, { icon: string; border: string; bg: string }> = {
  image: {
    icon: 'text-purple-500',
    border: 'border-purple-500',
    bg: 'bg-purple-50 dark:bg-purple-950/30',
  },
  video: {
    icon: 'text-pink-500',
    border: 'border-pink-500',
    bg: 'bg-pink-50 dark:bg-pink-950/30',
  },
  audio: {
    icon: 'text-indigo-500',
    border: 'border-indigo-500',
    bg: 'bg-indigo-50 dark:bg-indigo-950/30',
  },
  document: {
    icon: 'text-blue-500',
    border: 'border-blue-500',
    bg: 'bg-blue-50 dark:bg-blue-950/30',
  },
  custom: {
    icon: 'text-orange-500',
    border: 'border-orange-500',
    bg: 'bg-orange-50 dark:bg-orange-950/30',
  },
};

interface FileTypeSelectorProps {
  selectedTypes: string[];
  onChange: (types: string[]) => void;
  label?: string;
}

/**
 * FileTypeSelector - A card-based file type selector component
 * Each card displays the type name, icon, and supported file extensions
 */
export function FileTypeSelector({ selectedTypes, onChange, label }: FileTypeSelectorProps) {
  const t = useT('nodes');

  // Check if custom is selected (exclusive mode)
  const isCustomSelected = selectedTypes.includes('custom');

  // Get extensions description for a file type
  const getExtensionsDescription = useCallback(
    (type: FileType): string => {
      if (type === 'custom') {
        return '';
      }
      const extensions = FILE_TYPE_EXTENSIONS[type];
      if (!extensions) return '';
      // Format as ".ext, .ext, ..."
      return extensions.map(ext => `${ext.toUpperCase()}`).join(', ');
    },
    [t]
  );

  // Handle type selection
  const handleTypeClick = useCallback(
    (type: FileType) => {
      let newTypes: string[];

      if (type === 'custom') {
        // Custom is exclusive - selecting it clears others
        if (isCustomSelected) {
          newTypes = [];
        } else {
          newTypes = ['custom'];
        }
      } else {
        // Regular types - can be multi-selected
        if (isCustomSelected) {
          // If custom was selected, switch to this type
          newTypes = [type];
        } else if (selectedTypes.includes(type)) {
          // Deselect
          newTypes = selectedTypes.filter(t => t !== type);
        } else {
          // Add to selection
          newTypes = [...selectedTypes, type];
        }
      }

      onChange(newTypes);
    },
    [selectedTypes, isCustomSelected, onChange]
  );

  // Memoize the file type cards
  const typeCards = useMemo(() => {
    return FILE_TYPES.map(type => {
      const isSelected = selectedTypes.includes(type);
      const Icon = FILE_TYPE_ICONS[type];
      const colors = FILE_TYPE_COLORS[type];

      return (
        <button
          key={type}
          type="button"
          onClick={() => handleTypeClick(type)}
          className={cn(
            'relative flex flex-col items-start gap-2 px-2 py-1 rounded-lg border transition-all text-left w-full',
            'hover:shadow-sm focus:outline-none',
            isSelected
              ? cn(colors.border, colors.bg)
              : 'border-muted-foreground/50 hover:border-muted-foreground'
          )}
        >
          {/* Selection indicator */}
          {isSelected && (
            <div
              className={cn(
                'absolute top-2 right-2 w-5 h-5 rounded-full flex items-center justify-center',
                colors.icon.replace('text-', 'bg-')
              )}
            >
              <Check size={12} className="text-white" />
            </div>
          )}

          {/* Icon and title */}
          <div className="flex items-center gap-2">
            <div
              className={cn(
                'w-7 h-7 rounded-md flex items-center justify-center shrink-0',
                colors.bg
              )}
            >
              <Icon size={18} className={colors.icon} />
            </div>
            <div>
              <span
                className={cn('font-medium text-xs leading-none', isSelected && 'text-foreground')}
              >
                {t(`start.modal.file.types.${type}`)}
              </span>
              {/* Extensions description */}
              {type! !== 'custom' && (
                <p className="text-[10px] text-muted-foreground line-clamp-2 leading-relaxed">
                  {getExtensionsDescription(type)}
                </p>
              )}
            </div>
          </div>
        </button>
      );
    });
  }, [selectedTypes, isCustomSelected, handleTypeClick, t, getExtensionsDescription]);

  return (
    <div className="space-y-3">
      {label && <Label>{label}</Label>}
      <div className="flex-col space-y-2">{typeCards}</div>
    </div>
  );
}

export default FileTypeSelector;
