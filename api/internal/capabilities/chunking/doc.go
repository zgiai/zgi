// Package chunking contains the capability boundary between document parsing
// and business-specific indexing or preview flows.
//
// It is intentionally provider-agnostic: upstream parsing services produce a
// contracts.ParseArtifact, and this package plans or normalizes chunks without
// knowing whether the artifact came from local rules, VLM, Reducto, MinerU, or
// another provider.
//
// The package is currently a non-invasive foundation. It is safe to wire for
// shadow inspection while existing dataset indexing continues to use its legacy
// processors until an explicit cutover flag is introduced.
package chunking
