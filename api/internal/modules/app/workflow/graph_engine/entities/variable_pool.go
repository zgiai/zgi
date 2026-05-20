package entities

import (
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
)

func NewVariablePool() *VariablePool {
	vp := &VariablePool{
		VariableDictionary:    make(map[string]map[string]Variable),
		UserInputs:            make(map[string]interface{}),
		SystemVariables:       SystemVariableEmpty(),
		EnvironmentVariables:  make([]Variable, 0),
		ConversationVariables: make([]Variable, 0),
	}

	vp.modelPostInit()
	return vp
}

// Initialize registers all variables (system, environment, conversation) in the VariableDictionary
// This method should be called after modifying SystemVariables, EnvironmentVariables, or ConversationVariables
func (vp *VariablePool) Initialize() {
	vp.mu.Lock()
	defer vp.mu.Unlock()

	vp.modelPostInitLocked()
}

func (vp *VariablePool) modelPostInit() {
	vp.mu.Lock()
	defer vp.mu.Unlock()

	vp.modelPostInitLocked()
}

func (vp *VariablePool) modelPostInitLocked() {
	vp.ensureVariableDictionaryLocked()
	vp.addSystemVariablesLocked(vp.SystemVariables)

	for _, variable := range vp.EnvironmentVariables {
		nodeID := EnvironmentVariableNodeId
		variableKey := variable.GetName()
		if vp.VariableDictionary[nodeID] == nil {
			vp.VariableDictionary[nodeID] = make(map[string]Variable)
		}
		vp.VariableDictionary[nodeID][variableKey] = variable
	}

	for _, variable := range vp.ConversationVariables {
		nodeID := ConversationVariableNodeId
		variableKey := variable.GetName()
		if vp.VariableDictionary[nodeID] == nil {
			vp.VariableDictionary[nodeID] = make(map[string]Variable)
		}
		vp.VariableDictionary[nodeID][variableKey] = variable
	}
}

func (vp *VariablePool) Add(selector []string, value interface{}) {
	if len(selector) < SelectorsLength {
		return
	}

	variable := vp.segmentToVariable(vp.buildSegment(value), selector)

	vp.mu.Lock()
	defer vp.mu.Unlock()

	nodeID, variableKey := vp.selectorToKeys(selector)
	vp.ensureVariableDictionaryLocked()

	if vp.VariableDictionary[nodeID] == nil {
		vp.VariableDictionary[nodeID] = make(map[string]Variable)
	}

	vp.VariableDictionary[nodeID][variableKey] = variable
}

// AddSegment inserts a variable with a pre-built segment.
func (vp *VariablePool) AddSegment(selector []string, segment Segment) {
	if len(selector) < SelectorsLength || segment == nil {
		return
	}

	vp.mu.Lock()
	defer vp.mu.Unlock()

	nodeID, variableKey := vp.selectorToKeys(selector)
	vp.ensureVariableDictionaryLocked()

	if vp.VariableDictionary[nodeID] == nil {
		vp.VariableDictionary[nodeID] = make(map[string]Variable)
	}

	vp.VariableDictionary[nodeID][variableKey] = &variableWrapper{
		segment:  segment,
		name:     variableKey,
		selector: selector,
	}
}

func (vp *VariablePool) Get(selector []string) Variable {
	if len(selector) < SelectorsLength {
		return nil
	}

	vp.mu.RLock()
	defer vp.mu.RUnlock()

	return vp.getLocked(selector)
}

func (vp *VariablePool) getLocked(selector []string) Variable {
	nodeID, valName := vp.selectorToKeys(selector)

	if nodeDict, exists := vp.VariableDictionary[nodeID]; exists {
		if variable, found := nodeDict[valName]; found {
			// Requirement 5.4: Debug logging for variable access
			logger.Debug("workflow variable accessed",
				zap.String("node_id", nodeID),
				zap.String("variable_name", valName),
				zap.Int("selector_length", len(selector)),
				zap.Bool("found", true),
			)
			return variable
		}
		// Requirement 5.3: Non-existent variables return nil (empty value) instead of failing
		logger.Debug("workflow variable not found in node dictionary",
			zap.String("node_id", nodeID),
			zap.String("variable_name", valName),
			zap.Int("selector_length", len(selector)),
			zap.Bool("found", false),
		)
		return nil
	}

	// Requirement 5.3: Non-existent node returns nil (empty value) instead of failing
	logger.Debug("workflow variable node not found",
		zap.String("node_id", nodeID),
		zap.Int("selector_length", len(selector)),
		zap.Bool("found", false),
	)
	return nil
}

