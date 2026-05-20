package service

import "errors"

var (
	ErrAlreadySetup              = errors.New("system already setup")
	ErrNotInitValidated          = errors.New("system initialization validation is incomplete")
	ErrCloudBootstrapConfig      = errors.New("cloud bootstrap configuration is incomplete")
	ErrBootstrapAdminEmailExists = errors.New("bootstrap admin email already exists")
	ErrPasswordTooShort          = errors.New("password too short")
	ErrPasswordTooSimple         = errors.New("password too simple")
)
