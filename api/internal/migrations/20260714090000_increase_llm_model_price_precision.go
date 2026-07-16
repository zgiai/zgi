package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationIncreaseLLMModelPricePrecisionID = "20260714090000_increase_llm_model_price_precision"

func init() {
	registerSchemaMigration(
		migrationIncreaseLLMModelPricePrecisionID,
		upIncreaseLLMModelPricePrecision,
		nil,
	)
}

func upIncreaseLLMModelPricePrecision(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.llm_models
			ALTER COLUMN input_price TYPE numeric(10,6) USING input_price::numeric(10,6),
			ALTER COLUMN output_price TYPE numeric(10,6) USING output_price::numeric(10,6),
			ALTER COLUMN cached_input_price TYPE numeric(10,6) USING cached_input_price::numeric(10,6)
	`)
}
