// Utility: Normalize WorkflowDraftData or WorkflowData into strict WorkflowData
// Keep pure and side-effect free for easy testing and tree-shaking

import type {
  WorkflowData,
  WorkflowDraftData,
  FileUploadMethod,
  FileUploadType,
  ConversationVariableDraftItem,
  ConversationVariable,
  WebAppWorkflowConfigFeature,
} from '../type';

// Type guard to loosely detect if data already matches WorkflowData shape
function isWorkflowDataLike(data: unknown): data is WorkflowData {
  const d = data as Partial<WorkflowData> | undefined;
  return !!d && Array.isArray(d.environment_variables) && Array.isArray(d.conversation_variables);
}

// Ensure conversation variables have a valid 'type'. If only 'value_type' exists, map it.
function coerceConversationVariables(list: unknown): ConversationVariable[] {
  type CVType = ConversationVariable['type'];
  const allow: CVType[] = [
    'string',
    'number',
    'boolean',
    'object',
    'array[string]',
    'array[number]',
    'array[boolean]',
    'array[object]',
  ];
  const toCvType = (v: unknown): CVType => (allow.includes(v as CVType) ? (v as CVType) : 'string');

  if (!Array.isArray(list)) return [];
  return (list as unknown[]).map((raw, idx) => {
    const item = raw as Partial<ConversationVariable> & { value_type?: unknown };
    const fromType = toCvType((item as { type?: unknown }).type);
    const fromValueType = toCvType(item.value_type);
    const finalType: CVType = (item as { type?: unknown }).type ? fromType : fromValueType;
    return {
      id: typeof item.id === 'string' ? item.id : `cv-${idx}`,
      name: typeof item.name === 'string' ? item.name : '',
      type: finalType,
      value: (item as { value?: unknown }).value,
      description: typeof item.description === 'string' ? item.description : undefined,
    };
  });
}

