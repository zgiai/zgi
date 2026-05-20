'use client';

import { create } from 'zustand';
import type { HandleType } from '@xyflow/react';

export interface OriginatingHandleInfo {
  nodeId: string;
  handleId: string;
  handleType: HandleType;
}

// Edge-origin context used when inserting a node into an existing edge
export interface OriginatingEdgeInfo {
  edgeId: string;
  sourceId: string;
  targetId: string;
  sourceHandle?: string;
  targetHandle?: string;
  midPoint: { x: number; y: number };
}

export interface CreateNodeModalState {
  open: boolean;
  position: { x: number; y: number; parentId?: string } | null;
  anchorClientPosition: { x: number; y: number } | null;
  originatingHandle: OriginatingHandleInfo | null;
  originatingEdge: OriginatingEdgeInfo | null;
  openModal: (
    position?: { x: number; y: number; parentId?: string },
    originatingHandle?: OriginatingHandleInfo | null,
    anchorClientPosition?: { x: number; y: number } | null
  ) => void;
  // Open modal specifically for edge insertion, passing edge info and midpoint position
  openEdgeInsertModal: (params: {
    position: { x: number; y: number };
    anchorClientPosition?: { x: number; y: number } | null;
    edge: OriginatingEdgeInfo;
  }) => void;
  closeModal: () => void;
  clearOriginatingHandle: () => void;
  clearOriginatingEdge: () => void;
}

export const useCreateNodeModal = create<CreateNodeModalState>(set => ({
  open: false,
  position: null,
  anchorClientPosition: null,
  originatingHandle: null,
  originatingEdge: null,
  openModal: (position, originatingHandle, anchorClientPosition) =>
    set({
      open: true,
      position: position ?? null,
      anchorClientPosition: anchorClientPosition ?? null,
      originatingHandle: originatingHandle ?? null,
      // keep any existing edge origin unless explicitly opening for handle
    }),
  openEdgeInsertModal: ({ position, anchorClientPosition, edge }) =>
    set({
      open: true,
      position,
      anchorClientPosition: anchorClientPosition ?? null,
      originatingEdge: edge,
      // ensure handle-origin is cleared when opening from edge
      originatingHandle: null,
    }),
  closeModal: () =>
    set({
      open: false,
      position: null,
      anchorClientPosition: null,
      // Keep origins until explicit clear to allow follow-up logic
    }),
  clearOriginatingHandle: () =>
    set({
      originatingHandle: null,
    }),
  clearOriginatingEdge: () =>
    set({
      originatingEdge: null,
    }),
}));
