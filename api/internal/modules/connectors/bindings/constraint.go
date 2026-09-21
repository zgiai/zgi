package bindings

import (
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

// NewBindingConstraint captures the current operation set at creation. Runtime
// checks also apply the independent Connection constraint on every invocation.
func NewBindingConstraint(owner uuid.UUID, id core.Identifier) *core.AccessConstraint {
	operations := []core.Identifier{}
	definitions, err := catalog.BuiltinDefinitions()
	if err == nil {
		for _, definition := range definitions {
			if definition.ID == id.String() {
				for _, operation := range definition.Operations {
					operations = append(operations, core.Identifier(operation.ID))
				}
			}
		}
	}
	return core.OwnerConstraint(owner, operations)
}
