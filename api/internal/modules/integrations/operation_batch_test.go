package integrations

import (
	"reflect"
	"testing"
)

func TestOperationBatchMetadataDistinguishesRepeatedItemsAndFreezesPlan(t *testing.T) {
	items := make([]interface{}, 10)
	for index := range items {
		items[index] = map[string]interface{}{
			"recipient_id": "recipient-a", "recipient_type": "open_id", "text": "message",
		}
	}
	arguments := map[string]interface{}{"batch_items": items}
	first, err := EnsureOperationBatchMetadata(arguments, testMessageID, testConnectionID, "feishu", "feishu.message.send_user")
	if err != nil {
		t.Fatalf("EnsureOperationBatchMetadata() error = %v", err)
	}
	if first.ItemCount != 10 || len(first.ItemIDs) != 10 {
		t.Fatalf("batch metadata = %#v", first)
	}
	seen := map[string]struct{}{}
	for _, itemID := range first.ItemIDs {
		if _, duplicate := seen[itemID]; duplicate {
			t.Fatalf("duplicate item ID %q in %#v", itemID, first.ItemIDs)
		}
		seen[itemID] = struct{}{}
	}

	repeatedArguments := map[string]interface{}{"batch_items": items}
	repeated, err := EnsureOperationBatchMetadata(repeatedArguments, testMessageID, testConnectionID, "feishu", "feishu.message.send_user")
	if err != nil || !reflect.DeepEqual(first, repeated) {
		t.Fatalf("same frozen plan metadata = %#v, error = %v; want %#v", repeated, err, first)
	}

	changedItems := append([]interface{}(nil), items...)
	changedItems[3] = map[string]interface{}{
		"recipient_id": "recipient-b", "recipient_type": "open_id", "text": "message",
	}
	changed, err := EnsureOperationBatchMetadata(map[string]interface{}{"batch_items": changedItems}, testMessageID, testConnectionID, "feishu", "feishu.message.send_user")
	if err != nil {
		t.Fatalf("changed EnsureOperationBatchMetadata() error = %v", err)
	}
	if changed.BatchID == first.BatchID || changed.FrozenItemsDigest == first.FrozenItemsDigest {
		t.Fatalf("changed target reused batch identity: first=%#v changed=%#v", first, changed)
	}
	changedContentItems := append([]interface{}(nil), items...)
	changedContentItems[3] = map[string]interface{}{
		"recipient_id": "recipient-a", "recipient_type": "open_id", "text": "changed message",
	}
	changedContent, err := EnsureOperationBatchMetadata(map[string]interface{}{"batch_items": changedContentItems}, testMessageID, testConnectionID, "feishu", "feishu.message.send_user")
	if err != nil {
		t.Fatalf("changed-content EnsureOperationBatchMetadata() error = %v", err)
	}
	if changedContent.BatchID == first.BatchID || changedContent.FrozenItemsDigest == first.FrozenItemsDigest {
		t.Fatalf("changed content reused batch identity: first=%#v changed=%#v", first, changedContent)
	}

	newMessage, err := EnsureOperationBatchMetadata(map[string]interface{}{"batch_items": items}, secondMessageID, testConnectionID, "feishu", "feishu.message.send_user")
	if err != nil {
		t.Fatalf("new-message EnsureOperationBatchMetadata() error = %v", err)
	}
	if newMessage.BatchID == first.BatchID || reflect.DeepEqual(newMessage.ItemIDs, first.ItemIDs) {
		t.Fatalf("new user message reused batch identity: first=%#v new=%#v", first, newMessage)
	}
}
