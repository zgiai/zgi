package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"github.com/zgiai/zgi/api/pkg/logger"
)

func answerTemplateSegmentsFingerprint(segments []answerTemplateSegment) string {
	hasher := sha256.New()
	for _, segment := range segments {
		fmt.Fprintf(hasher, "%s|%d|%s|%d|%s|", segment.kind, len(segment.text), segment.text, len(segment.selectorKey), segment.selectorKey)
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func (c *answerOutputCoordinator) PreparePauseSnapshot() (*workflowpause.AnswerOutputState, []answerMessageChunk) {
	if c == nil {
		return nil, nil
	}

	c.emitMu.Lock()
	defer c.emitMu.Unlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	messages := c.drainLocked()
	for _, variable := range c.variables {
		if len(variable.chunks) == 0 {
			continue
		}
		variable.finalOnly = true
		variable.chunks = nil
	}

	return c.pauseSnapshotLocked(), messages
}

func (c *answerOutputCoordinator) RestorePauseSnapshot(snapshot *workflowpause.AnswerOutputState) error {
	if c == nil || snapshot == nil {
		return nil
	}

	c.emitMu.Lock()
	defer c.emitMu.Unlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.validatePauseSnapshotLocked(snapshot); err != nil {
		return err
	}

	for _, variable := range c.variables {
		variable.releasedText = ""
		variable.hasFinal = false
		variable.finalValue = ""
		variable.finalOnly = false
		variable.sourceSkipped = false
		variable.sourceFailed = false
		variable.finalizedSegment = false
		variable.chunks = nil
	}

	for _, emitter := range c.emitters {
		emitter.lifecycle = answerEmitterUnknown
		emitter.currentIndex = 0
		emitter.drained = false
		emitter.batchBarrierPassed = false
	}

	c.fullAnswer = strings.Builder{}
	if snapshot.FullAnswer != "" {
		c.fullAnswer.WriteString(snapshot.FullAnswer)
	}
	c.messageSent = snapshot.MessageSent

	for _, emitterState := range snapshot.Emitters {
		emitter := c.pauseSnapshotEmitterLocked(emitterState.NodeID)
		emitter.lifecycle = emitterState.Lifecycle
		emitter.currentIndex = emitterState.CurrentIndex
		emitter.drained = emitterState.Drained
		emitter.batchBarrierPassed = emitter.drained
	}

	for _, variableState := range snapshot.Variables {
		variable := c.variables[variableState.StateKey]
		variable.releasedText = variableState.ReleasedText
		variable.hasFinal = variableState.HasFinal
		variable.finalValue = variableState.FinalValue
		variable.finalOnly = variableState.FinalOnly
		variable.sourceSkipped = variableState.SourceSkipped
		variable.sourceFailed = variableState.SourceFailed
		variable.finalizedSegment = variableState.FinalizedSegment
	}

	return nil
}

func (c *answerOutputCoordinator) pauseSnapshotLocked() *workflowpause.AnswerOutputState {
	if c == nil {
		return nil
	}

	snapshot := &workflowpause.AnswerOutputState{
		FullAnswer:  c.fullAnswer.String(),
		MessageSent: c.messageSent,
		Emitters:    make([]workflowpause.AnswerOutputEmitterState, 0, len(c.emitterOrder)),
		Variables:   make([]workflowpause.AnswerOutputVariableState, 0, len(c.variables)),
	}

	for _, nodeID := range c.emitterOrder {
		emitter := c.emitters[nodeID]
		if emitter == nil {
			continue
		}
		snapshot.Emitters = append(snapshot.Emitters, workflowpause.AnswerOutputEmitterState{
			NodeID:              emitter.key,
			Lifecycle:           emitter.lifecycle,
			CurrentIndex:        emitter.currentIndex,
			Drained:             emitter.drained,
			TemplateFingerprint: emitter.templateFingerprint,
		})
	}

	variableKeys := make([]string, 0, len(c.variables))
	for stateKey := range c.variables {
		variableKeys = append(variableKeys, stateKey)
	}
	sort.Strings(variableKeys)
	for _, stateKey := range variableKeys {
		variable := c.variables[stateKey]
		snapshot.Variables = append(snapshot.Variables, workflowpause.AnswerOutputVariableState{
			StateKey:         variable.stateKey,
			ReleasedText:     variable.releasedText,
			HasFinal:         variable.hasFinal,
			FinalValue:       variable.finalValue,
			FinalOnly:        variable.finalOnly,
			SourceSkipped:    variable.sourceSkipped,
			SourceFailed:     variable.sourceFailed,
			FinalizedSegment: variable.finalizedSegment,
		})
	}

	return snapshot
}

func (c *answerOutputCoordinator) validatePauseSnapshotLocked(snapshot *workflowpause.AnswerOutputState) error {
	for _, emitterState := range snapshot.Emitters {
		emitter := c.pauseSnapshotEmitterLocked(emitterState.NodeID)
		if emitter == nil {
			return fmt.Errorf("answer emitter %s not found during pause restore", emitterState.NodeID)
		}
		if emitter.templateFingerprint != emitterState.TemplateFingerprint {
			return fmt.Errorf("answer emitter %s template fingerprint mismatch during pause restore", emitterState.NodeID)
		}
		if emitterState.CurrentIndex < 0 || emitterState.CurrentIndex > len(emitter.segments) {
			return fmt.Errorf("answer emitter %s current index %d out of range during pause restore", emitterState.NodeID, emitterState.CurrentIndex)
		}
	}

	for _, variableState := range snapshot.Variables {
		if _, exists := c.variables[variableState.StateKey]; !exists {
			return fmt.Errorf("answer variable %s not found during pause restore", variableState.StateKey)
		}
	}

	return nil
}

func (c *answerOutputCoordinator) pauseSnapshotEmitterLocked(snapshotNodeID string) *answerEmitter {
	if emitter := c.emitters[snapshotNodeID]; emitter != nil {
		return emitter
	}
	return c.emitters[answerEmitterKey(answerTopScope(), snapshotNodeID)]
}

func (c *answerOutputCoordinator) emitFinalOnlyLocked(nodeID string, variable *answerVariableState) []answerMessageChunk {
	if variable == nil || variable.finalValue == "" && variable.releasedText == "" {
		return nil
	}

	switch {
	case variable.releasedText == "":
		return c.recordFinalOnlyChunksLocked(nodeID, variable.finalValue)
	case strings.HasPrefix(variable.finalValue, variable.releasedText):
		return c.recordFinalOnlyChunksLocked(nodeID, strings.TrimPrefix(variable.finalValue, variable.releasedText))
	default:
		logger.Warn("answer ordered output final-only value does not match streamed prefix",
			"node_id", nodeID,
			"selector", variable.selectorKey,
			"streamed_length", len(variable.releasedText),
			"final_length", len(variable.finalValue),
		)
		return nil
	}
}

func (c *answerOutputCoordinator) recordFinalOnlyChunksLocked(nodeID string, text string) []answerMessageChunk {
	if text == "" {
		return nil
	}
	messages := make([]answerMessageChunk, 0)
	for _, chunk := range answerChunkText(text, c.chunkSize) {
		messages = append(messages, c.recordMessageLocked(nodeID, chunk))
	}
	return messages
}
