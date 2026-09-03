package execution

import (
	"context"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"testing"
)

func TestContinuationRejectsChangedAndTerminalNativeClaim(t *testing.T) {
	for _, change := range []string{"input", "contract", "unknown", "completed"} {
		t.Run(change, func(t *testing.T) {
			f := newConfirmationContinuationTestFixture(t)
			switch change {
			case "input":
				f.claim.InputDigest = "changed"
			case "contract":
				f.claim.ContractDigest = "changed"
			default:
				f.claim.State = change
			}
			f.mock.ExpectQuery(`SELECT .* FROM "connector_invocation_claims"`).WillReturnRows(nativeInvocationRows(f.claim))
			if _, err := f.store.Freeze(context.Background(), f.frozen); !core.IsErrorCode(err, core.ErrorCodeAccessDenied) {
				t.Fatalf("accepted %s: %v", change, err)
			}
			if err := f.mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
