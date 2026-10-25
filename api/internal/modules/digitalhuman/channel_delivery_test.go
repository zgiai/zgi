package digitalhuman

import (
	"errors"
	"testing"

	"gorm.io/gorm"
)

func TestChannelDeliveryLookupErrorSeparatesMissingTargetFromDatabaseFailure(t *testing.T) {
	missing := channelDeliveryLookupError("load channel installation for delivery", gorm.ErrRecordNotFound)
	if !errors.Is(missing, errChannelDeliveryTargetUnavailable) || !errors.Is(missing, gorm.ErrRecordNotFound) {
		t.Fatalf("missing target error = %v, want an unavailable target that keeps its cause", missing)
	}
	failure := channelDeliveryLookupError("load channel installation for delivery", errors.New("connection reset"))
	if errors.Is(failure, errChannelDeliveryTargetUnavailable) {
		t.Fatalf("database failure %v was classified as an unavailable target", failure)
	}
}
