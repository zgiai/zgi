export const DATASET_NAME_MIN_LENGTH = 1;
export const DATASET_NAME_MAX_LENGTH = 40;

export const DATASET_NAME_VALIDATION_OPTIONS = {
  allowSpace: true,
  minLength: DATASET_NAME_MIN_LENGTH,
  maxLength: DATASET_NAME_MAX_LENGTH,
} as const;
