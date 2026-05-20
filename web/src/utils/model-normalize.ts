import type { ModelItem, ModelParameters } from '@/services/types/model';

/**
 * Provides default values for ModelParameters
 */
export const getDefaultParameters = (): ModelParameters => ({
  supports_temperature: false,
  supports_top_p: false,
  supports_presence_penalty: false,
  supports_frequency_penalty: false,
  supports_logit_bias: false,
  supports_seed: false,
  supports_stop: false,
  max_stop_sequences: 0,
});

/**
 * Normalizes a model object by ensuring all nested property objects exist.
 * This is defensive against partial or legacy API responses.
 */
export function normalizeModel(model: ModelItem): ModelItem {
  if (!model) return model;

  return {
    ...model,
    endpoints: model.endpoints || {},
    features: model.features || {},
    tools: model.tools || {},
    parameters: {
      ...getDefaultParameters(),
      ...(model.parameters || {}),
    },
    use_cases: model.use_cases || [],
    input_modalities: model.input_modalities || [],
    output_modalities: model.output_modalities || [],
  };
}
