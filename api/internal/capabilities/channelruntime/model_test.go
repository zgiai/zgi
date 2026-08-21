package channelruntime

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

func TestDeliveryColumnMappingsMatchMigration(t *testing.T) {
	parsed, err := schema.Parse(&Delivery{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("schema.Parse() error = %v", err)
	}

	for fieldName, wantColumn := range map[string]string{
		"Kind":        "delivery_kind",
		"DeliveryKey": "delivery_key",
		"Sequence":    "sequence",
	} {
		field := parsed.LookUpField(fieldName)
		if field == nil {
			t.Fatalf("field %q was not found", fieldName)
		}
		if field.DBName != wantColumn {
			t.Fatalf("field %q DBName = %q, want %q", fieldName, field.DBName, wantColumn)
		}
	}
}