function coerceFeatures(raw: unknown): WorkflowData['features'] {
  const defaultTypes: FileUploadType[] = ['image'];
  const defaultMethods: FileUploadMethod[] = ['local_file', 'remote_url'];
  const defaultExt = ['jpg', 'jpeg', 'png', 'gif', 'webp', 'svg'];

  const obj =
    typeof raw === 'object' && raw
      ? (raw as Partial<WorkflowData['features']> & Record<string, unknown>)
      : {};
  const sqa = (obj as { suggested_questions_after_answer?: { enabled?: unknown } })
    .suggested_questions_after_answer;
  const tts = (
    obj as { text_to_speech?: { enabled?: unknown; voice?: unknown; language?: unknown } }
  ).text_to_speech;
  const stt = (obj as { speech_to_text?: { enabled?: unknown } }).speech_to_text;
  const rr = (obj as { retriever_resource?: { enabled?: unknown } }).retriever_resource;
  const swa = (obj as { sensitive_word_avoidance?: { enabled?: unknown } })
    .sensitive_word_avoidance;
  const ch = (
    obj as { conversation_history?: { enabled?: unknown; history_window_size?: unknown } }
  ).conversation_history;
  const fuRaw = (obj as { file_upload?: unknown }).file_upload as
    | Partial<WorkflowData['features']['file_upload']>
    | { image?: { enabled?: unknown; number_limits?: unknown; transfer_methods?: unknown } }
    | null
    | undefined;
  const webAppWorkflowConfigRaw = (
    obj as { webapp_workflow_config?: Partial<WebAppWorkflowConfigFeature> }
  ).webapp_workflow_config;

  const enabledFU = Boolean(
    typeof fuRaw === 'object' && fuRaw
      ? ((fuRaw as Record<string, unknown>)['enabled'] ??
          (fuRaw as { image?: { enabled?: unknown } }).image?.enabled)
      : false
  );
  const numberLimitsFU = (() => {
    if (typeof fuRaw === 'object' && fuRaw) {
      const n1 = (fuRaw as { number_limits?: unknown }).number_limits;
      const n2 = (fuRaw as { image?: { number_limits?: unknown } }).image?.number_limits;
      const n =
        typeof n1 === 'number' && Number.isFinite(n1)
          ? n1
          : typeof n2 === 'number' && Number.isFinite(n2)
            ? n2
            : undefined;
      return typeof n === 'number' && Number.isFinite(n) ? n : 3;
    }
    return 3;
  })();
  const methodsFU = (() => {
    if (typeof fuRaw === 'object' && fuRaw) {
      const m1 = (fuRaw as { allowed_file_upload_methods?: unknown }).allowed_file_upload_methods;
      const m2 = (fuRaw as { image?: { transfer_methods?: unknown } }).image?.transfer_methods;
      return Array.isArray(m1)
        ? (m1 as FileUploadMethod[])
        : Array.isArray(m2)
          ? (m2 as FileUploadMethod[])
          : defaultMethods;
    }
    return defaultMethods;
  })();
  const typesFU = (() => {
    if (typeof fuRaw === 'object' && fuRaw) {
      const t = (fuRaw as { allowed_file_types?: unknown }).allowed_file_types;
      return Array.isArray(t) ? (t as FileUploadType[]) : defaultTypes;
    }
    return defaultTypes;
  })();
  const extFU = (() => {
    if (typeof fuRaw === 'object' && fuRaw) {
      const e = (fuRaw as { allowed_file_extensions?: unknown }).allowed_file_extensions;
      return (Array.isArray(e) ? (e as string[]) : defaultExt).map(v =>
        v.toLowerCase().replace(/^\./, '')
      );
    }
    return defaultExt;
  })();

  return {
    opening_statement_type:
      obj.opening_statement_type === 'message'
        ? 'message'
        : obj.opening_statement_type === 'slogan'
          ? 'slogan'
          : typeof obj.opening_slogan === 'string' && obj.opening_slogan.trim().length > 0
            ? 'slogan'
            : typeof obj.opening_statement === 'string' && obj.opening_statement.trim().length > 0
              ? 'message'
              : 'slogan',
    opening_slogan: typeof obj.opening_slogan === 'string' ? obj.opening_slogan : '',
    opening_statement: typeof obj.opening_statement === 'string' ? obj.opening_statement : '',
    opening_statement_enabled:
      typeof obj.opening_statement_enabled === 'boolean'
        ? obj.opening_statement_enabled
        : typeof obj.opening_statement === 'string'
          ? obj.opening_statement.trim().length > 0
          : false,
    suggested_questions: Array.isArray(obj.suggested_questions)
      ? ((obj.suggested_questions as unknown[]).filter(x => typeof x === 'string') as string[])
      : [],
    suggested_questions_after_answer: {
      enabled: Boolean(sqa?.enabled ?? false),
    },
    text_to_speech: {
      enabled: Boolean(tts?.enabled ?? false),
      voice: typeof tts?.voice === 'string' ? (tts?.voice as string) : '',
      language: typeof tts?.language === 'string' ? (tts?.language as string) : '',
    },
    speech_to_text: {
      enabled: Boolean(stt?.enabled ?? false),
    },
    retriever_resource: {
      enabled: typeof rr?.enabled === 'boolean' ? (rr?.enabled as boolean) : true,
    },
    sensitive_word_avoidance: {
      enabled: Boolean(swa?.enabled ?? false),
    },
    conversation_history: {
      enabled: Boolean(ch?.enabled ?? false),
      history_window_size: (() => {
        const n = typeof ch?.history_window_size === 'number' ? ch?.history_window_size : 3;
        const v = Math.max(1, Math.min(50, Number.isFinite(n) ? (n as number) : 3));
        return v;
      })(),
    },
    file_upload: {
      enabled: enabledFU,
      allowed_file_types: typesFU,
      allowed_file_extensions: extFU,
      allowed_file_upload_methods: methodsFU,
      number_limits: numberLimitsFU,
    },
    webapp_workflow_config: {
      allow_view_run_detail: webAppWorkflowConfigRaw?.allow_view_run_detail ?? true,
      auto_expand_run_detail: Boolean(webAppWorkflowConfigRaw?.auto_expand_run_detail ?? false),
    },
  };
}

