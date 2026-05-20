package baseline

// File is one ordered chunk of the public initial schema.
// Statements are explicit PostgreSQL DDL managed by Go migrations.
type File struct {
	Name       string
	Statements []string
}

var Files = []File{
	ExtensionsSchema,
	IdentityAccessSchema,
	AppsWorkflowsSchema,
	ModelGatewaySchema,
	DataKnowledgeSchema,
	BillingCommerceSchema,
	RuntimeSystemSchema,
	CompatViewsSchema,
	ConstraintsSchema,
	IndexesSchema,
	ForeignKeysSchema,
}
