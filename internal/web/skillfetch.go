package web

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"
)

const maxFetchBytes = 1 << 20

var githubBlobPattern = regexp.MustCompile(`^https://github\.com/([^/]+)/([^/]+)/blob/(.+)$`)

func normalizeFetchURL(raw string) string {
	if m := githubBlobPattern.FindStringSubmatch(raw); m != nil {
		return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s", m[1], m[2], m[3])
	}
	return raw
}

func fetchSkillContent(rawURL string) (string, error) {
	url := normalizeFetchURL(rawURL)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBytes+1))
	if err != nil {
		return "", err
	}
	if len(body) > maxFetchBytes {
		return "", fmt.Errorf("%s: response larger than %d bytes", url, maxFetchBytes)
	}
	return string(body), nil
}
