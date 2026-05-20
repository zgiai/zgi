import type React from 'react';

export const DEFAULT_DRAG_PREVIEW_ANCHOR = { x: 24, y: 20 } as const;

let transparentDragImage: HTMLElement | null = null;

/**
 * @util setTransparentDragImage
 * @description Hides the browser's native drag ghost so the workflow preview owns the drag visual.
 */
export function setTransparentDragImage(dataTransfer: DataTransfer | null | undefined) {
  if (!dataTransfer || typeof document === 'undefined') return;

  if (!transparentDragImage) {
    transparentDragImage = document.createElement('div');
    transparentDragImage.style.position = 'fixed';
    transparentDragImage.style.top = '-1000px';
    transparentDragImage.style.left = '-1000px';
    transparentDragImage.style.width = '1px';
    transparentDragImage.style.height = '1px';
    transparentDragImage.style.opacity = '0';
    transparentDragImage.style.pointerEvents = 'none';
    document.body.appendChild(transparentDragImage);
  }

  dataTransfer.setDragImage(transparentDragImage, 0, 0);
}

export function getDragClientPosition(event: React.DragEvent): { x: number; y: number } | null {
  if (!event.clientX && !event.clientY) return null;
  return { x: event.clientX, y: event.clientY };
}
