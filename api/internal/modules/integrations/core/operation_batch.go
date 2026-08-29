package integrations

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const MaxOperationBatchItems = 50

type OperationBatchMetadata struct {
	BatchID           string
	ItemIDs           []string
	ItemCount         int
	FrozenItemsDigest string
}

// OperationBatchApprovalSummary contains only bounded provider argument data
// needed to understand the approval. It never contains batch IDs, item IDs or
// digests. The full item list remains sealed in the frozen invocation.
func OperationBatchApprovalSummary(items []map[string]interface{}, guard *SuccessDeduplicationDefinition) map[string]interface{} {
	summary := map[string]interface{}{"item_count": len(items)}
	if guard == nil || len(items) == 0 || len(guard.TargetArgumentPaths) == 0 {
		return summary
	}
	uniqueTargets := make(map[string]map[string]interface{})
	for _, item := range items {
		target := make(map[string]interface{}, len(guard.TargetArgumentPaths))
		for _, path := range guard.TargetArgumentPaths {
			value, _ := operationTargetValue(item, path)
			target[path] = value
		}
		encoded, err := json.Marshal(target)
		if err != nil {
			continue
		}
		uniqueTargets[string(encoded)] = target
	}
	summary["target_count"] = len(uniqueTargets)
	if len(uniqueTargets) == 1 {
		for _, target := range uniqueTargets {
			summary["target"] = target
		}
	}
	return summary
}

// EnsureOperationBatchMetadata replaces all caller-supplied batch metadata
// with a deterministic server-owned description of the frozen items. The
// shared governance layer subsequently signs the full enriched invocation, so
// changing an item, target, or count invalidates the approval.
func EnsureOperationBatchMetadata(arguments map[string]interface{}, messageID, connectionID, integrationID, actionID string) (OperationBatchMetadata, error) {
	items, batched, err := OperationBatchItems(arguments)
	if err != nil || !batched {
		delete(arguments, "operation_batch")
		return OperationBatchMetadata{}, err
	}
	if strings.TrimSpace(messageID) == "" || strings.TrimSpace(connectionID) == "" {
		return OperationBatchMetadata{}, invalidInput("batch execution requires message and connection context", nil)
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		return OperationBatchMetadata{}, invalidInput("batch items could not be frozen", err)
	}
	digest := sha256.Sum256(encoded)
	frozenDigest := hex.EncodeToString(digest[:])
	batchSeed, err := json.Marshal([]string{
		"integration-batch-v1", strings.TrimSpace(messageID), strings.TrimSpace(connectionID),
		strings.ToLower(strings.TrimSpace(integrationID)), strings.ToLower(strings.TrimSpace(actionID)), frozenDigest,
	})
	if err != nil {
		return OperationBatchMetadata{}, invalidInput("batch identity could not be created", err)
	}
	batchDigest := sha256.Sum256(batchSeed)
	metadata := OperationBatchMetadata{
		BatchID:           "batch-" + hex.EncodeToString(batchDigest[:12]),
		ItemIDs:           make([]string, len(items)),
		ItemCount:         len(items),
		FrozenItemsDigest: frozenDigest,
	}
	for index, item := range items {
		itemJSON, marshalErr := json.Marshal(item)
		if marshalErr != nil {
			return OperationBatchMetadata{}, invalidInput("batch item could not be frozen", marshalErr)
		}
		itemSeed := append([]byte(fmt.Sprintf("%s:%d:", metadata.BatchID, index+1)), itemJSON...)
		itemDigest := sha256.Sum256(itemSeed)
		metadata.ItemIDs[index] = fmt.Sprintf("item-%03d-%s", index+1, hex.EncodeToString(itemDigest[:8]))
	}
	publicItemIDs := make([]interface{}, len(metadata.ItemIDs))
	for index := range metadata.ItemIDs {
		publicItemIDs[index] = metadata.ItemIDs[index]
	}
	arguments["operation_batch"] = map[string]interface{}{
		"batch_id":            metadata.BatchID,
		"operation_item_ids":  publicItemIDs,
		"item_count":          metadata.ItemCount,
		"frozen_items_digest": metadata.FrozenItemsDigest,
	}
	return metadata, nil
}

func OperationBatchItems(arguments map[string]interface{}) ([]map[string]interface{}, bool, error) {
	raw, exists := arguments["batch_items"]
	if !exists {
		return nil, false, nil
	}
	values, ok := raw.([]interface{})
	if !ok {
		if typed, typedOK := raw.([]map[string]interface{}); typedOK {
			values = make([]interface{}, len(typed))
			for index := range typed {
				values[index] = typed[index]
			}
		} else {
			return nil, true, invalidInput("batch_items must be an array", nil)
		}
	}
	if len(values) < 2 || len(values) > MaxOperationBatchItems {
		return nil, true, invalidInput(fmt.Sprintf("batch_items must contain between 2 and %d items", MaxOperationBatchItems), nil)
	}
	items := make([]map[string]interface{}, len(values))
	for index, value := range values {
		item, ok := value.(map[string]interface{})
		if !ok || item == nil {
			return nil, true, invalidInput(fmt.Sprintf("batch_items[%d] must be an object", index), nil)
		}
		items[index] = cloneJSONMap(item)
	}
	return items, true, nil
}

func ReadOperationBatchMetadata(arguments map[string]interface{}) (OperationBatchMetadata, bool) {
	raw, ok := arguments["operation_batch"].(map[string]interface{})
	if !ok || raw == nil {
		return OperationBatchMetadata{}, false
	}
	batchID, _ := raw["batch_id"].(string)
	digest, _ := raw["frozen_items_digest"].(string)
	itemCount, _ := raw["item_count"].(int)
	if itemCount == 0 {
		if number, ok := raw["item_count"].(float64); ok {
			itemCount = int(number)
		}
	}
	itemIDs := make([]string, 0, itemCount)
	switch values := raw["operation_item_ids"].(type) {
	case []string:
		itemIDs = append(itemIDs, values...)
	case []interface{}:
		for _, value := range values {
			if itemID, ok := value.(string); ok {
				itemIDs = append(itemIDs, itemID)
			}
		}
	}
	metadata := OperationBatchMetadata{
		BatchID: strings.TrimSpace(batchID), ItemIDs: itemIDs, ItemCount: itemCount, FrozenItemsDigest: strings.TrimSpace(digest),
	}
	return metadata, metadata.BatchID != "" && metadata.FrozenItemsDigest != "" && metadata.ItemCount == len(metadata.ItemIDs)
}