export function normalizeToWorkflowData(data: WorkflowData | WorkflowDraftData): WorkflowData {
  // If it's already in WorkflowData shape, return as-is
  if (isWorkflowDataLike(data)) {
    // Coerce conversation variables to ensure 'type' exists (fallback to 'value_type' when necessary)
    const wd = data as WorkflowData;
    return {
      ...wd,
      features: coerceFeatures((wd as Partial<WorkflowData>).features),
      conversation_variables: coerceConversationVariables(wd.conversation_variables),
    } as WorkflowData;
  }

  const draftData = data as WorkflowDraftData;

  // Map draft conversation variables to workflow shape
  const mapConversationVars = (list: unknown): ConversationVariable[] =>
    coerceConversationVariables(list as ConversationVariableDraftItem[]);

  return {
    graph: {
      nodes: draftData.graph?.nodes || [],
      edges: draftData.graph?.edges || [],
      viewport: draftData.graph?.viewport || { x: 0, y: 0, zoom: 1 },
    },
    features: {
      opening_statement_type:
        draftData.features?.opening_statement_type === 'message'
          ? 'message'
          : draftData.features?.opening_statement_type === 'slogan'
            ? 'slogan'
            : Boolean(draftData.features?.opening_slogan?.trim())
              ? 'slogan'
              : Boolean(draftData.features?.opening_statement?.trim())
                ? 'message'
                : 'slogan',
      opening_slogan: draftData.features?.opening_slogan || '',
      opening_statement: draftData.features?.opening_statement || '',
      opening_statement_enabled:
        typeof draftData.features?.opening_statement_enabled === 'boolean'
          ? draftData.features.opening_statement_enabled
          : Boolean(draftData.features?.opening_statement?.trim()),
      suggested_questions: draftData.features?.suggested_questions || [],
      suggested_questions_after_answer: {
        enabled: draftData.features?.suggested_questions_after_answer?.enabled || false,
      },
      text_to_speech: {
        enabled: draftData.features?.text_to_speech?.enabled || false,
        voice: '',
        language: '',
      },
      speech_to_text: {
        enabled: draftData.features?.speech_to_text?.enabled || false,
      },
      retriever_resource: {
        enabled: draftData.features?.retriever_resource?.enabled ?? true,
      },
      // Ensure required field exists when converting from draft
      sensitive_word_avoidance: {
        enabled: false,
      },
      conversation_history: {
        enabled: Boolean(draftData.features?.conversation_history?.enabled ?? false),
        history_window_size: (() => {
          const n = draftData.features?.conversation_history?.history_window_size;
          const v = typeof n === 'number' && Number.isFinite(n) ? (n as number) : 3;
          return Math.max(1, Math.min(50, v));
        })(),
      },
      file_upload: (() => {
        const draftUpload = draftData.features?.file_upload;
        const defaultTypes: FileUploadType[] = ['image'];
        const defaultMethods: FileUploadMethod[] = ['local_file', 'remote_url'];
        const defaultExt = ['jpg', 'jpeg', 'png', 'gif', 'webp', 'svg'];
        const enabled = Boolean(draftUpload?.enabled ?? draftUpload?.image?.enabled ?? false);
        const numberLimits =
          (typeof draftUpload?.number_limits === 'number' &&
          Number.isFinite(draftUpload.number_limits)
            ? draftUpload.number_limits
            : draftUpload?.image?.number_limits) ?? 3;
        const methods = (
          Array.isArray(draftUpload?.allowed_file_upload_methods)
            ? (draftUpload?.allowed_file_upload_methods as FileUploadMethod[])
            : Array.isArray(draftUpload?.image?.transfer_methods)
              ? (draftUpload?.image?.transfer_methods as FileUploadMethod[]) || defaultMethods
              : defaultMethods
        ) as FileUploadMethod[];
        const types = (
          Array.isArray(draftUpload?.allowed_file_types)
            ? (draftUpload?.allowed_file_types as FileUploadType[])
            : defaultTypes
        ) as FileUploadType[];
        const exts = (
          Array.isArray(draftUpload?.allowed_file_extensions)
            ? (draftUpload?.allowed_file_extensions as string[]) || defaultExt
            : defaultExt
        ).map(e => e.toLowerCase().replace(/^\./, ''));
        return {
          enabled,
          allowed_file_types: types,
          allowed_file_extensions: exts,
          allowed_file_upload_methods: methods,
          number_limits: numberLimits,
        };
      })(),
      webapp_workflow_config: {
        allow_view_run_detail:
          draftData.features?.webapp_workflow_config?.allow_view_run_detail ?? true,
        auto_expand_run_detail: Boolean(
          draftData.features?.webapp_workflow_config?.auto_expand_run_detail ?? false
        ),
      },
    },
    environment_variables: Array.isArray(draftData.environment_variables)
      ? draftData.environment_variables
      : [],
    conversation_variables: mapConversationVars(draftData.conversation_variables),
    hash: '',
  };
}
