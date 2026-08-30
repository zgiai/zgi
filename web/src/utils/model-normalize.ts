import type { ModelItem, ModelParameters, ModelUseCase } from '@/services/types/model';

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

  const arrayOrEmpty = <T>(value: unknown): T[] => (Array.isArray(value) ? value : []);
  const objectOrEmpty = <T extends object>(value: unknown): T =>
    (value !== null && typeof value === 'object' && !Array.isArray(value) ? value : {}) as T;

  return {
    ...model,
    endpoints: objectOrEmpty<ModelItem['endpoints']>(model.endpoints),
    features: objectOrEmpty<ModelItem['features']>(model.features),
    tools: objectOrEmpty<ModelItem['tools']>(model.tools),
    parameters: {
      ...getDefaultParameters(),
      ...objectOrEmpty<NonNullable<ModelItem['parameters']>>(model.parameters),
    },
    use_cases: arrayOrEmpty<ModelUseCase>(model.use_cases),
    input_modalities: arrayOrEmpty<string>(model.input_modalities),
    output_modalities: arrayOrEmpty<string>(model.output_modalities),
  };
}

/** Normalizes an untrusted API list and drops entries that are not objects. */
export function normalizeModelList(value: unknown): ModelItem[] {
  if (!Array.isArray(value)) return [];
  return value
    .filter(
      (item): item is ModelItem => item !== null && typeof item === 'object' && !Array.isArray(item)
    )
    .map(normalizeModel);
}
