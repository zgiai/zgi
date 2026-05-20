import React from 'react';
import { Button } from '@/components/ui/button';
import SortableRow from './sortable-row';
import { DndContext, type DragEndEvent, type SensorDescriptor, closestCenter } from '@dnd-kit/core';
import { SortableContext, verticalListSortingStrategy } from '@dnd-kit/sortable';

export interface SortableListSectionProps {
  title: string;
  addLabel: string;
  emptyText: string;
  isReadOnly: boolean;
  items: Array<{ id: string; index: number }>;
  sensors: Array<SensorDescriptor<object>>;
  onDragEnd: (event: DragEndEvent) => void;
  onAdd: () => void;
  renderRow: (index: number) => React.ReactNode;
}

const SortableListSection: React.FC<SortableListSectionProps> = ({
  title,
  addLabel,
  emptyText,
  isReadOnly,
  items,
  sensors,
  onDragEnd,
  onAdd,
  renderRow,
}) => {
  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <div className="text-sm font-medium">{title}</div>
        <Button size="sm" onClick={onAdd} disabled={isReadOnly} aria-disabled={isReadOnly}>
          {addLabel}
        </Button>
      </div>
      {items.length === 0 ? (
        <p className="text-sm text-muted-foreground">{emptyText}</p>
      ) : (
        <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
          <SortableContext items={items.map(i => i.id)} strategy={verticalListSortingStrategy}>
            <div className="space-y-2">
              {items.map(({ id, index }) => (
                <SortableRow key={id} id={id} disabled={isReadOnly}>
                  {renderRow(index)}
                </SortableRow>
              ))}
            </div>
          </SortableContext>
        </DndContext>
      )}
    </div>
  );
};

export default React.memo(SortableListSection);
