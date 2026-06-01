import type { WorkflowData } from './type';

/**
 * Initial workflow data structure with empty graph and default features
 * This is the baseline state for new workflows
 */
export const initialWorkflowData: WorkflowData = {
  graph: {
    nodes: [],
    edges: [],
    viewport: { x: 0, y: 0, zoom: 1 },
  },
  features: {
    opening_statement_type: 'slogan',
    opening_guide_version: 2,
    opening_slogan: '',
    opening_statement: '',
    opening_statement_enabled: false,
    suggested_questions: [],
    suggested_questions_after_answer: { enabled: false },
    text_to_speech: { enabled: false, voice: '', language: '' },
    speech_to_text: { enabled: false },
    retriever_resource: { enabled: true },
    sensitive_word_avoidance: { enabled: false },
    conversation_history: { enabled: false, history_window_size: 3 },
    file_upload: {
      enabled: false,
      allowed_file_types: ['image'],
      allowed_file_extensions: ['jpg', 'jpeg', 'png', 'gif', 'webp', 'svg'],
      allowed_file_upload_methods: ['local_file', 'remote_url'],
      number_limits: 3,
    },
    webapp_workflow_config: {
      allow_view_run_detail: true,
      auto_expand_run_detail: false,
    },
  },
  environment_variables: [],
  conversation_variables: [],
  hash: '',
};
