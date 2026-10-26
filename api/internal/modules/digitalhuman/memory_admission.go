package digitalhuman

import (
	"fmt"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
)

// memoryWriteMode distinguishes how a memory change was produced. Background
// extraction never runs from console conversations, whatever the trust level.
type memoryWriteMode string

const (
	memoryWriteExplicit   memoryWriteMode = "explicit"
	memoryWriteProactive  memoryWriteMode = "proactive"
	memoryWriteBackground memoryWriteMode = "background"
)

const (
	memoryAdmissionReasonTrust        = "source_trust_not_admitted"
	memoryAdmissionReasonChannelScope = "channel_scope_unresolved"
	memoryAdmissionReasonEveryone     = "everyone_memory_operator_only"
)

var (
	errMemorySourceTrustRejected    = fmt.Errorf("%w: %s", ErrMemoryPolicyRejected, memoryAdmissionReasonTrust)
	errMemoryChannelScopeUnresolved = fmt.Errorf("%w: %s", ErrMemoryPolicyRejected, memoryAdmissionReasonChannelScope)
	// errMemoryEveryoneOperatorOnly rejects a change below operator trust to
	// a memory every user, external ones included, can see.
	errMemoryEveryoneOperatorOnly = fmt.Errorf("%w: %s", ErrMemoryPolicyRejected, memoryAdmissionReasonEveryone)
)

// err returns the rejection cause for a denied admission.
func (a memoryWriteAdmission) err() error {
	if a.Reason == memoryAdmissionReasonChannelScope {
		return errMemoryChannelScopeUnresolved
	}
	return errMemorySourceTrustRejected
}

// memoryWriteAdmission is the single source-trust policy for DigitalHuman
// memory writes (ADR 0004, ADR 0015). When admitted it returns the class and
// scope the write must use; the model never chooses them.
type memoryWriteAdmission struct {
	Allowed bool
	Reason  string
	Trust   SourceTrust
	Class   memoryruntime.MemoryClass
	Scope   *memoryruntime.MemoryScopeRef
	// InternalOnly limits the write to internal memories: everyone inside
	// the team shares them, and external end users never see them.
	InternalOnly    bool
	Origin          MemoryOrigin
	OriginChannelID string
}

// stamp gives a write request the visibility rule and origin of the
// admission.
func (a memoryWriteAdmission) stamp(request *MemoryProposalRequest) {
	request.internalOnly, request.origin, request.originChannelID = a.InternalOnly, a.Origin, a.OriginChannelID
	if a.InternalOnly || a.Scope != nil {
		request.Visibility = MemoryVisibilityInternal
	}
}

func admitMemoryWrite(run *Run, event invocation.Event, mode memoryWriteMode, autoExtraction bool) memoryWriteAdmission {
	trust := RunSourceTrust(run)
	rejected := memoryWriteAdmission{Reason: memoryAdmissionReasonTrust, Trust: trust}
	switch trust {
	case SourceTrustAuthorizedOperator:
		if mode == memoryWriteExplicit || (mode == memoryWriteProactive && autoExtraction) {
			return memoryWriteAdmission{Allowed: true, Trust: trust, Class: memoryruntime.MemoryClassSelf, Origin: MemoryOriginOperator}
		}
	case SourceTrustWorkspaceMember:
		// Members share the DigitalHuman's memory inside the team: explicitly
		// at any time, otherwise when automatic extraction is on.
		if mode == memoryWriteExplicit || autoExtraction {
			return memoryWriteAdmission{Allowed: true, Trust: trust, Class: memoryruntime.MemoryClassSelf, InternalOnly: true, Origin: MemoryOriginMember}
		}
	case SourceTrustChannelParticipant:
		if !autoExtraction {
			return rejected
		}
		channelID, ok := runChannelScopeID(run, event)
		if !ok {
			return memoryWriteAdmission{Reason: memoryAdmissionReasonChannelScope, Trust: trust}
		}
		return memoryWriteAdmission{
			Allowed: true, Trust: trust, Class: memoryruntime.MemoryClassSelf, InternalOnly: true,
			Origin: MemoryOriginChannel, OriginChannelID: channelID.String(),
		}
	case SourceTrustExternalEndUser:
		// An end user writes only their own instance's memory: explicitly at
		// any time, and proactively or in the background when automatic
		// extraction is on.
		instanceID, ok := runInstanceScopeID(run)
		if !ok || (mode != memoryWriteExplicit && !autoExtraction) {
			return rejected
		}
		return memoryWriteAdmission{
			Allowed: true, Trust: trust, Class: memoryruntime.MemoryClassBusiness, Origin: MemoryOriginEndUser,
			Scope: &memoryruntime.MemoryScopeRef{Type: memoryruntime.MemoryScopeInstance, ID: instanceID.String()},
		}
	}
	return rejected
}

