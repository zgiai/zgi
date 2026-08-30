package text2sql

import "errors"

var (
	ErrInvalidInput    = errors.New("text2sql input is invalid")
	ErrSchemaLoad      = errors.New("text2sql schema load failed")
	ErrIndexNotReady   = errors.New("text2sql schema index is not ready")
	ErrGeneration      = errors.New("text2sql generation failed")
	ErrInvalidResponse = errors.New("text2sql response is invalid")
	ErrSQLParse        = errors.New("text2sql sql parse failed")
	ErrTableOutOfScope = errors.New("text2sql table is outside selected scope")
	ErrColumnNotFound  = errors.New("text2sql column does not exist")
	ErrGuardDenied     = errors.New("text2sql sql guard denied")
)
