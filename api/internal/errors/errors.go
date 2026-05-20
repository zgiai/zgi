package errors

import "errors"

// Common errors
var (
	ErrNotFound                  = errors.New("not found")
	ErrUnauthorized              = errors.New("unauthorized")
	ErrForbidden                 = errors.New("forbidden")
	ErrInvalidParam              = errors.New("invalid parameter")
	ErrInternal                  = errors.New("internal server error")
	ErrEmailFrozen               = errors.New("this email is frozen, please contact the administrator")
	ErrNoWorkspace               = errors.New("no workspace found, please contact system admin to invite you to join in a workspace")
	ErrAccountAlreadyInWorkspace = errors.New("account already in tenant")
	// ... other common errors ...
)

// Account operation related errors
var (
	ErrCannotOperateSelf    = errors.New("cannot operate on self")
	ErrNoPermission         = errors.New("no permission to perform this action")
	ErrMemberNotInWorkspace = errors.New("member not in tenant")
	ErrInvalidAction        = errors.New("invalid action")
	ErrRoleAlreadyAssigned  = errors.New("the provided role is already assigned to the member")
)

// Optional: custom error type with code
type CodeError struct {
	Code    string
	Message string
}

func (e *CodeError) Error() string {
	return e.Message
}

func NewCodeError(code, msg string) *CodeError {
	return &CodeError{Code: code, Message: msg}
}

// Specific error types
type CannotOperateSelfError struct {
	Message string
}

func (e *CannotOperateSelfError) Error() string {
	return e.Message
}

func NewCannotOperateSelfError(msg string) *CannotOperateSelfError {
	if msg == "" {
		msg = "cannot operate on self"
	}
	return &CannotOperateSelfError{Message: msg}
}

type NoPermissionError struct {
	Message string
}

func (e *NoPermissionError) Error() string {
	return e.Message
}

func NewNoPermissionError(msg string) *NoPermissionError {
	if msg == "" {
		msg = "no permission to perform this action"
	}
	return &NoPermissionError{Message: msg}
}

type MemberNotInWorkspaceError struct {
	Message string
}

func (e *MemberNotInWorkspaceError) Error() string {
	return e.Message
}

func NewMemberNotInWorkspaceError(msg string) *MemberNotInWorkspaceError {
	if msg == "" {
		msg = "member not in tenant"
	}
	return &MemberNotInWorkspaceError{Message: msg}
}

type InvalidActionError struct {
	Message string
}

func (e *InvalidActionError) Error() string {
	return e.Message
}

func NewInvalidActionError(msg string) *InvalidActionError {
	if msg == "" {
		msg = "invalid action"
	}
	return &InvalidActionError{Message: msg}
}

type RoleAlreadyAssignedError struct {
	Message string
}

func (e *RoleAlreadyAssignedError) Error() string {
	return e.Message
}

func NewRoleAlreadyAssignedError(msg string) *RoleAlreadyAssignedError {
	if msg == "" {
		msg = "the provided role is already assigned to the member"
	}
	return &RoleAlreadyAssignedError{Message: msg}
}
