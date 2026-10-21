package catalog

import (
	"errors"
	"net/url"
	"sort"
	"strings"
)

// ConnectorCategory is a curated product-purpose category. It is presentation
// metadata and deliberately separate from Classification (connector/data_source)
// and CapabilityType (action/data).
type ConnectorCategory string

const (
	ConnectorCategoryCommunication ConnectorCategory = "communication"
	ConnectorCategoryProductivity  ConnectorCategory = "productivity"
	ConnectorCategoryInformation   ConnectorCategory = "information"
	ConnectorCategoryDeveloper     ConnectorCategory = "developer"
	ConnectorCategoryOther         ConnectorCategory = "other"
)

func (category ConnectorCategory) Valid() bool {
	switch category {
	case ConnectorCategoryCommunication, ConnectorCategoryProductivity,
		ConnectorCategoryInformation, ConnectorCategoryDeveloper, ConnectorCategoryOther:
		return true
	default:
		return false
	}
}

// ConnectorTag is a curated cross-category label. Tags never grant a
// capability or influence runtime authorization.
type ConnectorTag string

const (
	ConnectorTagSocial        ConnectorTag = "social"
	ConnectorTagMessaging     ConnectorTag = "messaging"
	ConnectorTagEmail         ConnectorTag = "email"
	ConnectorTagEnterprise    ConnectorTag = "enterprise"
	ConnectorTagCollaboration ConnectorTag = "collaboration"
	ConnectorTagDocuments     ConnectorTag = "documents"
	ConnectorTagTables        ConnectorTag = "tables"
	ConnectorTagWorkflow      ConnectorTag = "workflow"
	ConnectorTagSearch        ConnectorTag = "search"
	ConnectorTagWeb           ConnectorTag = "web"
	ConnectorTagResearch      ConnectorTag = "research"
	ConnectorTagCode          ConnectorTag = "code"
	ConnectorTagRepository    ConnectorTag = "repository"
	ConnectorTagIssues        ConnectorTag = "issues"
	ConnectorTagIntegration   ConnectorTag = "integration"
)

func (tag ConnectorTag) Valid() bool {
	switch tag {
	case ConnectorTagSocial, ConnectorTagMessaging, ConnectorTagEmail,
		ConnectorTagEnterprise, ConnectorTagCollaboration, ConnectorTagDocuments,
		ConnectorTagTables, ConnectorTagWorkflow, ConnectorTagSearch, ConnectorTagWeb,
		ConnectorTagResearch, ConnectorTagCode, ConnectorTagRepository, ConnectorTagIssues,
		ConnectorTagIntegration:
		return true
	default:
		return false
	}
}

// PresentationMetadata is platform-owned display metadata. Evidence is kept
// private so public Catalog responses expose only the safe projection.
type PresentationMetadata struct {
	ConnectorID string
	Category    ConnectorCategory
	Tags        []ConnectorTag
	SourceURLs  []string
	Publisher   string
	ReviewedAt  string
	Rationale   string
}

// ConnectorPresentation is the public, credential-free Catalog projection.
type ConnectorPresentation struct {
	Category ConnectorCategory `json:"category"`
	Tags     []ConnectorTag    `json:"tags,omitempty"`
}

