package connectors_test

import (
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	connectorbindings "github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	connectorcatalog "github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	connectorconnections "github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
)

const connectorSecretSafetySentinel = "CONNECTOR-SECRET-SAFETY-SENTINEL"

func TestConnectorSecretBearingBoundaryFieldsAreNeverSerialized(t *testing.T) {
	models := map[string]any{
		"personal connection": connectorcore.PersonalConnection{
			EncryptedCredentials: connectorSecretSafetySentinel,
		},
		"setup attempt": connectorcore.KeySetupAttempt{
			IdempotencyKeyHash: connectorSecretSafetySentinel,
			RequestFingerprint: connectorSecretSafetySentinel,
		},
		"setup command": connectorconnections.KeySetupCommand{
			IdempotencyKey: connectorSecretSafetySentinel,
			Values:         map[string]string{"token": connectorSecretSafetySentinel},
		},
		"reconnect command": connectorconnections.ReconnectCommand{
			IdempotencyKey: connectorSecretSafetySentinel,
			Values:         map[string]string{"token": connectorSecretSafetySentinel},
		},
		"candidate verification": connectorconnections.CandidateVerificationRequest{
			Credentials: map[string]string{"token": connectorSecretSafetySentinel},
		},
		"credential seal": connectorconnections.CredentialSealRequest{
			Values: map[string]string{"token": connectorSecretSafetySentinel},
		},
		"binding command": connectorbindings.UpsertBindingCommand{
			IdempotencyKey: connectorSecretSafetySentinel,
		},
		"execution request": connectorexecution.ExecutionRequest{
			Input: map[string]string{"token": connectorSecretSafetySentinel},
		},
		"credential resolution": connectorexecution.CredentialResolutionRequest{
			EncryptedCredentials: connectorSecretSafetySentinel,
		},
		"provider invocation": connectorexecution.ProviderInvocation{
			Credentials: map[string]string{"token": connectorSecretSafetySentinel},
			Input:       connectorSecretSafetySentinel,
		},
		"catalog definition": connectorcatalog.Definition{
			PluginDigest: connectorSecretSafetySentinel,
		},
	}

	for name, model := range models {
		t.Run(name, func(t *testing.T) {
			encoded, err := json.Marshal(model)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			if strings.Contains(string(encoded), connectorSecretSafetySentinel) {
				t.Fatalf("serialized secret sentinel: %s", encoded)
			}
		})
	}

	publicError := connectorcore.NewError(
		connectorcore.ErrorCodeInternal,
		true,
		errors.New(connectorSecretSafetySentinel),
	)
	encodedError, err := json.Marshal(publicError)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(publicError.Error(), connectorSecretSafetySentinel) ||
		strings.Contains(string(encodedError), connectorSecretSafetySentinel) {
		t.Fatalf("public error retained provider cause: %s / %s", publicError, encodedError)
	}

}

func TestConnectorProductionStructsAndLoggingKeepSecretFieldsInternal(t *testing.T) {
	secretFields := map[string]struct{}{
		"EncryptedCredentials":   {},
		"IdempotencyKeyHash":     {},
		"RequestFingerprint":     {},
		"ProtectedCredentialRef": {},
		"ReversalSnapshotRef":    {},
		"Values":                 {},
		"Credentials":            {},
		"Input":                  {},
	}
	logFunctions := map[string]struct{}{
		"Debug": {}, "Info": {}, "Warn": {}, "Error": {}, "Fatal": {},
		"Print": {}, "Printf": {}, "Println": {}, "Sprintf": {}, "Errorf": {},
	}

	root := "."
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
		if parseErr != nil {
			return parseErr
		}
		// This private wire type exists only inside the authenticated continuation
		// codec. Its input must be serialized before encryption. Public requests
		// and all other structs still exclude Input from JSON.
		var encryptedInput *ast.Field
		if filepath.ToSlash(path) == "credentials/continuation.go" {
			ast.Inspect(parsed, func(node ast.Node) bool {
				typeSpec, ok := node.(*ast.TypeSpec)
				if !ok || typeSpec.Name.Name != "connectorContinuationPayload" {
					return true
				}
				structure, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					return false
				}
				for _, field := range structure.Fields.List {
					for _, name := range field.Names {
						if name.Name == "Input" {
							encryptedInput = field
						}
					}
				}
				return false
			})
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			switch current := node.(type) {
			case *ast.Field:
				if current == encryptedInput {
					return true
				}
				for _, name := range current.Names {
					if _, sensitive := secretFields[name.Name]; !sensitive {
						continue
					}
					tag := ""
					if current.Tag != nil {
						tag = strings.Trim(current.Tag.Value, "`")
					}
					if reflect.StructTag(tag).Get("json") != "-" {
						t.Errorf("%s: secret-bearing field %s must use json:\"-\"", path, name.Name)
					}
				}
			case *ast.CallExpr:
				selector, ok := current.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if _, isLogLike := logFunctions[selector.Sel.Name]; !isLogLike {
					return true
				}
				for _, argument := range current.Args {
					if field, found := sensitiveIdentifier(argument, secretFields); found {
						t.Errorf("%s: log/format call %s includes secret-bearing field %s", path, selector.Sel.Name, field)
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func sensitiveIdentifier(node ast.Node, sensitive map[string]struct{}) (string, bool) {
	match := ""
	ast.Inspect(node, func(current ast.Node) bool {
		if match != "" {
			return false
		}
		identifier, ok := current.(*ast.Ident)
		if !ok {
			return true
		}
		if _, found := sensitive[identifier.Name]; found {
			match = identifier.Name
			return false
		}
		return true
	})
	return match, match != ""
}
