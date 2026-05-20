package vectordb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/zgiai/ginext/config"
	"github.com/zgiai/ginext/internal/observability"
	"github.com/zgiai/ginext/pkg/logger"
)

// WeaviateClient represents a Weaviate vector database client
type WeaviateClient struct {
	endpoint   string
	apiKey     string
	httpClient *http.Client
	batchSize  int
}

// NewWeaviateClient creates a new Weaviate client using configuration
func NewWeaviateClient(cfg *config.VectorStoreConfig) *WeaviateClient {
	return &WeaviateClient{
		endpoint: cfg.WeaviateEndpoint,
		apiKey:   cfg.WeaviateAPIKey,
		httpClient: observability.HTTPClient(&http.Client{
			Timeout: 30 * time.Second,
		}),
		batchSize: cfg.IndexingBatchSize,
	}
}

// StoreVector stores a vector with metadata in Weaviate
func (c *WeaviateClient) StoreVector(ctx context.Context, id, className string, properties map[string]interface{}, vector []float64) error {
	if c.endpoint == "" {
		return fmt.Errorf("weaviate endpoint not configured")
	}

	object := VectorObject{
		ID:         id,
		Class:      className,
		Properties: properties,
		Vector:     vector,
	}

	jsonData, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal vector object: %w", err)
	}

	url := fmt.Sprintf("%s/v1/objects", c.endpoint)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to store vector in weaviate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		// Read response body for detailed error information
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("weaviate returned status code: %d, response: %s", resp.StatusCode, string(body))
	}

	logger.Info("Vector stored in Weaviate", map[string]interface{}{
		"id":          id,
		"class":       className,
		"vector_size": len(vector),
		"status_code": resp.StatusCode,
	})

	return nil
}

// StoreVectors stores vectors with metadata in Weaviate using the batch objects API.
func (c *WeaviateClient) StoreVectors(ctx context.Context, objects []VectorObject) error {
	if c.endpoint == "" {
		return fmt.Errorf("weaviate endpoint not configured")
	}
	if len(objects) == 0 {
		return nil
	}

	batchSize := c.batchSize
	if batchSize <= 0 {
		batchSize = 100
	}

	for start := 0; start < len(objects); start += batchSize {
		end := start + batchSize
		if end > len(objects) {
			end = len(objects)
		}
		if err := c.storeVectorBatch(ctx, objects[start:end]); err != nil {
			return err
		}
	}

	return nil
}

func (c *WeaviateClient) storeVectorBatch(ctx context.Context, objects []VectorObject) error {
	payload := map[string]interface{}{
		"objects": objects,
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal vector batch: %w", err)
	}

	url := fmt.Sprintf("%s/v1/batch/objects", c.endpoint)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create batch request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to store vector batch in weaviate: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("weaviate batch returned status code: %d, response: %s", resp.StatusCode, string(body))
	}

	if err := validateWeaviateBatchResponse(body, objects); err != nil {
		return err
	}

	logger.Info("Vector batch stored in Weaviate", map[string]interface{}{
		"count":       len(objects),
		"status_code": resp.StatusCode,
	})

	return nil
}

