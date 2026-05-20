package baseline

var ExtensionsSchema = File{
	Name: "00_extensions",
	Statements: []string{
		`CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;`,
		`CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA public;`,
	},
}
