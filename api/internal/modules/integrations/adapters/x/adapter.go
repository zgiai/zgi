package x

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

type Adapter struct {
	client *client
}

func New(httpClient *http.Client) (*Adapter, error) {
	apiClient, err := newClient(httpClient)
	if err != nil {
		return nil, err
	}
	return &Adapter{client: apiClient}, nil
}

func newForBaseURL(httpClient *http.Client, baseURL string) (*Adapter, error) {
	apiClient, err := newClientForBaseURL(httpClient, baseURL)
	if err != nil {
		return nil, err
	}
	return &Adapter{client: apiClient}, nil
}

func (adapter *Adapter) DriverID() string { return DriverID }

func (adapter *Adapter) Execute(ctx context.Context, request integrations.ActionRequest) (*integrations.ActionResult, error) {
	if isAppBearerConnection(request.Connection) && !supportsAppBearerAction(request.ActionID) {
		return nil, integrations.NewError(
			integrations.ErrorCodeAccessDenied,
			"X app-only credentials cannot perform this action",
			nil,
		)
	}
	accessToken, err := xAccessToken(request.Connection)
	if err != nil {
		return nil, err
	}
	switch request.ActionID {
	case ActionGetAccount:
		output, meta, err := adapter.getAccount(ctx, accessToken)
		return xActionResult(output, meta, 1), err
	case ActionGetUserByUsername:
		output, meta, err := adapter.getUserByUsername(ctx, accessToken, request.Input)
		return xActionResult(output, meta, 1), err
	case ActionListOwnPosts:
		output, meta, err := adapter.listOwnPosts(ctx, accessToken, request.Input)
		return xActionResult(output, meta, outputCount(output, "posts")), err
	case ActionListPostsByUser:
		output, meta, err := adapter.listPostsByUser(ctx, accessToken, request.Input)
		return xActionResult(output, meta, outputCount(output, "posts")), err
	case ActionSearchRecentPosts:
		output, meta, err := adapter.searchRecentPosts(ctx, accessToken, request.Input)
		return xActionResult(output, meta, outputCount(output, "posts")), err
	case ActionCreatePost:
		output, meta, err := adapter.createPost(ctx, accessToken, request.Input)
		return xActionResult(output, meta, 1), err
	default:
		return nil, integrations.NewError(integrations.ErrorCodeInvalidInput, "X action is not supported", nil)
	}
}

// ValidateCredentials performs only deterministic provider-specific checks
// before encryption. Network verification belongs to ValidateConnection so it
// always runs through the audited, quota-controlled connection-test pipeline.
// OAuth credentials are created by the OAuth flow and never enter this manual
// credential validation path.
func (adapter *Adapter) ValidateCredentials(_ context.Context, request integrations.CredentialValidationRequest) error {
	if !strings.EqualFold(strings.TrimSpace(request.IntegrationID), IntegrationID) ||
		!strings.EqualFold(strings.TrimSpace(request.DriverID), DriverID) ||
		!strings.EqualFold(strings.TrimSpace(request.AuthMethodID), AppBearerAuthMethodID) {
		return integrations.NewError(integrations.ErrorCodeInvalidInput, "X authentication method is unsupported", nil)
	}
	token := strings.TrimSpace(request.Credentials["bearer_token"])
	if token == "" {
		return integrations.NewError(integrations.ErrorCodeAuthInvalid, "X credentials are unavailable", nil)
	}
	return nil
}

func (adapter *Adapter) ValidateConnection(ctx context.Context, connection *integrations.ResolvedConnection) (*integrations.ConnectionProfile, error) {
	accessToken, err := xAccessToken(connection)
	if err != nil {
		return nil, err
	}
	if isAppBearerConnection(connection) {
		_, meta, validationErr := adapter.fetchPublicValidationUser(ctx, accessToken)
		if validationErr != nil {
			return nil, validationErr
		}
		return &integrations.ConnectionProfile{
			AccountID:         "app-only",
			DisplayName:       "X public data",
			GrantedScopes:     []string{ScopeUsersRead, ScopePostsRead},
			ScopeEvidence:     integrations.AuthScopeEvidenceConnectorDeclared,
			ProviderRequestID: meta.RequestID,
		}, nil
	}
	user, meta, err := adapter.fetchCurrentUser(ctx, accessToken)
	if err != nil {
		return nil, err
	}
	return &integrations.ConnectionProfile{
		AccountID: user.ID, DisplayName: firstNonEmpty(user.Name, user.Username),
		GrantedScopes:     append([]string(nil), connection.GrantedScopes...),
		ProviderRequestID: meta.RequestID,
	}, nil
}

