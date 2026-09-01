package text2sql

import "errors"

var (
	ErrInvalidInput    = errors.New("text2sql input is invalid")
	ErrSchemaLoad      = errors.New("text2sql schema load failed")
	ErrIndexNotReady   = errors.New("AI schema index is not ready; open the database's AI schema index panel to build or retry it")
	ErrGeneration      = errors.New("text2sql generation failed")
	ErrInvalidResponse = errors.New("text2sql response is invalid")
	ErrSQLParse        = errors.New("text2sql sql parse failed")
	ErrTableOutOfScope = errors.New("text2sql table is outside selected scope")
	ErrColumnNotFound  = errors.New("text2sql column does not exist")
	ErrGuardDenied     = errors.New("text2sql sql guard denied")
)
