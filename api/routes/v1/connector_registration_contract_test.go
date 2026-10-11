package v1

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestDigitalHumanProductionConnectorWiring(t *testing.T) {
	for _, check := range []struct{ file, function, call string }{
		{"routes.go", "RegisterRoutes", "GetConnectorSurfaceDispatcher"},
		{"digital_human_routes.go", "InitializeDigitalHumanRuntime", "newDigitalHumanSkillRuntime"},
	} {
		source, err := parser.ParseFile(token.NewFileSet(), check.file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, declaration := range source.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok || fn.Name.Name != check.function {
				continue
			}
			ast.Inspect(fn, func(node ast.Node) bool {
				if check.file == "routes.go" {
					literal, ok := node.(*ast.CompositeLit)
					if !ok {
						return true
					}
					name, ok := literal.Type.(*ast.Ident)
					if !ok || name.Name != "DigitalHumanRuntimeDeps" {
						return true
					}
					for _, element := range literal.Elts {
						field, ok := element.(*ast.KeyValueExpr)
						if !ok {
							continue
						}
						key, ok := field.Key.(*ast.Ident)
						if !ok || key.Name != "ConnectorDispatcher" {
							continue
						}
						call, ok := field.Value.(*ast.CallExpr)
						if !ok {
							continue
						}
						selector, ok := call.Fun.(*ast.SelectorExpr)
						found = ok && selector.Sel.Name == check.call
					}
				} else if call, ok := node.(*ast.CallExpr); ok {
					if name, ok := call.Fun.(*ast.Ident); ok && name.Name == check.call {
						found = true
					}
				}
				return true
			})
		}
		if !found {
			t.Errorf("%s must wire DigitalHuman through %s", check.function, check.call)
		}
	}
}

func TestRegisterRoutesAlwaysInitializesConnectors(t *testing.T) {
	source, err := parser.ParseFile(token.NewFileSet(), "routes.go", nil, 0)
	if err != nil {
		t.Fatalf("parse routes.go: %v", err)
	}
	var register *ast.FuncDecl
	for _, declaration := range source.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok && function.Name.Name == "RegisterRoutes" {
			register = function
			break
		}
	}
	if register == nil {
		t.Fatal("RegisterRoutes is missing")
	}

	admitted, registered := false, false
	runtimeDependencies := map[string]bool{
		"ConnectorChatRuntime":       false,
		"ConnectorAgentRuntime":      false,
		"ConnectorConfirmationStore": false,
	}
	// These must be statements in the main registration body, not inside an
	// optional feature gate. Authentication remains in RegisterConnectorRoutes.
	for _, statement := range register.Body.List {
		switch statement := statement.(type) {
		case *ast.IfStmt:
			if initializer, ok := statement.Init.(*ast.AssignStmt); ok {
				for _, expression := range initializer.Rhs {
					if call, ok := expression.(*ast.CallExpr); ok {
						if selector, ok := call.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "EnsureConnectorDefinitions" {
							admitted = true
						}
					}
				}
			}
		case *ast.ExprStmt:
			if call, ok := statement.X.(*ast.CallExpr); ok {
				if function, ok := call.Fun.(*ast.Ident); ok && function.Name == "RegisterConnectorRoutes" {
					registered = true
				}
			}
		case *ast.AssignStmt:
			for _, expression := range statement.Rhs {
				literal, ok := expression.(*ast.CompositeLit)
				if !ok {
					continue
				}
				name, ok := literal.Type.(*ast.Ident)
				if !ok || name.Name != "AIChatRouteDeps" {
					continue
				}
				for _, element := range literal.Elts {
					if field, ok := element.(*ast.KeyValueExpr); ok {
						if name, ok := field.Key.(*ast.Ident); ok {
							if _, required := runtimeDependencies[name.Name]; required {
								runtimeDependencies[name.Name] = true
							}
						}
					}
				}
			}
		}
	}
	if !admitted || !registered {
		t.Errorf("Connector definition admission and route registration must be unconditional: admission=%t registration=%t", admitted, registered)
	}
	for name, configured := range runtimeDependencies {
		if !configured {
			t.Errorf("AIChatRouteDeps must always configure %s", name)
		}
	}
}
