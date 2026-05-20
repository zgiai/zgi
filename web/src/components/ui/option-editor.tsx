import React, { useCallback, useEffect, useMemo, useRef } from 'react';
import {
  DndContext,
  closestCenter,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core';
import {
  arrayMove,
  SortableContext,
  sortableKeyboardCoordinates,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable';
import { useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { GripVertical, Plus, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { cn } from '@/lib/utils';
import { generateClientId } from '@/utils/client-id';

// Generate a stable id for each option
const genId = (): string => {
  // This id is only used for React keys and DnD identifiers
  // It will never be sent to backend
  return generateClientId('option');
};

// Sortable item row - memoized to avoid unnecessary re-renders
const SortableItem: React.FC<{
  id: string;
  children: React.ReactNode;
  itemClassName?: string;
  handleClassName?: string;
}> = React.memo(({ id, children, itemClassName, handleClassName }) => {
  const { attributes, listeners, setNodeRef, transform, transition } = useSortable({ id });

  const style: React.CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition,
  };

  return (
    <div ref={setNodeRef} style={style} className={cn('flex items-center gap-2', itemClassName)}>
      <span {...attributes} {...listeners} className={cn('cursor-move', handleClassName)}>
        <GripVertical size={16} />
      </span>
      {children}
    </div>
  );
});
SortableItem.displayName = 'SortableItem';

interface OptionEditorProps {
  options: string[];
  onChange: (options: string[]) => void;
  addButtonPlacement?: 'bottom' | 'header';
  labels?: {
    title?: string;
    add?: string;
    placeholder?: (index: number) => string;
  };
  classNames?: {
    root?: string;
    label?: string;
    list?: string;
    item?: string;
    handle?: string;
    input?: string;
    removeButton?: string;
    addButton?: string;
  };
}

const OptionEditorComponent: React.FC<OptionEditorProps> = ({
  options,
  onChange,
  addButtonPlacement = 'bottom',
  labels,
  classNames,
}) => {
  // Keep a stable id list aligned with the options array
  const idsRef = useRef<string[]>([]);
  // Mark internal structural updates (add/remove/reorder) to avoid resetting ids from external updates
  const internalStructureUpdateRef = useRef(false);

  // Initialize or repair ids length only when length truly changes externally
  useEffect(() => {
    if (idsRef.current.length === options.length) return;
    if (internalStructureUpdateRef.current) {
      // Skip once for our own structural update
      internalStructureUpdateRef.current = false;
      return;
    }
    // External length change, rebuild ids
    idsRef.current = Array.from({ length: options.length }, () => genId());
  }, [options.length]);

  // Ensure ids exist on first mount
  useEffect(() => {
    if (idsRef.current.length === 0 && options.length > 0) {
      idsRef.current = Array.from({ length: options.length }, () => genId());
    }
  }, [options.length]);

  const sensors = useSensors(
    useSensor(PointerSensor),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    })
  );

  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      const { active, over } = event;
      if (!over || active.id === over.id) return;

      const oldIndex = idsRef.current.indexOf(String(active.id));
      const newIndex = idsRef.current.indexOf(String(over.id));
      if (oldIndex === -1 || newIndex === -1) return;

      const newOptions = arrayMove(options, oldIndex, newIndex);
      // Reorder ids in the same way to keep them stable
      idsRef.current = arrayMove(idsRef.current, oldIndex, newIndex);

      internalStructureUpdateRef.current = true;
      onChange(newOptions);
    },
    [options, onChange]
  );

  const handleOptionChange = useCallback(
    (index: number, value: string) => {
      const newOptions = [...options];
      newOptions[index] = value;
      onChange(newOptions);
    },
    [options, onChange]
  );

  const handleRemoveOption = useCallback(
    (index: number) => {
      const newOptions = [...options];
      newOptions.splice(index, 1);

      // Remove corresponding id to keep alignment
      idsRef.current.splice(index, 1);

      internalStructureUpdateRef.current = true;
      onChange(newOptions);
    },
    [options, onChange]
  );

  const handleAddOption = useCallback(() => {
    const newOptions = [...options, ''];
    // Append a new id for the new item
    idsRef.current.push(genId());

    internalStructureUpdateRef.current = true;
    onChange(newOptions);
  }, [options, onChange]);

  // Compute pair list for rendering to avoid repeatedly reading ref in map
  const items = useMemo(() => {
    // Ensure ids length at least equals options length in case of direct external set (rare)
    if (idsRef.current.length < options.length) {
      const missing = options.length - idsRef.current.length;
      for (let i = 0; i < missing; i += 1) idsRef.current.push(genId());
    }
    return options.map((value, idx) => ({ id: idsRef.current[idx], value, index: idx }));
  }, [options]);

  return (
    <div className={cn('space-y-2', classNames?.root)}>
      <div className="flex items-center justify-between gap-3">
        <Label className={classNames?.label}>{labels?.title ?? 'Options'}</Label>
        {addButtonPlacement === 'header' ? (
          <Button
            type="button"
            variant="outline"
            className={cn(
              'h-8 rounded-xl border-dashed px-3 text-sm font-medium',
              classNames?.addButton
            )}
            onClick={handleAddOption}
          >
            <Plus className="size-4" />
            {labels?.add ?? 'Add Option'}
          </Button>
        ) : null}
      </div>
      <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
        <SortableContext items={items.map(i => i.id)} strategy={verticalListSortingStrategy}>
          <div className={cn('space-y-2', classNames?.list)}>
            {items.map(({ id, value, index }) => (
              <SortableItem
                key={id}
                id={id}
                itemClassName={classNames?.item}
                handleClassName={classNames?.handle}
              >
                <Input
                  value={value}
                  onChange={e => handleOptionChange(index, e.target.value)}
                  placeholder={
                    labels?.placeholder ? labels.placeholder(index + 1) : `Option ${index + 1}`
                  }
                  className={cn('flex-grow', classNames?.input)}
                />
                <Button
                  variant="ghost"
                  isIcon
                  onClick={() => handleRemoveOption(index)}
                  className={classNames?.removeButton}
                >
                  <X className="h-4 w-4" />
                </Button>
              </SortableItem>
            ))}
          </div>
        </SortableContext>
      </DndContext>
      {addButtonPlacement === 'bottom' ? (
        <Button
          variant="outline"
          className={cn('mt-2', classNames?.addButton)}
          onClick={handleAddOption}
        >
          {labels?.add ?? 'Add Option'}
        </Button>
      ) : null}
    </div>
  );
};

// Memo to avoid unnecessary re-renders when parent re-renders without changing props
const OptionEditor = React.memo(OptionEditorComponent);
OptionEditor.displayName = 'OptionEditor';

export { OptionEditor };
