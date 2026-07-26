package workflow

import "time"

type workflowStreamSelectionKind int

const (
	workflowStreamSelectionResult workflowStreamSelectionKind = iota + 1
	workflowStreamSelectionError
	workflowStreamSelectionDone
	workflowStreamSelectionContextDone
	workflowStreamSelectionHeartbeat
)

var workflowStreamHeartbeatInterval = 10 * time.Second

type workflowStreamSelection struct {
	kind    workflowStreamSelectionKind
	event   *WorkflowStreamEvent
	err     error
	outputs map[string]interface{}
	ok      bool
}

func receiveWorkflowStreamSelection(
	resultChan <-chan *WorkflowStreamEvent,
	errorChan <-chan error,
	doneChan <-chan map[string]interface{},
	ctxDone <-chan struct{},
) workflowStreamSelection {
	var heartbeat <-chan time.Time
	var heartbeatTimer *time.Timer
	if workflowStreamHeartbeatInterval > 0 {
		heartbeatTimer = time.NewTimer(workflowStreamHeartbeatInterval)
		defer heartbeatTimer.Stop()
		heartbeat = heartbeatTimer.C
	}

	for {
		// Terminal failures must win over a merely closed doneChan, otherwise the stream
		// can end before workflow_finished(failed) is emitted to the frontend.
		if errorChan != nil {
			select {
			case err, ok := <-errorChan:
				if !ok {
					errorChan = nil
				} else if err != nil {
					return workflowStreamSelection{kind: workflowStreamSelectionError, err: err}
				}
			default:
			}
		}

		if resultChan != nil {
			select {
			case event, ok := <-resultChan:
				if !ok {
					resultChan = nil
				} else if event != nil {
					return workflowStreamSelection{kind: workflowStreamSelectionResult, event: event}
				}
			default:
			}
		}

		// executeWorkflowStream closes resultChan and errorChan before its wrapper closes
		// doneChan. Do not observe completion until both event channels have been fully
		// drained. A best-effort priority check is insufficient here: when the producer
		// emits its final events and exits before this goroutine is scheduled again, all
		// three channels are ready and a select may randomly choose done first.
		if resultChan == nil && errorChan == nil {
			select {
			case outputs, ok := <-doneChan:
				return workflowStreamSelection{kind: workflowStreamSelectionDone, outputs: outputs, ok: ok}
			case <-ctxDone:
				return workflowStreamSelection{kind: workflowStreamSelectionContextDone}
			case <-heartbeat:
				return workflowStreamSelection{kind: workflowStreamSelectionHeartbeat}
			}
		}

		if ctxDone != nil {
			select {
			case <-ctxDone:
				return workflowStreamSelection{kind: workflowStreamSelectionContextDone}
			default:
			}
		}

		select {
		case err, ok := <-errorChan:
			if !ok {
				errorChan = nil
				continue
			}
			if err != nil {
				return workflowStreamSelection{kind: workflowStreamSelectionError, err: err}
			}
		case event, ok := <-resultChan:
			if !ok {
				resultChan = nil
				continue
			}
			if event != nil {
				return workflowStreamSelection{kind: workflowStreamSelectionResult, event: event}
			}
		case <-ctxDone:
			return workflowStreamSelection{kind: workflowStreamSelectionContextDone}
		case <-heartbeat:
			return workflowStreamSelection{kind: workflowStreamSelectionHeartbeat}
		}
	}
}
