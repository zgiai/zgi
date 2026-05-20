import type { NodeInfo } from '@/components/chat/types';
import { extractWorkflowRunError, normalizeWorkflowBillingCode } from '@/utils/workflow/billing';
import { extractLlmGatewayRequest } from '@/utils/workflow/run-events';

/**
 * Unwrap SSE payload to extract data field if present
 */
export function unwrap(payload: unknown): Record<string, unknown> {
  const obj = typeof payload === 'object' && payload ? (payload as Record<string, unknown>) : {};
  const data = 'data' in obj ? (obj['data'] as unknown) : undefined;
  return typeof data === 'object' && data ? (data as Record<string, unknown>) : obj;
}

/**
 * Map SSE node event to NodeInfo for UI display
 * Handles multiple field naming conventions from backend
 */
export function mapNode(node: unknown, finished: boolean): NodeInfo {
  const p = unwrap(node);

  const statusRaw = typeof p['status'] === 'string' ? (p['status'] as string) : undefined;
  let status: NodeInfo['status'];
  if (statusRaw === 'failed') {
    status = 'failed';
  } else if (statusRaw === 'paused') {
    status = 'paused';
  } else if (statusRaw === 'stopped') {
    status = 'stopped';
  } else if (
    statusRaw === 'success' ||
    statusRaw === 'succeeded' ||
    statusRaw === 'completed'
  ) {
    status = 'success';
  } else {
    status = finished ? 'success' : 'running';
  }

  const rawErr = p['error'];
  const parsedError = extractWorkflowRunError(rawErr);
  const error =
    parsedError?.message ??
    (normalizeWorkflowBillingCode(parsedError?.code)
      ? `Billing error (${normalizeWorkflowBillingCode(parsedError?.code)})`
      : undefined);

  let nodeId: string | undefined;
  if (typeof p['node_id'] === 'string') {
    nodeId = p['node_id'] as string;
  } else if (typeof p['node_id'] === 'number') {
    nodeId = String(p['node_id'] as number);
  } else if (typeof p['execution_id'] === 'string') {
    nodeId = p['execution_id'] as string;
  } else if (typeof p['execution_id'] === 'number') {
    nodeId = String(p['execution_id'] as number);
  } else if (typeof p['id'] === 'string') {
    nodeId = p['id'] as string;
  } else if (typeof p['id'] === 'number') {
    nodeId = String(p['id'] as number);
  } else {
    nodeId = undefined;
  }

  const rawType =
    typeof p['node_type'] === 'string'
      ? (p['node_type'] as string)
      : typeof p['type'] === 'string'
        ? (p['type'] as string)
        : undefined;
  const hyphen = rawType ? rawType.replace(/_/g, '-').toLowerCase() : undefined;
  let nodeType: NodeInfo['nodeType'] | undefined;
  if (hyphen) {
    switch (hyphen) {
      case 'database':
        nodeType = 'call-database';
        break;
      case 'if_else':
      case 'if-else':
        nodeType = 'if-else';
        break;
      case 'http':
      case 'http-request':
        nodeType = 'http-request';
        break;
      case 'assign':
      case 'assigner':
        nodeType = 'assigner';
        break;
      case 'iterationstart':
      case 'iteration-start':
        nodeType = 'iteration-start';
        break;
      default:
        nodeType = hyphen;
        break;
    }
  } else {
    nodeType = undefined;
  }

  let title: string;
  if (typeof p['title'] === 'string') {
    title = p['title'] as string;
  } else if (typeof p['node_title'] === 'string') {
    title = p['node_title'] as string;
  } else if (typeof p['name'] === 'string') {
    title = p['name'] as string;
  } else if (typeof p['label'] === 'string') {
    title = p['label'] as string;
  } else {
    title = nodeType ?? nodeId ?? '';
  }

  const elapsedTime =
    typeof p['elapsed_time'] === 'number' ? (p['elapsed_time'] as number) : undefined;
  const inputs = p['inputs'];
  const outputs = p['outputs'];
  const modelInput = extractLlmGatewayRequest(p);
  return {
    status,
    error,
    elapsedTime,
    nodeId,
    nodeType,
    title,
    data: {
      input: inputs,
      output: finished ? outputs : undefined,
      modelInput,
    },
  };
}
