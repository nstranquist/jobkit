package jobsearch

import (
	"context"
	"net/http"
	"strings"
)

// BoardProvider is one public ATS client. A new board is a new provider,
// not a switch in find.
type BoardProvider interface {
	ID() string
	Aliases() []string
	Detect(host string, pathParts []string) (slug string, ok bool)
	Fetch(ctx context.Context, client *http.Client, board Board) ([]Job, error)
}

type greenhouseProvider struct{}

func (greenhouseProvider) ID() string        { return "greenhouse" }
func (greenhouseProvider) Aliases() []string { return []string{"gh"} }
func (greenhouseProvider) Detect(host string, parts []string) (string, bool) {
	if !strings.Contains(host, "greenhouse.io") || len(parts) == 0 {
		return "", false
	}
	return parts[0], true
}
func (greenhouseProvider) Fetch(ctx context.Context, client *http.Client, board Board) ([]Job, error) {
	return fetchGreenhouse(ctx, client, board)
}

type leverProvider struct{}

func (leverProvider) ID() string        { return "lever" }
func (leverProvider) Aliases() []string { return nil }
func (leverProvider) Detect(host string, parts []string) (string, bool) {
	if (host != "jobs.lever.co" && host != "jobs.eu.lever.co") || len(parts) == 0 {
		return "", false
	}
	return parts[0], true
}
func (leverProvider) Fetch(ctx context.Context, client *http.Client, board Board) ([]Job, error) {
	return fetchLever(ctx, client, board)
}

type ashbyProvider struct{}

func (ashbyProvider) ID() string        { return "ashby" }
func (ashbyProvider) Aliases() []string { return nil }
func (ashbyProvider) Detect(host string, parts []string) (string, bool) {
	if host != "jobs.ashbyhq.com" || len(parts) == 0 {
		return "", false
	}
	return parts[0], true
}
func (ashbyProvider) Fetch(ctx context.Context, client *http.Client, board Board) ([]Job, error) {
	return fetchAshby(ctx, client, board)
}

var boardRegistry = []BoardProvider{
	greenhouseProvider{},
	leverProvider{},
	ashbyProvider{},
}

// Providers returns the built-in detect/fetch registry in stable order.
func Providers() []BoardProvider {
	out := make([]BoardProvider, len(boardRegistry))
	copy(out, boardRegistry)
	return out
}

// LookupProvider resolves a provider id or alias.
func LookupProvider(id string) (BoardProvider, bool) {
	want := strings.ToLower(strings.TrimSpace(id))
	if want == "" {
		return nil, false
	}
	for _, p := range boardRegistry {
		if p.ID() == want {
			return p, true
		}
		for _, alias := range p.Aliases() {
			if alias == want {
				return p, true
			}
		}
	}
	return nil, false
}
