# Models API Documentation

## Base URL
```
http://localhost:7001/v1/models
```

## Authentication
All endpoints require authentication. Add your API key to the request headers:
```
Authorization: Bearer YOUR_API_KEY
```

## Endpoints

### List Models
Get a list of all models with optional filtering.

**GET** `/v1/models`

#### Query Parameters
- `skip` (integer, optional): Number of records to skip. Default: 0
- `limit` (integer, optional): Maximum number of records to return. Default: 100, Max: 100
- `provider_id` (integer, optional): Filter by provider ID
- `type` (string, optional): Filter by model type (e.g., "LLM", "VISION")
- `supports_streaming` (boolean, optional): Filter by streaming support
- `supports_function_calling` (boolean, optional): Filter by function calling support
- `supports_roles` (boolean, optional): Filter by role support
- `fine_tuning_available` (boolean, optional): Filter by fine-tuning availability
- `multi_lang_support` (boolean, optional): Filter by multi-language support
- `max_price_per_1k_tokens` (decimal, optional): Filter by maximum price per 1k tokens
- `min_context_length` (integer, optional): Filter by minimum context length
- `tags` (array of strings, optional): Filter by tags
- `search` (string, optional): Search in model name and description

#### Response
```json
[
  {
    "id": 4,
    "provider_id": 192,
    "model_name": "GPT-3.5 Turbo",
    "model_version": "0.1",
    "description": "Fast and efficient language model",
    "type": "LLM",
    "modalities": {
      "text": true
    },
    "max_context_length": 4096,
    "supports_streaming": true,
    "supports_function_calling": true,
    "supports_roles": false,
    "supports_functions": false,
    "price_per_1k_tokens": "0.002",
    "api_call_name": "gpt-3.5-turbo",
    "created_at": "2024-12-13T22:52:00",
    "updated_at": "2024-12-13T22:52:00",
    "deleted_at": null
  }
]
```

### Get Model
Get a specific model by ID.

**GET** `/v1/models/{model_id}`

#### Path Parameters
- `model_id` (integer, required): The ID of the model to retrieve

#### Response
```json
{
  "id": 4,
  "provider_id": 192,
  "model_name": "GPT-3.5 Turbo",
  "model_version": "0.1",
  "description": "Fast and efficient language model",
  "type": "LLM",
  "modalities": {
    "text": true
  },
  "max_context_length": 4096,
  "supports_streaming": true,
  "supports_function_calling": true,
  "supports_roles": false,
  "supports_functions": false,
  "price_per_1k_tokens": "0.002",
  "api_call_name": "gpt-3.5-turbo",
  "created_at": "2024-12-13T22:52:00",
  "updated_at": "2024-12-13T22:52:00",
  "deleted_at": null
}
```

### Create Model
Create a new model.

**POST** `/v1/models`

#### Request Body
```json
{
  "provider_id": 192,
  "model_name": "GPT-3.5 Turbo",
  "model_version": "0.1",
  "description": "Fast and efficient language model",
  "type": "LLM",
  "modalities": {
    "text": true
  },
  "max_context_length": 4096,
  "supports_streaming": true,
  "supports_function_calling": true,
  "price_per_1k_tokens": 0.002,
  "api_call_name": "gpt-3.5-turbo"
}
```

#### Required Fields
- `provider_id` (integer): Foreign key to model_providers.id
- `model_name` (string): Model name
- `type` (string): Model type (e.g., "LLM", "VISION")

#### Optional Fields
- `model_version` (string): Model version
- `description` (string): Brief model description
- `modalities` (object): Supported modalities (input/output)
- `max_context_length` (integer): Maximum context length in tokens
- `supports_streaming` (boolean): If streaming is supported
- `supports_function_calling` (boolean): If function calling is supported
- `supports_roles` (boolean): If multi-role messages supported
- `supports_functions` (boolean): If tool functions are supported
- `embedding_dimensions` (integer): Vector dimensions if embedding model
- `default_temperature` (decimal): Default temperature parameter
- `default_top_p` (decimal): Default top_p parameter
- `default_max_tokens` (integer): Default max_tokens setting
- `price_per_1k_tokens` (decimal): Price per 1000 tokens
- `input_cost_per_1k_tokens` (decimal): Input cost per 1000 tokens
- `output_cost_per_1k_tokens` (decimal): Output cost per 1000 tokens
- `api_call_name` (string): API call endpoint name
- `latency_ms_estimate` (integer): Estimated latency in ms
- `rate_limit_per_minute` (integer): Rate limit per minute
- `fine_tuning_available` (boolean): If fine-tuning is available
- `multi_lang_support` (boolean): If multiple languages are supported
- `release_date` (string): Release date (YYYY-MM-DD)
- `developer_name` (string): Developer/company name
- `developer_website` (string): Developer website URL
- `training_data_sources` (string): Training data sources description
- `parameters_count` (string): Number of parameters
- `model_architecture` (string): Model architecture description
- `documentation_url` (string): Documentation URL
- `demo_url` (string): Demo URL
- `tags` (array of strings): Model tags

### Update Model
Update an existing model.

**PUT** `/v1/models/{model_id}`

#### Path Parameters
- `model_id` (integer, required): The ID of the model to update

#### Request Body
All fields are optional. Only include the fields you want to update.
```json
{
  "description": "Updated description",
  "supports_roles": true
}
```

### Delete Model
Soft delete a model.

**DELETE** `/v1/models/{model_id}`

#### Path Parameters
- `model_id` (integer, required): The ID of the model to delete

#### Response
```json
true
```

## Error Responses

### 404 Not Found
```json
{
  "detail": "Model not found"
}
```

### 500 Internal Server Error
```json
{
  "detail": {
    "message": "Internal server error",
    "error_type": "SQLAlchemyError",
    "error_details": "Database operation failed"
  }
}
```