// backgroundExtractionAdmitted reports whether a completed run may be queued
// for background extraction at all, so rejected sources cost no model call.
func backgroundExtractionAdmitted(run *Run) bool {
	if _, ok := runInstanceScopeID(run); ok {
		return true
	}
	trust := RunSourceTrust(run)
	return trust == SourceTrustChannelParticipant || trust == SourceTrustWorkspaceMember
}

// memoryToolAvailable reports whether the inline memory tool is offered. It
// mirrors admitMemoryWrite so the model is never handed a tool whose every
// call would be rejected.
func memoryToolAvailable(run *Run, autoExtraction bool) bool {
	switch RunSourceTrust(run) {
	case SourceTrustAuthorizedOperator, SourceTrustWorkspaceMember:
		return true
	case SourceTrustChannelParticipant:
		return autoExtraction
	case SourceTrustExternalEndUser:
		_, ok := runInstanceScopeID(run)
		return ok
	default:
		return false
	}
}

// memoryReadScope is the set of memories one run may recall: the
// DigitalHuman's own memory, without internal entries for an external end
// user (ADR 0015), and the run's own instance notes.
type memoryReadScope struct {
	// external hides internal memories.
	external   bool
	instanceID string
	// instanceSince hides instance memory unwritten since then, when
	// instance memory expires.
	instanceSince *time.Time
}

func memoryReadScopeForRun(run *Run, event invocation.Event) memoryReadScope {
	var scope memoryReadScope
	if run == nil {
		return scope
	}
	scope.external = RunSourceTrust(run) == SourceTrustExternalEndUser
	if instanceID, ok := runInstanceScopeID(run); ok {
		scope.instanceID = instanceID.String()
		if ttl := instanceMemoryTTL(); ttl > 0 {
			since := time.Now().Add(-ttl)
			scope.instanceSince = &since
		}
	}
	return scope
}

func (s memoryReadScope) allows(memory *DigitalHumanMemory) bool {
	if memory == nil {
		return false
	}
	if memory.MemoryClass == memoryruntime.MemoryClassSelf {
		return !s.external || memory.Visibility == MemoryVisibilityEveryone
	}
	switch memory.ScopeType {
	case memoryruntime.MemoryScopeInstance:
		return s.instanceID != "" && memory.ScopeID == s.instanceID && (s.instanceSince == nil || !memory.UpdatedAt.Before(*s.instanceSince))
	default:
		return false
	}
}

// sqlPredicate expresses allows for repository queries over
// digital_human_memories.
func (s memoryReadScope) sqlPredicate() (string, []any) {
	clause := "(memory_class = ?"
	args := []any{memoryruntime.MemoryClassSelf}
	if s.external {
		clause = "((memory_class = ? AND visibility = ?)"
		args = append(args, MemoryVisibilityEveryone)
	}
	if s.instanceID != "" {
		clause += " OR (memory_class = ? AND scope_type = ? AND scope_id = ?"
		args = append(args, memoryruntime.MemoryClassBusiness, memoryruntime.MemoryScopeInstance, s.instanceID)
		if s.instanceSince != nil {
			clause += " AND updated_at >= ?"
			args = append(args, *s.instanceSince)
		}
		clause += ")"
	}
	return clause + ")", args
}