func validateWeaviateBatchResponse(body []byte, objects []VectorObject) error {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}

	var results []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Result struct {
			Errors struct {
				Error []struct {
					Message string `json:"message"`
				} `json:"error"`
			} `json:"errors"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &results); err != nil {
		return nil
	}

	batchErrors := make(map[string]error)
	for i, result := range results {
		objectID := result.ID
		if objectID == "" && i < len(objects) {
			objectID = objects[i].ID
		}

		var messages []string
		for _, batchErr := range result.Result.Errors.Error {
			if strings.TrimSpace(batchErr.Message) != "" {
				messages = append(messages, batchErr.Message)
			}
		}
		if len(messages) > 0 {
			batchErrors[objectID] = fmt.Errorf("%s", strings.Join(messages, "; "))
		}
	}
	if len(batchErrors) > 0 {
		return &BatchVectorError{Errors: batchErrors}
	}

	return nil
}

// SearchVectors performs similarity search in Weaviate
func (c *WeaviateClient) SearchVectors(ctx context.Context, className string, vector []float64, limit int) ([]map[string]interface{}, error) {
	return c.searchVectors(ctx, className, "", vector, limit, false)
}

// SearchVectorsWithQuestions performs similarity search in Weaviate, including both regular segments and questions
func (c *WeaviateClient) SearchVectorsWithQuestions(ctx context.Context, className, questionClassName string, vector []float64, limit int) ([]map[string]interface{}, error) {
	return c.searchVectors(ctx, className, questionClassName, vector, limit, true)
}

// searchVectors performs similarity search in Weaviate (internal method)
func (c *WeaviateClient) searchVectors(ctx context.Context, className, questionClassName string, vector []float64, limit int, includeQuestions bool) ([]map[string]interface{}, error) {
	if c.endpoint == "" {
		return nil, fmt.Errorf("weaviate endpoint not configured")
	}

	// Clean className for GraphQL - replace hyphens with underscores
	cleanClassName := strings.ReplaceAll(className, "-", "_")

	var cleanQuestionClassName string
	if includeQuestions {
		cleanQuestionClassName = strings.ReplaceAll(questionClassName, "-", "_")
	}

	// Try to find the actual class name in Weaviate
	actualClassName, err := c.findActualClassName(ctx, cleanClassName)
	if err != nil {
		logger.Warn("Failed to query Weaviate schema, using original class name", map[string]interface{}{
			"error":               err.Error(),
			"original_class_name": className,
		})
		actualClassName = cleanClassName
	}

	var actualQuestionClassName string
	if includeQuestions {
		actualQuestionClassName, err = c.findActualClassName(ctx, cleanQuestionClassName)
		if err != nil {
			logger.Warn("Failed to query Weaviate schema, using original question class name", map[string]interface{}{
				"error":               err.Error(),
				"original_class_name": questionClassName,
			})
			actualQuestionClassName = cleanQuestionClassName
		}
	}

	logger.Debug("Resolved class name for Weaviate", map[string]interface{}{
		"original_class_name": className,
		"cleaned_class_name":  cleanClassName,
		"actual_class_name":   actualClassName,
		"include_questions":   includeQuestions,
	})

	var searchQuery map[string]interface{}

	if includeQuestions {
		logger.Debug("Resolved class names for Weaviate", map[string]interface{}{
			"original_class_name":          className,
			"original_question_class_name": questionClassName,
			"cleaned_class_name":           cleanClassName,
			"cleaned_question_class_name":  cleanQuestionClassName,
			"actual_class_name":            actualClassName,
			"actual_question_class_name":   actualQuestionClassName,
		})

		// Build GraphQL query to search both classes
		// Query fields: doc_id, dataset_id, document_id, doc_hash, text
		searchQuery = map[string]interface{}{
			"query": fmt.Sprintf(`{
				Get {
					%s(
						nearVector: {
							vector: %v
						}
						limit: %d
					) {
						_additional {
							id
							distance
						}
						doc_id
						dataset_id
						document_id
						doc_hash
						text
					}
					%s(
						nearVector: {
							vector: %v
						}
						limit: %d
					) {
						_additional {
							id
							distance
						}
						doc_id
						dataset_id
						document_id
						doc_hash
						text
					}
				}
			}`, actualClassName, vector, limit, actualQuestionClassName, vector, limit),
		}
	} else {
		// Build GraphQL query
		// Query fields: doc_id, dataset_id, document_id, doc_hash, text
		searchQuery = map[string]interface{}{
			"query": fmt.Sprintf(`{
				Get {
					%s(
						nearVector: {
							vector: %v
						}
						limit: %d
					) {
						_additional {
							id
							distance
						}
						doc_id
						dataset_id
						document_id
						doc_hash
						text
					}
				}
			}`, actualClassName, vector, limit),
		}
	}

	jsonData, err := json.Marshal(searchQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal search query: %w", err)
	}

	url := fmt.Sprintf("%s/v1/graphql", c.endpoint)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to search vectors in weaviate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weaviate search returned status code: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}

	// Debug: log the actual response
	logger.Debug("Weaviate raw response", map[string]interface{}{
		"response":          result,
		"class_name":        className,
		"include_questions": includeQuestions,
	})

	// Extract results from GraphQL response
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		// Log the actual response structure for debugging
		logger.Error("Weaviate response missing 'data' field", fmt.Errorf("response structure: %+v", result))
		return nil, fmt.Errorf("invalid response format: missing 'data' field")
	}

	get, ok := data["Get"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response format")
	}

	if includeQuestions {
		// Get results from both classes
		var allResults []map[string]interface{}

		// Get regular segment results
		if classResults, ok := get[actualClassName].([]interface{}); ok {
			for _, item := range classResults {
				if itemMap, ok := item.(map[string]interface{}); ok {
					allResults = append(allResults, itemMap)
				}
			}
		}

		// Get question results
		if questionClassResults, ok := get[actualQuestionClassName].([]interface{}); ok {
			for _, item := range questionClassResults {
				if itemMap, ok := item.(map[string]interface{}); ok {
					allResults = append(allResults, itemMap)
				}
			}
		}

		// Sort results by distance (ascending - smaller distance means more similar)
		sort.Slice(allResults, func(i, j int) bool {
			additionalI, okI := allResults[i]["_additional"].(map[string]interface{})
			additionalJ, okJ := allResults[j]["_additional"].(map[string]interface{})

			if !okI || !okJ {
				return false
			}

			distanceI, okI := additionalI["distance"].(float64)
			distanceJ, okJ := additionalJ["distance"].(float64)

			if !okI || !okJ {
				return false
			}

			return distanceI < distanceJ
		})

		// Limit results to requested limit
		finalResults := allResults
		if len(allResults) > limit {
			finalResults = allResults[:limit]
		}

		logger.Info("Vector search with questions completed", map[string]interface{}{
			"class":              className,
			"question_class":     questionClassName,
			"query_vector":       len(vector),
			"total_result_count": len(allResults),
			"returned_count":     len(finalResults),
		})

		return finalResults, nil
	} else {
		classResults, ok := get[actualClassName].([]interface{})
		if !ok {
			return []map[string]interface{}{}, nil // No results found
		}

		results := make([]map[string]interface{}, len(classResults))
		for i, item := range classResults {
			if itemMap, ok := item.(map[string]interface{}); ok {
				results[i] = itemMap
			}
		}

		logger.Info("Vector search completed", map[string]interface{}{
			"class":        className,
			"query_vector": len(vector),
			"result_count": len(results),
		})

		return results, nil
	}
}

// SearchByFullText performs BM25 full text search in Weaviate
func (c *WeaviateClient) SearchByFullText(ctx context.Context, className, query string, limit int) ([]map[string]interface{}, error) {
	if c.endpoint == "" {
		return nil, fmt.Errorf("weaviate endpoint not configured")
	}

	// Clean className for GraphQL - replace hyphens with underscores
	cleanClassName := strings.ReplaceAll(className, "-", "_")

	// Try to find the actual class name in Weaviate
	actualClassName, err := c.findActualClassName(ctx, cleanClassName)
	if err != nil {
		logger.Warn("Failed to query Weaviate schema, using original class name", map[string]interface{}{
			"error":               err.Error(),
			"original_class_name": className,
		})
		actualClassName = cleanClassName
	}

	logger.Debug("Resolved class name for Weaviate BM25 search", map[string]interface{}{
		"original_class_name": className,
		"cleaned_class_name":  cleanClassName,
		"actual_class_name":   actualClassName,
	})

	// Build GraphQL query for BM25 search
	// Query fields: doc_id, dataset_id, document_id, doc_hash, text
	searchQuery := map[string]interface{}{
		"query": fmt.Sprintf(`{
			Get {
				%s(
					bm25: {
						properties: ["text"]
						query: "%s"
					}
					limit: %d
				) {
					_additional {
						id
						vector
					}
					doc_id
					dataset_id
					document_id
					doc_hash
					text
				}
			}
		}`, actualClassName, query, limit),
	}

	jsonData, err := json.Marshal(searchQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal BM25 search query: %w", err)
	}

	url := fmt.Sprintf("%s/v1/graphql", c.endpoint)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to perform BM25 search in weaviate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weaviate BM25 search returned status code: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode BM25 search response: %w", err)
	}

	// Debug: log the actual response
	logger.Debug("Weaviate BM25 raw response", map[string]interface{}{
		"response":   result,
		"class_name": className,
		"query":      query,
	})

	// Extract results from GraphQL response
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		// Log the actual response structure for debugging
		logger.Error("Weaviate BM25 response missing 'data' field", fmt.Errorf("response structure: %+v", result))
		return nil, fmt.Errorf("invalid response format: missing 'data' field")
	}

	get, ok := data["Get"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response format")
	}

	classResults, ok := get[actualClassName].([]interface{})
	if !ok {
		return []map[string]interface{}{}, nil // No results found
	}

	results := make([]map[string]interface{}, len(classResults))
	for i, item := range classResults {
		if itemMap, ok := item.(map[string]interface{}); ok {
			results[i] = itemMap
		}
	}

	logger.Info("BM25 full text search completed", map[string]interface{}{
		"class":        className,
		"query":        query,
		"result_count": len(results),
	})

	return results, nil
}

// ListClasses retrieves all available classes from Weaviate schema
func (c *WeaviateClient) ListClasses(ctx context.Context) ([]string, error) {
	if c.endpoint == "" {
		return nil, fmt.Errorf("weaviate endpoint not configured")
	}

	url := fmt.Sprintf("%s/v1/schema", c.endpoint)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if c.apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	}

	client := observability.HTTPClient(&http.Client{Timeout: 30 * time.Second})
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get schema: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weaviate schema returned status code: %d", resp.StatusCode)
	}

	var schema map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&schema); err != nil {
		return nil, fmt.Errorf("failed to decode schema response: %w", err)
	}

	classes, ok := schema["classes"].([]interface{})
	if !ok {
		return []string{}, nil
	}

	var classNames []string
	for _, class := range classes {
		if classMap, ok := class.(map[string]interface{}); ok {
			if className, ok := classMap["class"].(string); ok {
				classNames = append(classNames, className)
			}
		}
	}

	return classNames, nil
}

// findActualClassName attempts to find the actual class name in Weaviate schema
func (c *WeaviateClient) findActualClassName(ctx context.Context, requestedClassName string) (string, error) {
	// Get all available classes
	classes, err := c.ListClasses(ctx)
	if err != nil {
		return requestedClassName, err
	}

	// Log available classes for debugging
	logger.Debug("Available Weaviate classes", map[string]interface{}{
		"classes":   classes,
		"requested": requestedClassName,
	})

	// First, try exact match
	for _, class := range classes {
		if class == requestedClassName {
			return class, nil
		}
	}

	// If we're looking for a Dataset_ class, try to find Vector_index_ variant with _Node suffix
	if strings.HasPrefix(requestedClassName, "Dataset_") {
		datasetID := strings.TrimPrefix(requestedClassName, "Dataset_")
		// Generate the correct pattern
		normalizedDatasetID := strings.ReplaceAll(datasetID, "-", "_")
		vectorIndexClassName := fmt.Sprintf("Vector_index_%s_Node", normalizedDatasetID)

		for _, class := range classes {
			if class == vectorIndexClassName {
				logger.Info("Found matching vector index class", map[string]interface{}{
					"requested": requestedClassName,
					"found":     class,
				})
				return class, nil
			}
		}
	}

	// If no match found, return the original name
	logger.Warn("No matching class found in Weaviate schema", map[string]interface{}{
		"requested":         requestedClassName,
		"available_classes": classes,
	})

	return requestedClassName, nil
}

// CreateClass creates a new class schema in Weaviate
func (c *WeaviateClient) CreateClass(ctx context.Context, className string, properties []map[string]interface{}) error {
	if c.endpoint == "" {
		return fmt.Errorf("weaviate endpoint not configured")
	}

	classSchema := map[string]interface{}{
		"class":       className,
		"description": fmt.Sprintf("Class for %s vectors", className),
		"properties":  properties,
		"vectorizer":  "none", // We provide our own vectors
	}

	jsonData, err := json.Marshal(classSchema)
	if err != nil {
		return fmt.Errorf("failed to marshal class schema: %w", err)
	}

	url := fmt.Sprintf("%s/v1/schema", c.endpoint)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create class in weaviate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		// Read response body for detailed error information
		body, _ := io.ReadAll(resp.Body)

		// Class might already exist, which is fine
		if resp.StatusCode == http.StatusUnprocessableEntity {
			logger.Info("Class creation failed (might already exist)", map[string]interface{}{
				"class":    className,
				"response": string(body),
			})
			return nil
		}
		return fmt.Errorf("weaviate returned status code: %d, response: %s", resp.StatusCode, string(body))
	}

	logger.Info("Class created in Weaviate", map[string]interface{}{
		"class":       className,
		"status_code": resp.StatusCode,
	})

	return nil
}

// HealthCheck checks if Weaviate is accessible
func (c *WeaviateClient) HealthCheck(ctx context.Context) error {
	if c.endpoint == "" {
		return fmt.Errorf("weaviate endpoint not configured")
	}

	url := fmt.Sprintf("%s/v1/.well-known/ready", c.endpoint)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if c.apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reach weaviate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("weaviate health check failed with status: %d", resp.StatusCode)
	}

	logger.Info("Weaviate health check passed", map[string]interface{}{
		"endpoint": c.endpoint,
	})

	return nil
}

func (c *WeaviateClient) DeleteClass(ctx context.Context, className string) error {
	if c.endpoint == "" {
		return fmt.Errorf("weaviate endpoint not configured")
	}

	// First check if class exists
	classes, err := c.ListClasses(ctx)
	if err != nil {
		return fmt.Errorf("failed to list classes: %w", err)
	}

	classExists := false
	for _, class := range classes {
		if class == className {
			classExists = true
			break
		}
	}

	// If class doesn't exist, return without error (idempotent)
	if !classExists {
		logger.Info("Class does not exist, skipping deletion", map[string]interface{}{
			"class": className,
		})
		return nil
	}

	url := fmt.Sprintf("%s/v1/schema/%s", c.endpoint, className)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if c.apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete class from weaviate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		// Read response body for detailed error information
		body, _ := io.ReadAll(resp.Body)
		// If class doesn't exist (404), consider it a success (idempotent)
		if resp.StatusCode == http.StatusNotFound {
			logger.Info("Class not found during deletion, treating as success", map[string]interface{}{
				"class":       className,
				"status_code": resp.StatusCode,
				"response":    string(body),
			})
			return nil
		}
		return fmt.Errorf("weaviate returned status code: %d, response: %s", resp.StatusCode, string(body))
	}

	logger.Info("Class deleted from Weaviate", map[string]interface{}{
		"class":       className,
		"status_code": resp.StatusCode,
	})

	return nil
}

// DeleteObjectsByField deletes objects from a class based on a field value
func (c *WeaviateClient) DeleteObjectsByField(ctx context.Context, className, fieldName, fieldValue string) error {
	if c.endpoint == "" {
		return fmt.Errorf("weaviate endpoint not configured")
	}

	// GraphQL mutation for batch deletion
	deleteMutation := map[string]interface{}{
		"query": fmt.Sprintf(`mutation {
			BatchDelete%s: DeleteObjects(
				input: {
					class: "%s"
					where: {
						operator: Equal
						path: ["%s"]
						valueText: "%s"
					}
				}
			) {
				matches {
					totalCount
				}
				output
			}
		}`, className, className, fieldName, fieldValue),
	}

	jsonData, err := json.Marshal(deleteMutation)
	if err != nil {
		return fmt.Errorf("failed to marshal delete mutation: %w", err)
	}

	url := fmt.Sprintf("%s/v1/graphql", c.endpoint)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete objects in weaviate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("weaviate delete returned status code: %d, response: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode delete response: %w", err)
	}

	logger.Info("Objects deleted by field", map[string]interface{}{
		"class":      className,
		"field_name": fieldName,
		"result":     result,
	})

	return nil
}

// DeleteObjectByID deletes a single object by its ID
func (c *WeaviateClient) DeleteObjectByID(ctx context.Context, className, objectID string) error {
	if c.endpoint == "" {
		return fmt.Errorf("weaviate endpoint not configured")
	}

	url := fmt.Sprintf("%s/v1/objects/%s/%s", c.endpoint, className, objectID)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if c.apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete object from weaviate: %w", err)
	}
	defer resp.Body.Close()

	// If object doesn't exist, return without error (idempotent)
	if resp.StatusCode == http.StatusNotFound {
		logger.Info("Object not found during deletion, treating as success", map[string]interface{}{
			"class":       className,
			"object_id":   objectID,
			"status_code": resp.StatusCode,
		})
		return nil
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		// Read response body for detailed error information
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("weaviate returned status code: %d, response: %s", resp.StatusCode, string(body))
	}

	logger.Info("Object deleted from Weaviate", map[string]interface{}{
		"class":       className,
		"object_id":   objectID,
		"status_code": resp.StatusCode,
	})

	return nil
}

// DeleteObjectsByIDs deletes multiple objects by their IDs
func (c *WeaviateClient) DeleteObjectsByIDs(ctx context.Context, className string, objectIDs []string) error {
	if c.endpoint == "" {
		return fmt.Errorf("weaviate endpoint not configured")
	}

	for _, objectID := range objectIDs {
		err := c.DeleteObjectByID(ctx, className, objectID)
		if err != nil {
			return fmt.Errorf("failed to delete object %s: %w", objectID, err)
		}
	}

	logger.Info("Objects deleted by IDs", map[string]interface{}{
		"class":        className,
		"object_count": len(objectIDs),
	})

	return nil
}
