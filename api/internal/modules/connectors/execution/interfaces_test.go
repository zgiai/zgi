package execution

import (
	"github.com/zgiai/zgi/api/internal/modules/connectors/audit"
	"github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
)

var (
	_ BindingReader           = (*bindings.GormRepository)(nil)
	_ ConnectionReader        = (*connections.GormRepository)(nil)
	_ DecisionOutcomeRecorder = (*audit.GormRepository)(nil)
)
