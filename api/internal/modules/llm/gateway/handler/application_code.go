package handler

import (
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	"github.com/zgiai/zgi/api/pkg/apperror"
)

// applicationCode maps a gateway/domain error to the stable cataloged identity
// that Pre exposes to clients. It is the single classification point shared by
// the legacy protocol classifier and the message projector, so the wire code
// and the projected code can never disagree.
//
// Ordering matters: the four quota causes carried by BillingUserError must be
// resolved before the generic DomainErrBillingFailed that BillingUserError
// wraps; otherwise every cause collapses into a system billing failure.
func applicationCode(err error) (apperror.Code, bool) {
	return gateway.ApplicationCodeForError(err)
}

// applicationErrorFor returns the cataloged application error for a gateway
// error. It preserves an already-wrapped apperror so operation, params, and
// cause survive projection; otherwise it wraps the concrete cause with the
// mapped code and a stable operation name.
func applicationErrorFor(err error) (error, bool) {
	if err == nil {
		return nil, false
	}
	if appErr, ok := apperror.As(err); ok {
		return appErr, true
	}
	code, supported := applicationCode(err)
	if !supported {
		return err, false
	}
	return apperror.Wrap(err, code, apperror.WithOperation("gateway.protocol_error")), true
}