var connectorPresentationRegistry = map[string]PresentationMetadata{
	"weixin": {
		ConnectorID: "weixin", Category: ConnectorCategoryCommunication,
		Tags:       []ConnectorTag{ConnectorTagSocial, ConnectorTagMessaging},
		SourceURLs: []string{"https://www.tencent.com/zh-cn/products/weixin-wechat/"}, Publisher: "Tencent", ReviewedAt: "2026-09-12",
		Rationale: "Tencent describes Weixin as a communication and social platform; the ZGI contract reads direct messages and replies.",
	},
	"wecom": {
		ConnectorID: "wecom", Category: ConnectorCategoryCommunication,
		Tags:       []ConnectorTag{ConnectorTagEnterprise, ConnectorTagMessaging, ConnectorTagCollaboration},
		SourceURLs: []string{"https://www.tencent.com/zh-cn/products/wecom/"}, Publisher: "Tencent", ReviewedAt: "2026-09-12",
		Rationale: "Tencent describes WeCom as enterprise communication and office software; the ZGI contract centers on bot messages and sessions.",
	},
	"dingtalk": {
		ConnectorID: "dingtalk", Category: ConnectorCategoryProductivity,
		Tags:       []ConnectorTag{ConnectorTagEnterprise, ConnectorTagMessaging, ConnectorTagCollaboration, ConnectorTagWorkflow},
		SourceURLs: []string{"https://www.dingtalk.com/", "https://docs.dingtalk.com/"}, Publisher: "DingTalk", ReviewedAt: "2026-09-12",
		Rationale: "DingTalk presents an enterprise work platform with communication, document, collaboration and workflow scenarios.",
	},
	"feishu": {
		ConnectorID: "feishu", Category: ConnectorCategoryProductivity,
		Tags:       []ConnectorTag{ConnectorTagEnterprise, ConnectorTagMessaging, ConnectorTagCollaboration, ConnectorTagDocuments, ConnectorTagTables},
		SourceURLs: []string{"https://docs.feishu.cn/docs"}, Publisher: "Feishu", ReviewedAt: "2026-09-12",
		Rationale: "Feishu documentation highlights real-time collaboration, comments, documents, tables and organized data.",
	},
	"gmail": {
		ConnectorID: "gmail", Category: ConnectorCategoryCommunication,
		Tags:       []ConnectorTag{ConnectorTagEmail},
		SourceURLs: []string{"https://support.google.com/a/users/answer/9259748"}, Publisher: "Google", ReviewedAt: "2026-09-12",
		Rationale: "Google describes Gmail as a work communication hub; the ZGI contract exposes mailbox operations.",
	},
	"qqmail": {
		ConnectorID: "qqmail", Category: ConnectorCategoryCommunication,
		Tags:       []ConnectorTag{ConnectorTagEmail},
		SourceURLs: []string{"https://help.mail.qq.com/detail/106/985"}, Publisher: "Tencent", ReviewedAt: "2026-09-12",
		Rationale: "QQ Mail official help centers on sending, receiving and third-party mail protocols.",
	},
	"netease163": {
		ConnectorID: "netease163", Category: ConnectorCategoryCommunication,
		Tags:       []ConnectorTag{ConnectorTagEmail},
		SourceURLs: []string{"https://help.mail.126.com/faqDetail.do?code=d7a5dc8471cd0c0e8b4b8f4f8e49998b374173cfe9171305fa1ce630d7f67ac22532c23a1d70ca26"}, Publisher: "NetEase", ReviewedAt: "2026-09-12",
		Rationale: "NetEase official help centers on mailbox client protocols and mail send/receive.",
	},
	"github": {
		ConnectorID: "github", Category: ConnectorCategoryDeveloper,
		Tags:       []ConnectorTag{ConnectorTagCode, ConnectorTagRepository, ConnectorTagIssues, ConnectorTagCollaboration},
		SourceURLs: []string{"https://docs.github.com/en/get-started/start-your-journey/what-is-github"}, Publisher: "GitHub", ReviewedAt: "2026-09-12",
		Rationale: "GitHub documents code hosting, repositories, issues, review, project planning and software delivery collaboration.",
	},
	"github-mcp": {
		ConnectorID: "github-mcp", Category: ConnectorCategoryDeveloper,
		Tags:       []ConnectorTag{ConnectorTagCode, ConnectorTagRepository, ConnectorTagIssues},
		SourceURLs: []string{"https://github.com/github/github-mcp-server"}, Publisher: "GitHub", ReviewedAt: "2026-10-08",
		Rationale: "GitHub's official remote MCP server exposes read-only issue discovery and retrieval tools.",
	},
	"web-search": {
		ConnectorID: "web-search", Category: ConnectorCategoryInformation,
		Tags:       []ConnectorTag{ConnectorTagSearch, ConnectorTagWeb, ConnectorTagResearch},
		SourceURLs: []string{"https://exa.ai/docs/reference/search"}, Publisher: "Exa", ReviewedAt: "2026-09-12",
		Rationale: "Exa's official Search API searches the web and extracts result content for information retrieval.",
	},
}

var errInvalidPresentationMetadata = errors.New("invalid connector presentation metadata")

// ValidatePresentationMetadata checks platform-owned registry records without
// touching Connector definition validation or contract digests.
func ValidatePresentationMetadata(metadata PresentationMetadata) error {
	if strings.TrimSpace(metadata.ConnectorID) == "" || !metadata.Category.Valid() {
		return errInvalidPresentationMetadata
	}
	seen := make(map[ConnectorTag]struct{}, len(metadata.Tags))
	for _, tag := range metadata.Tags {
		if !tag.Valid() {
			return errInvalidPresentationMetadata
		}
		if _, ok := seen[tag]; ok {
			return errInvalidPresentationMetadata
		}
		seen[tag] = struct{}{}
	}
	if metadata.Category != ConnectorCategoryOther {
		if strings.TrimSpace(metadata.Publisher) == "" || strings.TrimSpace(metadata.ReviewedAt) == "" || strings.TrimSpace(metadata.Rationale) == "" || len(metadata.SourceURLs) == 0 {
			return errInvalidPresentationMetadata
		}
	}
	for _, raw := range metadata.SourceURLs {
		u, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
			return errInvalidPresentationMetadata
		}
	}
	return nil
}

// PresentationFor returns a defensive public projection and whether the ID is
// covered by the reviewed registry. Unknown IDs use a conservative category.
func PresentationFor(connectorID string) (ConnectorPresentation, bool) {
	metadata, ok := connectorPresentationRegistry[connectorID]
	if !ok {
		return ConnectorPresentation{Category: ConnectorCategoryOther}, false
	}
	tags := append([]ConnectorTag(nil), metadata.Tags...)
	return ConnectorPresentation{Category: metadata.Category, Tags: tags}, true
}

// PresentationMetadataFor is used by tests and maintenance tooling to inspect
// the private evidence record without exposing it through the API.
func PresentationMetadataFor(connectorID string) (PresentationMetadata, bool) {
	metadata, ok := connectorPresentationRegistry[connectorID]
	if !ok {
		return PresentationMetadata{}, false
	}
	metadata.Tags = append([]ConnectorTag(nil), metadata.Tags...)
	metadata.SourceURLs = append([]string(nil), metadata.SourceURLs...)
	return metadata, true
}

// ValidatePresentationRegistry verifies every checked-in evidence record.
func ValidatePresentationRegistry() error {
	ids := make([]string, 0, len(connectorPresentationRegistry))
	for id, metadata := range connectorPresentationRegistry {
		if id != metadata.ConnectorID || ValidatePresentationMetadata(metadata) != nil {
			return errInvalidPresentationMetadata
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return nil
}
