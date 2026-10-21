package agentexecution

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
)

const ReleaseContractVersion = 1
const ReleaseHeader = "X-ZGI-Runtime-Release"

var releaseIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
var releaseDigestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

// ReleaseManifest is an operator-owned artifact, not a user Agent setting.
// API/Web image pins are deployment evidence: this process verifies only its
// own binary, effective configuration and protocol, not remote containers.
type ReleaseManifest struct {
	Version                 int    `json:"version"`
	ID                      string `json:"id"`
	APIImage                string `json:"api_image"`
	WebImage                string `json:"web_image"`
	WorkerDigest            string `json:"worker_digest"`
	ConfigurationDigest     string `json:"configuration_digest"`
	PlatformProtocolVersion int    `json:"platform_protocol_version"`
}

func ReleaseDigest(reader io.Reader) (string, error) {
	hash := sha256.New()
	if _, err := io.Copy(hash, reader); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func VerifyRelease(reader io.Reader, workerDigest, configurationDigest string) (ReleaseManifest, error) {
	data, err := io.ReadAll(io.LimitReader(reader, 65537))
	if err != nil || len(data) > 65536 {
		return ReleaseManifest{}, fmt.Errorf("invalid or oversized runtime release manifest")
	}
	var manifest ReleaseManifest
	// Disallow unknown fields so a misspelled pin cannot silently be ignored.
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return ReleaseManifest{}, fmt.Errorf("invalid runtime release manifest")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return ReleaseManifest{}, fmt.Errorf("trailing runtime release manifest data")
	}
	if manifest.Version != ReleaseContractVersion || !releaseIDPattern.MatchString(manifest.ID) ||
		!immutableReleaseImage(manifest.APIImage) || !immutableReleaseImage(manifest.WebImage) ||
		!releaseDigestPattern.MatchString(manifest.WorkerDigest) || !releaseDigestPattern.MatchString(manifest.ConfigurationDigest) ||
		manifest.PlatformProtocolVersion != PlatformProtocolVersion {
		return ReleaseManifest{}, fmt.Errorf("incomplete or incompatible runtime release manifest")
	}
	if workerDigest != manifest.WorkerDigest || configurationDigest != manifest.ConfigurationDigest {
		return ReleaseManifest{}, fmt.Errorf("runtime release binary or configuration mismatch")
	}
	return manifest, nil
}

func immutableReleaseImage(value string) bool {
	index := strings.LastIndex(value, "@")
	return index > 0 && !strings.ContainsAny(value, " \t\r\n") && releaseDigestPattern.MatchString(value[index+1:])
}
