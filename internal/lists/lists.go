// Package lists downloads block lists from URLs or reads them from files.
package lists

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/zihado/hole/internal/blocklist"
)

// maxListBytes caps a single list download to protect memory.
const maxListBytes = 64 << 20

// DefaultSources are well-maintained public block lists.
var DefaultSources = []string{
	"https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts",
}

// Fetcher loads block lists.
type Fetcher struct {
	client *http.Client
}

// NewFetcher creates a Fetcher using client for HTTP sources.
func NewFetcher(client *http.Client) *Fetcher {
	return &Fetcher{client: client}
}

// Fetch loads and parses one source: an http(s) URL or a local file path.
func (f *Fetcher) Fetch(ctx context.Context, source string) ([]string, error) {
	rc, err := f.open(ctx, source)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return blocklist.Parse(io.LimitReader(rc, maxListBytes)), nil
}

func (f *Fetcher) open(ctx context.Context, source string) (io.ReadCloser, error) {
	if !strings.HasPrefix(source, "http://") && !strings.HasPrefix(source, "https://") {
		file, err := os.Open(source)
		if err != nil {
			return nil, fmt.Errorf("open list %s: %w", source, err)
		}
		return file, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, fmt.Errorf("build request %s: %w", source, err)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", source, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("download %s: unexpected status %s", source, resp.Status)
	}
	return resp.Body, nil
}

// FetchAll loads every source, returning the combined domains and one error
// per failed source. A failing source does not abort the others.
func (f *Fetcher) FetchAll(ctx context.Context, sources []string) ([]string, []error) {
	var all []string
	var errs []error
	for _, src := range sources {
		domains, err := f.Fetch(ctx, src)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		all = append(all, domains...)
	}
	return all, errs
}