func (adapter *Adapter) ProbeConnection(ctx context.Context, connection *integrations.ResolvedConnection) (*integrations.HealthProbeReport, error) {
	profile, err := adapter.ValidateConnection(ctx, connection)
	if err != nil {
		status := integrations.HealthProbeStatusUnhealthy
		switch integrations.ErrorCode(err) {
		case integrations.ErrorCodeTimeout, integrations.ErrorCodeUpstream, integrations.ErrorCodeRateLimited:
			status = integrations.HealthProbeStatusDegraded
		}
		return &integrations.HealthProbeReport{
			Status: status,
			Checks: []integrations.HealthProbeCheck{{
				Code: integrations.ErrorCode(err), Status: status, Message: "X connection check failed",
			}},
		}, err
	}
	return &integrations.HealthProbeReport{
		Status: integrations.HealthProbeStatusHealthy, Profile: profile,
		Checks: []integrations.HealthProbeCheck{{
			Code: xHealthCheckCode(connection), Status: integrations.HealthProbeStatusHealthy,
		}},
	}, nil
}

func (adapter *Adapter) getAccount(ctx context.Context, accessToken string) (map[string]interface{}, responseMeta, error) {
	user, meta, err := adapter.fetchCurrentUser(ctx, accessToken)
	if err != nil {
		return nil, meta, err
	}
	return map[string]interface{}{
		"provider": IntegrationID, "request_id": bounded(meta.RequestID, 128),
		"account": safeUser(user),
	}, meta, nil
}

func (adapter *Adapter) getUserByUsername(ctx context.Context, accessToken string, input map[string]interface{}) (map[string]interface{}, responseMeta, error) {
	username, err := normalizedXUsername(input)
	if err != nil {
		return nil, responseMeta{}, err
	}
	query := url.Values{
		"user.fields": []string{"id,name,username,description,created_at,profile_image_url,verified,public_metrics,url"},
	}
	var response xUserResponse
	meta, err := adapter.client.getJSON(ctx, accessToken, "/2/users/by/username/"+url.PathEscape(username), query, &response)
	if err != nil {
		return nil, meta, err
	}
	if strings.TrimSpace(response.Data.ID) == "" || strings.TrimSpace(response.Data.Username) == "" {
		return nil, meta, integrations.NewError(integrations.ErrorCodeResponseInvalid, "X user response is incomplete", nil)
	}
	return map[string]interface{}{
		"provider": IntegrationID, "request_id": bounded(meta.RequestID, 128),
		"user": safeUser(response.Data),
	}, meta, nil
}

func (adapter *Adapter) fetchCurrentUser(ctx context.Context, accessToken string) (xUser, responseMeta, error) {
	query := url.Values{"user.fields": []string{"id,name,username,description,created_at,profile_image_url,verified,public_metrics,url"}}
	var response xUserResponse
	meta, err := adapter.client.getJSON(ctx, accessToken, "/2/users/me", query, &response)
	if err != nil {
		return xUser{}, meta, err
	}
	if strings.TrimSpace(response.Data.ID) == "" || strings.TrimSpace(response.Data.Username) == "" {
		return xUser{}, meta, integrations.NewError(integrations.ErrorCodeResponseInvalid, "X identity response is incomplete", nil)
	}
	return response.Data, meta, nil
}

func (adapter *Adapter) fetchPublicValidationUser(ctx context.Context, bearerToken string) (xUser, responseMeta, error) {
	query := url.Values{"user.fields": []string{"id,name,username"}}
	var response xUserResponse
	meta, err := adapter.client.getJSON(ctx, bearerToken, "/2/users/by/username/xdevelopers", query, &response)
	if err != nil {
		return xUser{}, meta, err
	}
	if strings.TrimSpace(response.Data.ID) == "" || strings.TrimSpace(response.Data.Username) == "" {
		return xUser{}, meta, integrations.NewError(integrations.ErrorCodeResponseInvalid, "X public identity response is incomplete", nil)
	}
	return response.Data, meta, nil
}

