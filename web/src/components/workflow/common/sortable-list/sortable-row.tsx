import React from 'react';
import { GripVertical } from 'lucide-react';
import { useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';

export const SortableRow: React.FC<{
  id: string;
  children: React.ReactNode;
  disabled?: boolean;
}> = ({ id, children, disabled = false }) => {
  const { attributes, listeners, setNodeRef, transform, transition } = useSortable({
    id,
    disabled,
  });
  const style: React.CSSProperties = { transform: CSS.Transform.toString(transform), transition };
  return (
    <div ref={setNodeRef} style={style}>
      <div className="flex items-center gap-2">
        <span
          {...attributes}
          {...listeners}
          className={
            disabled ? 'text-muted-foreground opacity-50' : 'text-muted-foreground cursor-move'
          }
        >
          <GripVertical className="w-4 h-4" />
        </span>
        <div className="grow w-0 min-w-0">{children}</div>
      </div>
    </div>
  );
};

export default SortableRow;
