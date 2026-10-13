package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/semver"
)

// DefaultReleasesAPI is where the hub learns about releases.
const DefaultReleasesAPI = "https://api.github.com/repos/cowlhq/lotse/releases/latest"

const releaseCheckEvery = 6 * time.Hour

// Release is a released version.
type Release struct {
	Version     string
	Notes       string
	URL         string
	PublishedAt int64
}

// releases knows the newest release, looking every few hours.
type releases struct {
	api    string
	client *http.Client

	mu        sync.Mutex
	latest    *Release
	checkedAt int64
	checkErr  string
}

func (r *releases) get() (latest *Release, checkedAt int64, checkErr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.latest, r.checkedAt, r.checkErr
}

// check asks for the newest release now.
func (r *releases) check(ctx context.Context) {
	rel, err := r.fetch(ctx)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checkedAt = time.Now().Unix()
	if err != nil {
		r.checkErr = err.Error()
		return
	}
	r.latest, r.checkErr = rel, ""
}

func (r *releases) fetch(ctx context.Context) (*Release, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.api, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	client := r.client
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("looking for releases: %s", res.Status)
	}
	var gh struct {
		TagName     string    `json:"tag_name"`
		Body        string    `json:"body"`
		HTMLURL     string    `json:"html_url"`
		PublishedAt time.Time `json:"published_at"`
	}
	if err := json.NewDecoder(res.Body).Decode(&gh); err != nil {
		return nil, err
	}
	if !semver.IsValid(gh.TagName) {
		return nil, fmt.Errorf("the newest release has no version: %q", gh.TagName)
	}
	return &Release{
		Version: strings.TrimPrefix(gh.TagName, "v"), Notes: gh.Body, URL: gh.HTMLURL,
		PublishedAt: gh.PublishedAt.Unix(),
	}, nil
}

// watch looks now and every few hours until ctx ends.
func (r *releases) watch(ctx context.Context) {
	for {
		r.check(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(releaseCheckEvery):
		}
	}
}

// newer says whether version a is newer than b. Development builds are
// neither newer nor older than anything.
func newer(a, b string) bool {
	a, b = "v"+strings.TrimPrefix(a, "v"), "v"+strings.TrimPrefix(b, "v")
	return semver.IsValid(a) && semver.IsValid(b) && semver.Compare(a, b) > 0
}