func (adapter *Adapter) listOwnPosts(ctx context.Context, accessToken string, input map[string]interface{}) (map[string]interface{}, responseMeta, error) {
	user, userMeta, err := adapter.fetchCurrentUser(ctx, accessToken)
	if err != nil {
		return nil, userMeta, err
	}
	maxResults := inputInteger(input, "max_results", 20, 5, 100)
	query := url.Values{
		"max_results":  []string{strconv.Itoa(maxResults)},
		"tweet.fields": []string{"id,text,created_at,lang,conversation_id,possibly_sensitive,public_metrics"},
	}
	if paginationToken := bounded(inputString(input, "pagination_token"), 1024); paginationToken != "" {
		query.Set("pagination_token", paginationToken)
	}
	var response xPostsResponse
	meta, err := adapter.client.getJSON(ctx, accessToken, "/2/users/"+url.PathEscape(user.ID)+"/tweets", query, &response)
	meta.Attempts += userMeta.Attempts
	if meta.RequestID == "" {
		meta.RequestID = userMeta.RequestID
	}
	if err != nil {
		return nil, meta, err
	}
	return postsOutput(meta, response, maxResults), meta, nil
}

func (adapter *Adapter) listPostsByUser(ctx context.Context, accessToken string, input map[string]interface{}) (map[string]interface{}, responseMeta, error) {
	userID, err := requiredXUserID(input)
	if err != nil {
		return nil, responseMeta{}, err
	}
	maxResults, err := optionalXInteger(input, "max_results", 20, 5, 100)
	if err != nil {
		return nil, responseMeta{}, err
	}
	query := url.Values{
		"max_results":  []string{strconv.Itoa(maxResults)},
		"tweet.fields": []string{"id,text,created_at,lang,conversation_id,possibly_sensitive,public_metrics"},
	}
	paginationToken, err := optionalXToken(input, "pagination_token")
	if err != nil {
		return nil, responseMeta{}, err
	}
	if paginationToken != "" {
		query.Set("pagination_token", paginationToken)
	}
	var response xPostsResponse
	meta, err := adapter.client.getJSON(ctx, accessToken, "/2/users/"+url.PathEscape(userID)+"/tweets", query, &response)
	if err != nil {
		return nil, meta, err
	}
	return postsOutput(meta, response, maxResults), meta, nil
}

func (adapter *Adapter) searchRecentPosts(ctx context.Context, accessToken string, input map[string]interface{}) (map[string]interface{}, responseMeta, error) {
	queryText := strings.TrimSpace(inputString(input, "query"))
	if queryText == "" || len([]rune(queryText)) > 512 {
		return nil, responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "X search query is invalid", nil)
	}
	maxResults := inputInteger(input, "max_results", 10, 10, 100)
	query := url.Values{
		"query":        []string{queryText},
		"max_results":  []string{strconv.Itoa(maxResults)},
		"tweet.fields": []string{"id,text,created_at,lang,conversation_id,possibly_sensitive,public_metrics"},
	}
	if nextToken := bounded(inputString(input, "next_token"), 1024); nextToken != "" {
		query.Set("next_token", nextToken)
	}
	var response xPostsResponse
	meta, err := adapter.client.getJSON(ctx, accessToken, "/2/tweets/search/recent", query, &response)
	if err != nil {
		return nil, meta, err
	}
	return postsOutput(meta, response, maxResults), meta, nil
}

func (adapter *Adapter) createPost(ctx context.Context, accessToken string, input map[string]interface{}) (map[string]interface{}, responseMeta, error) {
	text := strings.TrimSpace(inputString(input, "text"))
	if text == "" || len([]rune(text)) > 280 {
		return nil, responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "X post text must contain between 1 and 280 characters", nil)
	}
	var response xCreatePostResponse
	meta, err := adapter.client.postJSON(ctx, accessToken, "/2/tweets", map[string]string{"text": text}, &response)
	if err != nil {
		return nil, meta, err
	}
	if strings.TrimSpace(response.Data.ID) == "" {
		return nil, meta, integrations.NewError(integrations.ErrorCodeResponseInvalid, "X create post response is incomplete", nil)
	}
	return map[string]interface{}{
		"provider": IntegrationID, "request_id": bounded(meta.RequestID, 128),
		"post": map[string]interface{}{"id": bounded(response.Data.ID, 128), "text": bounded(response.Data.Text, 1000)},
	}, meta, nil
}

func postsOutput(meta responseMeta, response xPostsResponse, limit int) map[string]interface{} {
	posts := make([]interface{}, 0, min(len(response.Data), limit))
	for index, post := range response.Data {
		if index >= limit {
			break
		}
		posts = append(posts, safePost(post))
	}
	return map[string]interface{}{
		"provider": IntegrationID, "request_id": bounded(meta.RequestID, 128),
		"posts": posts, "next_token": bounded(response.Meta.NextToken, 1024),
		"result_count": max(response.Meta.ResultCount, len(posts)),
	}
}

