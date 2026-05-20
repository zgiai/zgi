import React from 'react';
import type { OnConnectEnd } from '@xyflow/react';
import { useCreateNodeModal } from './use-create-node-modal';
import { useWorkflowStore } from '../store';

interface ConnectEndClientPoint {
  x: number;
  y: number;
}

interface UseConnectDropCreateParams {
  isReadOnly: boolean;
  beginConnection: () => void;
  finishConnection: () => void;
}

const CANVAS_CONNECT_DROP_SELECTOR =
  '.react-flow__pane, .react-flow__background, .react-flow__connectionline';

function getConnectEndClientPoint(event: MouseEvent | TouchEvent): ConnectEndClientPoint | null {
  if ('changedTouches' in event) {
    const touch = event.changedTouches[0] ?? event.touches[0];
    return touch ? { x: touch.clientX, y: touch.clientY } : null;
  }

  return { x: event.clientX, y: event.clientY };
}

function isConnectEndOnEmptyCanvas(point: ConnectEndClientPoint): boolean {
  const target = document.elementFromPoint(point.x, point.y);
  if (!(target instanceof Element)) return false;

  const flowRoot = target.closest('.react-flow');
  if (!(flowRoot instanceof HTMLElement)) return false;

  const rootRect = flowRoot.getBoundingClientRect();
  const isInsideFlow =
    point.x >= rootRect.left &&
    point.x <= rootRect.right &&
    point.y >= rootRect.top &&
    point.y <= rootRect.bottom;
  if (!isInsideFlow) return false;

  if (target.closest('.react-flow__node, .react-flow__handle, .react-flow__panel')) {
    return false;
  }

  return Boolean(target.closest(CANVAS_CONNECT_DROP_SELECTOR));
}

export function useConnectDropCreate({
  isReadOnly,
  beginConnection,
  finishConnection,
}: UseConnectDropCreateParams) {
  const openCreateNodeModal = useCreateNodeModal(state => state.openModal);

  const handleConnectEnd = React.useCallback<OnConnectEnd>(
    (event, connectionState) => {
      finishConnection();

      if (isReadOnly) return;

      const point = getConnectEndClientPoint(event);
      const fromHandle = connectionState.fromHandle;
      const droppedOnTarget = Boolean(connectionState.toHandle || connectionState.toNode);

      if (
        !point ||
        droppedOnTarget ||
        !fromHandle?.nodeId ||
        fromHandle.type !== 'source' ||
        !isConnectEndOnEmptyCanvas(point)
      ) {
        return;
      }

      openCreateNodeModal(
        point,
        {
          nodeId: fromHandle.nodeId,
          handleId: fromHandle.id ?? 'source',
          handleType: 'source',
        },
        point
      );
      useWorkflowStore.getState().clearDraggingNodePreview();
    },
    [finishConnection, isReadOnly, openCreateNodeModal]
  );

  return {
    handleConnectStart: beginConnection,
    handleConnectEnd,
  };
}
