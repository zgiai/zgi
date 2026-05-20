// Package inspectsvc contains HTTP-free PDF inspection orchestration for the
// playground: native full-document parsing, page-level VLM fallback, image
// captions, PDF rasterization, and OpenAI-compatible Chat Completions calls.
//
// UI layers should handle only multipart input, caching, and task progress;
// inspection logic belongs here so the runtime has a single path.
package inspectsvc