func safeUser(user xUser) map[string]interface{} {
	return map[string]interface{}{
		"id": bounded(user.ID, 128), "name": bounded(user.Name, 255), "username": bounded(user.Username, 128),
		"description": bounded(user.Description, 1000), "created_at": bounded(user.CreatedAt, 64),
		"profile_image_url": safeXURL(user.ProfileImageURL), "url": safeXURL(user.URL),
		"verified": user.Verified,
		"public_metrics": map[string]interface{}{
			"followers_count": max(user.PublicMetrics.FollowersCount, 0),
			"following_count": max(user.PublicMetrics.FollowingCount, 0),
			"tweet_count":     max(user.PublicMetrics.TweetCount, 0),
			"listed_count":    max(user.PublicMetrics.ListedCount, 0),
		},
	}
}

func safePost(post xPost) map[string]interface{} {
	return map[string]interface{}{
		"id": bounded(post.ID, 128), "text": bounded(post.Text, 1000),
		"created_at": bounded(post.CreatedAt, 64), "lang": bounded(post.Lang, 32),
		"conversation_id": bounded(post.ConversationID, 128), "possibly_sensitive": post.PossiblySensitive,
		"public_metrics": map[string]interface{}{
			"retweet_count":    max(post.PublicMetrics.RetweetCount, 0),
			"reply_count":      max(post.PublicMetrics.ReplyCount, 0),
			"like_count":       max(post.PublicMetrics.LikeCount, 0),
			"quote_count":      max(post.PublicMetrics.QuoteCount, 0),
			"bookmark_count":   max(post.PublicMetrics.BookmarkCount, 0),
			"impression_count": max(post.PublicMetrics.ImpressionCount, 0),
		},
	}
}

func xAccessToken(connection *integrations.ResolvedConnection) (string, error) {
	if connection == nil || !strings.EqualFold(connection.IntegrationID, IntegrationID) ||
		!strings.EqualFold(connection.DriverID, DriverID) {
		return "", integrations.NewError(integrations.ErrorCodeConnectionInvalid, "X connection is invalid", nil)
	}
	credentialKey := "access_token"
	if isAppBearerConnection(connection) {
		credentialKey = "bearer_token"
	}
	token := strings.TrimSpace(connection.Credentials[credentialKey])
	if token == "" {
		return "", integrations.NewError(integrations.ErrorCodeAuthInvalid, "X credentials are unavailable", nil)
	}
	return token, nil
}

func isAppBearerConnection(connection *integrations.ResolvedConnection) bool {
	return connection != nil && strings.EqualFold(strings.TrimSpace(connection.AuthMethodID), AppBearerAuthMethodID)
}

func supportsAppBearerAction(actionID string) bool {
	switch actionID {
	case ActionGetUserByUsername, ActionListPostsByUser, ActionSearchRecentPosts:
		return true
	default:
		return false
	}
}

func xHealthCheckCode(connection *integrations.ResolvedConnection) string {
	if isAppBearerConnection(connection) {
		return "x_app_bearer_public_read"
	}
	return "x_authenticated_user"
}

func xActionResult(output map[string]interface{}, meta responseMeta, count int) *integrations.ActionResult {
	if output == nil && meta.Attempts == 0 && meta.RequestID == "" &&
		meta.Diagnostics == (integrations.ProviderDiagnostics{}) {
		return nil
	}
	attempts := meta.Attempts
	if output != nil && attempts == 0 {
		attempts = 1
	}
	if output == nil {
		count = 0
	}
	return &integrations.ActionResult{
		Output: output, ProviderRequestID: bounded(meta.RequestID, 128),
		ProviderDiagnostics: meta.Diagnostics,
		ResultCount:         max(count, 0),
		AttemptCount:        attempts,
	}
}

func outputCount(output map[string]interface{}, key string) int {
	if values, ok := output[key].([]interface{}); ok {
		return len(values)
	}
	return 0
}

type xUserResponse struct {
	Data xUser `json:"data"`
}

type xUser struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Username        string         `json:"username"`
	Description     string         `json:"description"`
	CreatedAt       string         `json:"created_at"`
	ProfileImageURL string         `json:"profile_image_url"`
	URL             string         `json:"url"`
	Verified        bool           `json:"verified"`
	PublicMetrics   xPublicMetrics `json:"public_metrics"`
}

type xPostsResponse struct {
	Data []xPost `json:"data"`
	Meta struct {
		NextToken   string `json:"next_token"`
		ResultCount int    `json:"result_count"`
	} `json:"meta"`
}

