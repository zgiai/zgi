package pause

import graphentities "github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"

func SnapshotVariablePool(pool *graphentities.VariablePool) VariablePoolSnapshot {
	snapshot := VariablePoolSnapshot{
		Variables:       make(map[string]map[string]interface{}),
		UserInputs:      make(map[string]interface{}),
		SystemVariables: graphentities.SystemVariableEmpty(),
	}
	if pool == nil {
		return snapshot
	}

	if pool.SystemVariables != nil {
		copied := *pool.SystemVariables
		snapshot.SystemVariables = &copied
	}
	for key, value := range pool.UserInputs {
		snapshot.UserInputs[key] = value
	}
	for nodeID, variables := range pool.VariableDictionary {
		if snapshot.Variables[nodeID] == nil {
			snapshot.Variables[nodeID] = make(map[string]interface{}, len(variables))
		}
		for name, variable := range variables {
			if variable == nil {
				continue
			}
			snapshot.Variables[nodeID][name] = variable.ToObject()
		}
	}
	return snapshot
}

func RestoreVariablePoolSnapshot(pool *graphentities.VariablePool, snapshot VariablePoolSnapshot) {
	if pool == nil {
		return
	}
	if snapshot.SystemVariables != nil {
		copied := *snapshot.SystemVariables
		pool.SystemVariables = &copied
	}
	pool.UserInputs = make(map[string]interface{}, len(snapshot.UserInputs))
	for key, value := range snapshot.UserInputs {
		pool.UserInputs[key] = value
	}
	if pool.VariableDictionary == nil {
		pool.VariableDictionary = make(map[string]map[string]graphentities.Variable)
	}
	for nodeID, variables := range snapshot.Variables {
		for name, value := range variables {
			pool.Add([]string{nodeID, name}, value)
		}
	}
}
