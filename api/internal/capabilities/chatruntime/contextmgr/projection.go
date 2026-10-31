package contextmgr

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	// defaultArtifactPartRunes is how much of an original one read returns
	// when the model names no limit; maxArtifactPartRunes bounds any read.
	defaultArtifactPartRunes = 12_000
	maxArtifactPartRunes     = 24_000
	// maxArtifactQueryLines bounds the lines a search returns.
	maxArtifactQueryLines = 200
)

// rememberOriginal keeps the complete content of a projected tool result for
// read_context_artifact and returns its reference.
func (m *Manager) rememberOriginal(hash, content string) (string, error) {
	runPart, err := safePathPart(m.state.AgentRunID)
	if err != nil {
		return "", err
	}
	if m.originals == nil {
		m.originals = map[string]string{}
	}
	m.originals[hash] = content
	return contextArtifactScheme + "://" + contextArtifactHost + "/" + runPart + "/" + hash, nil
}

// originalContent finds the complete tool result with this content hash:
// one projected in this process, or one in the turn's transcript, which keeps
// originals across continuations and resumes.
func (m *Manager) originalContent(hash string) (string, bool) {
	if content, ok := m.originals[hash]; ok {
		return content, true
	}
	for _, messages := range [][]adapter.Message{m.state.TurnTranscript, m.state.Messages} {
		for _, message := range messages {
			role := strings.ToLower(strings.TrimSpace(message.Role))
			if role != "tool" && role != "user" {
				continue
			}
			content := contentString(message.Content)
			digest := sha256.Sum256([]byte(content))
			if hex.EncodeToString(digest[:]) == hash {
				return content, true
			}
		}
	}
	return "", false
}

// headAndTail keeps the beginning and the end of content within limit
// characters, three quarters from the beginning.
func headAndTail(content string, limit int) string {
	runes := []rune(content)
	if limit <= 0 || len(runes) <= limit {
		return content
	}
	head := limit * 3 / 4
	tail := limit - head
	return string(runes[:head]) +
		fmt.Sprintf("\n\n[... %d characters omitted; read them with read_context_artifact ...]\n\n", len(runes)-head-tail) +
		string(runes[len(runes)-tail:])
}

// ReadContextArtifactPart returns a part of a projected tool result's
// original: the characters from part.Offset, or the lines that contain
// part.Query. A whole original is returned only when it fits one part, so a
// read cannot bring back the size the projection removed.
func (m *Manager) ReadContextArtifactPart(ctx context.Context, artifactRef string, part ContextArtifactPart) (ContextArtifactResult, error) {
	if m == nil {
		return ContextArtifactResult{}, fmt.Errorf("context artifact storage is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return ContextArtifactResult{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	runPart, contentHash, err := parseContextArtifactRef(artifactRef)
	if err != nil {
		return ContextArtifactResult{}, err
	}
	expectedRunPart, err := safePathPart(m.state.AgentRunID)
	if err != nil {
		return ContextArtifactResult{}, err
	}
	if runPart != expectedRunPart && !artifactRefAppearsInMessages(artifactRef, m.state.Messages) {
		return ContextArtifactResult{}, fmt.Errorf("context artifact does not belong to the current Agent run")
	}
	var content string
	if m.toolStore != nil {
		if content, err = m.toolStore.Get(ctx, runPart, contentHash); err != nil {
			return ContextArtifactResult{}, err
		}
	} else {
		var ok bool
		if content, ok = m.originalContent(contentHash); !ok {
			return ContextArtifactResult{}, fmt.Errorf("context artifact is no longer available")
		}
	}
	digest := sha256.Sum256([]byte(content))
	if hex.EncodeToString(digest[:]) != contentHash {
		return ContextArtifactResult{}, fmt.Errorf("context artifact integrity check failed")
	}

	runes := []rune(content)
	limit := part.Limit
	if limit <= 0 {
		limit = defaultArtifactPartRunes
	}
	limit = min(limit, maxArtifactPartRunes)
	result := ContextArtifactResult{
		ArtifactRef:      strings.TrimSpace(artifactRef),
		ContentHash:      contentHash,
		SourceToolCallID: m.sourceToolCallIDForArtifact(artifactRef),
		TotalChars:       len(runes),
		TotalTokens:      m.estimator.EstimateText(content, strings.TrimSpace(m.state.Model)).Tokens,
	}
	if query := strings.TrimSpace(part.Query); query != "" {
		result.Query = query
		result.Content = matchingLines(content, query, limit)
	} else {
		offset := min(max(part.Offset, 0), len(runes))
		end := min(offset+limit, len(runes))
		result.Offset = offset
		result.Content = string(runes[offset:end])
		result.Complete = offset == 0 && end == len(runes)
	}
	result.ReturnedTokens = m.estimator.EstimateText(result.Content, strings.TrimSpace(m.state.Model)).Tokens
	return result, nil
}

// matchingLines returns the lines of content that contain query, ignoring
// case, each with its line number, within limit characters.
func matchingLines(content, query string, limit int) string {
	query = strings.ToLower(query)
	var builder strings.Builder
	count := 0
	for number, line := range strings.Split(content, "\n") {
		if !strings.Contains(strings.ToLower(line), query) {
			continue
		}
		entry := fmt.Sprintf("%d: %s\n", number+1, line)
		if count >= maxArtifactQueryLines || len([]rune(builder.String()))+len([]rune(entry)) > limit {
			builder.WriteString("[more matching lines omitted]\n")
			break
		}
		builder.WriteString(entry)
		count++
	}
	if count == 0 {
		return "[no line contains the query]"
	}
	return builder.String()
}
