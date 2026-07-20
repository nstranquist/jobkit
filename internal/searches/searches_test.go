package searches

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestResolveBoardsExpandsGroups(t *testing.T) {
	cfg := &Config{
		Boards: map[string][]string{
			"core": {"greenhouse:acme", "lever:demo"},
			"all":  {"@core", "ashby:Ashby"},
		},
	}
	boards, specs, err := cfg.ResolveBoards([]string{"@all"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(specs, []string{"greenhouse:acme", "lever:demo", "ashby:Ashby"}) {
		t.Fatalf("specs = %#v", specs)
	}
	if len(boards) != 3 || boards[2].Provider != "ashby" || boards[2].Slug != "Ashby" {
		t.Fatalf("boards = %#v", boards)
	}
}

func TestResolveBoardsExpandsTargetPacks(t *testing.T) {
	cfg := &Config{
		Targets: map[string]Target{
			"ai": {Boards: []string{"greenhouse:openai", "ashby:Cursor"}},
		},
	}
	boards, specs, err := cfg.ResolveBoards([]string{"#ai"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(specs, []string{"greenhouse:openai", "ashby:Cursor"}) {
		t.Fatalf("specs = %#v", specs)
	}
	if len(boards) != 2 || boards[1].Provider != "ashby" || boards[1].Slug != "Cursor" {
		t.Fatalf("boards = %#v", boards)
	}
}

func TestResolveBoardsRejectsUnknownTargetPack(t *testing.T) {
	cfg := &Config{}
	boards, specs, err := cfg.ResolveBoards([]string{"#missing"})
	if err == nil {
		t.Fatalf("expected unknown target error, got boards=%#v specs=%#v", boards, specs)
	}
}

func TestResolveBoardsRejectsTargetPackCycles(t *testing.T) {
	cfg := &Config{
		Targets: map[string]Target{
			"a": {Boards: []string{"#b"}},
			"b": {Boards: []string{"#a"}},
		},
	}
	boards, specs, err := cfg.ResolveBoards([]string{"#a"})
	if err == nil {
		t.Fatalf("expected target cycle error, got boards=%#v specs=%#v", boards, specs)
	}
}

func TestResolveBoardsRejectsCycles(t *testing.T) {
	cfg := &Config{Boards: map[string][]string{"a": {"@b"}, "b": {"@a"}}}
	boards, specs, err := cfg.ResolveBoards([]string{"@a"})
	if err == nil {
		t.Fatalf("expected cycle error, got boards=%#v specs=%#v", boards, specs)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "searches.yaml")
	cfg := Template()
	cfg.AddSearch("custom", Profile{Query: "go backend", Boards: []string{"@ai-infra"}, RemoteOnly: true, Limit: 10})
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Searches["custom"].Query != "go backend" {
		t.Fatalf("custom query = %q", got.Searches["custom"].Query)
	}
}
