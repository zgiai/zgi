package gateway

import (
	"errors"
	"net/url"
	"strings"

	appconfig "github.com/zgiai/zgi/api/config"
)

const officialRouteInternalPath = "/v1/internal"

func resolveOfficialRouteBaseURL() (string, error) {
	consoleBaseURL := strings.TrimSpace(appconfig.Current().Console.APIURL)
	if consoleBaseURL == "" {
		return "", errors.New("console api url is required for official cloud routes")
	}

	parsed, err := url.Parse(consoleBaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("invalid console api url for official cloud routes")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("invalid console api url for official cloud routes")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("invalid console api url for official cloud routes")
	}

	return strings.TrimRight(consoleBaseURL, "/") + officialRouteInternalPath, nil
}
