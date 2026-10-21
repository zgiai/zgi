package connectortools

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
)

const operationLocalizedNameBudget = 256

// connectionLabelBudget bounds the untrusted, user-controlled Personal
// Connection label projected into a model-visible Tool description.
const connectionLabelBudget = 80

// digitalHumanTargetMarkerHexLength bounds the opaque, non-reversible marker
// that stands in for a Personal Connection identity in model-visible text.
const digitalHumanTargetMarkerHexLength = 10

// errInvalidDigitalHumanConnectionIdentity is deliberately free of the rejected
// input so a malformed identity can never leak into diagnostics or model text.
var errInvalidDigitalHumanConnectionIdentity = errors.New("invalid Digital Human connection identity")

// OperationRuntimeDescription augments an admitted Connector operation's
// English instructions with bounded positive Chinese names for lexical tool
// discovery. It deliberately excludes localized long descriptions: a safety
// phrase such as "does not send messages" is not a sending capability.
// Descriptions are presentation only; callers must still use authorized
// operation snapshots for runtime projection.
func OperationRuntimeDescription(definition catalog.Definition, operation catalog.Operation) string {
	description := strings.TrimSpace(operation.Description)
	if description == "" {
		description = strings.TrimSpace(operation.Name)
	}
	action := simplifiedChineseName(operation.NameI18n)
	connector := simplifiedChineseName(definition.NameI18n)
	if strings.EqualFold(action, connector) {
		connector = ""
	}
	if action == "" && connector == "" {
		return description
	}
	separator := ""
	if description != "" {
		separator = "\n\n"
	}
	remaining := operationLocalizedNameBudget - len([]rune(separator))
	// Allocate the action first so a long connector brand cannot crowd out the
	// business operation. Never truncate the existing English instructions.
	localized := boundedName(action, remaining)
	remaining -= len([]rune(localized))
	if connector != "" {
		joiner := ""
		if localized != "" {
			joiner = " · "
		}
		if remaining > len([]rune(joiner)) {
			localized += joiner + boundedName(connector, remaining-len([]rune(joiner)))
		}
	}
	return description + separator + localized
}

func simplifiedChineseName(values catalog.LocalizedText) string {
	// These are exactly the aliases grouped by the catalog's locale validator.
	// Underscore tags, zh-Hant and arbitrary locale fallbacks are not aliases.
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, alias := range []string{"zh-hans", "zh-cn", "zh"} {
		for _, key := range keys {
			if strings.EqualFold(key, alias) {
				if name := strings.Join(strings.Fields(values[key]), " "); name != "" {
					return name
				}
			}
		}
	}
	return ""
}

func boundedName(value string, limit int) string {
	characters := []rune(value)
	if len(characters) > limit {
		characters = characters[:limit]
	}
	return string(characters)
}

// DigitalHumanConnectionRuntimeDescription appends a target-marker sentence to
// an admitted Connector operation's runtime description. A DigitalHuman host
// may bind the same Connector operation more than once, so the sentence
// distinguishes the exact owner-scoped Connection that a Tool is bound to.
//
// The raw Connection UUID is never model-visible. Only a short deterministic
// opaque marker derived from its SHA-256 digest is emitted, and it is phrased
// as a server target marker rather than a connection identifier. The label is
// user-controlled and therefore sanitized, bounded, quoted and phrased as a
// label (never as instructions). A blank label falls back to the marker so
// duplicate and blank labels stay distinguishable without naming any identity.
// A malformed, empty or nil connection identity fails closed instead of
// emitting a raw or partial id. Chat, Agent and Workflow projections must not
// call this: exactly one Connection is active there.
func DigitalHumanConnectionRuntimeDescription(description, connectionID, label string) (string, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(connectionID))
	if err != nil || parsed == uuid.Nil {
		return "", errInvalidDigitalHumanConnectionIdentity
	}
	marker := digitalHumanTargetMarker(parsed)
	sentence := fmt.Sprintf("Digital Human server target marker: %s.", marker)
	if display := boundedName(cleanConnectionText(label), connectionLabelBudget); display != "" {
		sentence = fmt.Sprintf(
			"Digital Human connection label: %s (server target marker %s).",
			strconv.Quote(display),
			marker,
		)
	}
	description = strings.TrimSpace(description)
	if description == "" {
		return sentence, nil
	}
	return description + "\n\n" + sentence, nil
}

// digitalHumanTargetMarker derives a bounded, deterministic marker from the
// canonical Connection UUID. The digest is presentation-only: it is not an
// authorization, dispatch or identity-resolution input.
func digitalHumanTargetMarker(connectionID uuid.UUID) string {
	digest := sha256.Sum256([]byte(connectionID.String()))
	return hex.EncodeToString(digest[:])[:digitalHumanTargetMarkerHexLength]
}

// cleanConnectionText replaces control and bidirectional formatting characters
// with spaces and collapses whitespace. The result is untrusted data, not a
// format string, so callers must still quote it before interpolation.
func cleanConnectionText(raw string) string {
	cleaned := make([]rune, 0, len([]rune(raw)))
	for _, character := range raw {
		if unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			cleaned = append(cleaned, ' ')
			continue
		}
		cleaned = append(cleaned, character)
	}
	return strings.Join(strings.Fields(string(cleaned)), " ")
}
