package skills

import "embed"

// embeddedSkillCatalog keeps system skills available when the runtime is
// deployed as a standalone binary without the source tree next to it.
//
//go:embed catalog/**/*
var embeddedSkillCatalog embed.FS
