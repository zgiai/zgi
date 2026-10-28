package digitalhuman

import (
	"errors"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/datatypes"
)

func TestBindingAuthorizationModeMapsTheApprovalPolicy(t *testing.T) {
	ssh := catalog.ServerOpsSSHDefinition()
	for metadata, want := range map[string]string{
		`{"allowed_operation_ids":["server.command.run"]}`:                                    core.AuthorizationModeAsk,
		`{"approval_policy":"always_ask"}`:                                                    core.AuthorizationModeAsk,
		`{"approval_policy":"read_only_auto","allowed_operation_ids":["server.command.run"]}`: core.AuthorizationModeReadOnlyAuto,
	} {
		got, err := bindingAuthorizationMode(&ResourceBinding{Metadata: datatypes.JSON(metadata)}, ssh)
		if err != nil || got != want {
			t.Errorf("bindingAuthorizationMode(%s) = %q, %v, want %q", metadata, got, err, want)
		}
	}
	if _, err := bindingAuthorizationMode(&ResourceBinding{Metadata: datatypes.JSON(`{"approval_policy":"direct"}`)}, ssh); !errors.Is(err, ErrResourceBindingInvalid) {
		t.Errorf("an unknown policy was accepted: %v", err)
	}
	other := ssh
	other.ID = "github"
	if _, err := bindingAuthorizationMode(&ResourceBinding{Metadata: datatypes.JSON(`{"approval_policy":"read_only_auto"}`)}, other); !errors.Is(err, ErrResourceBindingInvalid) {
		t.Errorf("a connector that cannot prove reads accepted read_only_auto: %v", err)
	}
}