type xPost struct {
	ID                string         `json:"id"`
	Text              string         `json:"text"`
	CreatedAt         string         `json:"created_at"`
	Lang              string         `json:"lang"`
	ConversationID    string         `json:"conversation_id"`
	PossiblySensitive bool           `json:"possibly_sensitive"`
	PublicMetrics     xPublicMetrics `json:"public_metrics"`
}

type xPublicMetrics struct {
	FollowersCount  int `json:"followers_count"`
	FollowingCount  int `json:"following_count"`
	TweetCount      int `json:"tweet_count"`
	ListedCount     int `json:"listed_count"`
	RetweetCount    int `json:"retweet_count"`
	ReplyCount      int `json:"reply_count"`
	LikeCount       int `json:"like_count"`
	QuoteCount      int `json:"quote_count"`
	BookmarkCount   int `json:"bookmark_count"`
	ImpressionCount int `json:"impression_count"`
}

type xCreatePostResponse struct {
	Data struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	} `json:"data"`
}

func bounded(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}

func safeXURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.User != nil {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "x.com" && !strings.HasSuffix(host, ".x.com") &&
		host != "twimg.com" && !strings.HasSuffix(host, ".twimg.com") {
		return ""
	}
	parsed.Fragment = ""
	return bounded(parsed.String(), 2048)
}

func inputString(input map[string]interface{}, key string) string {
	value, _ := input[key].(string)
	return value
}

func inputInteger(input map[string]interface{}, key string, fallback, minimum, maximum int) int {
	value := fallback
	switch typed := input[key].(type) {
	case int:
		value = typed
	case int64:
		value = int(typed)
	case float64:
		if typed == float64(int(typed)) {
			value = int(typed)
		}
	case json.Number:
		if parsed, err := strconv.Atoi(string(typed)); err == nil {
			value = parsed
		}
	}
	return min(max(value, minimum), maximum)
}

func normalizedXUsername(input map[string]interface{}) (string, error) {
	raw, ok := input["username"].(string)
	if !ok || raw == "" || raw != strings.TrimSpace(raw) {
		return "", integrations.NewError(integrations.ErrorCodeInvalidInput, "X username is invalid", nil)
	}
	username := strings.TrimPrefix(raw, "@")
	if username == "" || len(username) > 15 {
		return "", integrations.NewError(integrations.ErrorCodeInvalidInput, "X username is invalid", nil)
	}
	for _, char := range username {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '_' {
			continue
		}
		return "", integrations.NewError(integrations.ErrorCodeInvalidInput, "X username is invalid", nil)
	}
	return username, nil
}

func requiredXUserID(input map[string]interface{}) (string, error) {
	value, ok := input["user_id"].(string)
	if !ok || value == "" || value != strings.TrimSpace(value) || len(value) > 32 {
		return "", integrations.NewError(integrations.ErrorCodeInvalidInput, "X user ID is invalid", nil)
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return "", integrations.NewError(integrations.ErrorCodeInvalidInput, "X user ID is invalid", nil)
		}
	}
	return value, nil
}

func optionalXInteger(input map[string]interface{}, key string, fallback, minimum, maximum int) (int, error) {
	raw, exists := input[key]
	if !exists {
		return fallback, nil
	}
	value := 0
	valid := true
	switch typed := raw.(type) {
	case int:
		value = typed
	case int64:
		if typed < int64(minimum) || typed > int64(maximum) {
			valid = false
		} else {
			value = int(typed)
		}
	case float64:
		if typed < float64(minimum) || typed > float64(maximum) || typed != float64(int(typed)) {
			valid = false
		} else {
			value = int(typed)
		}
	case json.Number:
		parsed, err := strconv.Atoi(string(typed))
		valid = err == nil
		value = parsed
	default:
		valid = false
	}
	if !valid || value < minimum || value > maximum {
		return 0, integrations.NewError(integrations.ErrorCodeInvalidInput, "X "+key+" is outside the allowed range", nil)
	}
	return value, nil
}

func optionalXToken(input map[string]interface{}, key string) (string, error) {
	raw, exists := input[key]
	if !exists {
		return "", nil
	}
	value, ok := raw.(string)
	if !ok || value == "" || value != strings.TrimSpace(value) || len([]rune(value)) > 1024 {
		return "", integrations.NewError(integrations.ErrorCodeInvalidInput, "X "+key+" is invalid", nil)
	}
	return value, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

var _ integrations.Adapter = (*Adapter)(nil)
var _ integrations.ConnectionTester = (*Adapter)(nil)
var _ integrations.HealthProbe = (*Adapter)(nil)