func (vp *VariablePool) Has(selector []string) bool {
	return vp.Get(selector) != nil
}

func (vp *VariablePool) GetWithPath(selector []string) Variable {
	if len(selector) < SelectorsLength {
		return nil
	}

	vp.mu.RLock()
	defer vp.mu.RUnlock()

	if len(selector) == SelectorsLength {
		return vp.getLocked(selector)
	}

	if len(selector) == 2 {
		result := vp.getLocked(selector)
		if result != nil {
			return result
		}
	} else if len(selector) > 2 {
		result := vp.getLocked(selector[:2])
		if result != nil {
			for _, attr := range selector[2:] {
				nestedSegment := vp.getNestedAttributeSegment(result, attr)
				if nestedSegment == nil {
					result = nil
					break
				}
				result = vp.segmentToVariable(nestedSegment, selector)
			}
			return result
		}
	}

	return nil
}

func (vp *VariablePool) Remove(selector []string) {
	if len(selector) == 0 {
		return
	}

	vp.mu.Lock()
	defer vp.mu.Unlock()

	vp.ensureVariableDictionaryLocked()
	if len(selector) == 1 {
		vp.VariableDictionary[selector[0]] = make(map[string]Variable)
		return
	}

	nodeID, variableKey := vp.selectorToKeys(selector)
	if nodeDict, exists := vp.VariableDictionary[nodeID]; exists {
		delete(nodeDict, variableKey)
	}
}

func (vp *VariablePool) selectorToKeys(selector []string) (string, string) {
	return selector[0], selector[1]
}

func (vp *VariablePool) addSystemVariablesLocked(systemVariable *SystemVariable) {
	if systemVariable == nil {
		return
	}
	sysVarMapping := systemVariable.ToDict()
	for key, value := range sysVarMapping {
		if value == nil {
			continue
		}
		selector := []string{SystemVariableNodeId, key}
		if vp.getLocked(selector) != nil {
			continue
		}
		variable := vp.segmentToVariable(vp.buildSegment(value), selector)
		if vp.VariableDictionary[SystemVariableNodeId] == nil {
			vp.VariableDictionary[SystemVariableNodeId] = make(map[string]Variable)
		}
		vp.VariableDictionary[SystemVariableNodeId][key] = variable
	}
}

func (vp *VariablePool) ensureVariableDictionaryLocked() {
	if vp.VariableDictionary == nil {
		vp.VariableDictionary = make(map[string]map[string]Variable)
	}
}

func (vw *variableWrapper) ToObject() interface{} {
	return vw.segment.ToObject()
}

func (vw *variableWrapper) GetValue() interface{} {
	return vw.segment.GetValue()
}

func (vw *variableWrapper) GetType() shared.SegmentType {
	return vw.segment.GetType()
}

func (vw *variableWrapper) GetName() string {
	return vw.name
}

func (vw *variableWrapper) GetSelector() []string {
	return vw.selector
}

func (vw *variableWrapper) Text() string {
	return vw.segment.Text()
}

func (vw *variableWrapper) Log() string {
	return vw.segment.Log()
}

func (vw *variableWrapper) Markdown() string {
	return vw.segment.Markdown()
}

func (vw *variableWrapper) Size() int {
	return vw.segment.Size()
}

func EmptyVariablePool() *VariablePool {
	return &VariablePool{
		VariableDictionary:    make(map[string]map[string]Variable),
		UserInputs:            make(map[string]interface{}),
		SystemVariables:       SystemVariableEmpty(),
		EnvironmentVariables:  make([]Variable, 0),
		ConversationVariables: make([]Variable, 0),
	}
}
