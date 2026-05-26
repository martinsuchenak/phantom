package commands

import (
	"testing"

	"github.com/martinsuchenak/phantom/internal/config"
)

func TestServedRepos_AllServed(t *testing.T) {
	c := config.DefaultConfig()
	c.Projects = map[string]config.Project{
		"app1": {Path: "/srv/app1", Serve: true},
		"app2": {Path: "/srv/app2", Serve: true},
	}
	repos, names := servedRepos(c)
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}
	if repos["app1"] != "/srv/app1" {
		t.Errorf("app1: expected /srv/app1, got %q", repos["app1"])
	}
	if repos["app2"] != "/srv/app2" {
		t.Errorf("app2: expected /srv/app2, got %q", repos["app2"])
	}
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	if !nameSet["app1"] || !nameSet["app2"] {
		t.Errorf("expected names to contain app1 and app2, got %v", names)
	}
}

func TestServedRepos_SomeServed(t *testing.T) {
	c := config.DefaultConfig()
	c.Projects = map[string]config.Project{
		"served":     {Path: "/srv/served", Serve: true},
		"not-served": {Path: "/srv/not-served", Serve: false},
		"no-flag":    {Path: "/srv/no-flag"},
	}
	repos, names := servedRepos(c)
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d: %v", len(repos), repos)
	}
	if repos["served"] != "/srv/served" {
		t.Errorf("served: expected /srv/served, got %q", repos["served"])
	}
	if len(names) != 1 || names[0] != "served" {
		t.Errorf("expected names [served], got %v", names)
	}
}

func TestServedRepos_NoneServed(t *testing.T) {
	c := config.DefaultConfig()
	c.Projects = map[string]config.Project{
		"app1": {Path: "/srv/app1", Serve: false},
		"app2": {Path: "/srv/app2"},
	}
	repos, names := servedRepos(c)
	if len(repos) != 0 {
		t.Errorf("expected empty repos, got %v", repos)
	}
	if len(names) != 0 {
		t.Errorf("expected empty names, got %v", names)
	}
}

func TestServedRepos_EmptyProjects(t *testing.T) {
	c := config.DefaultConfig()
	repos, names := servedRepos(c)
	if len(repos) != 0 {
		t.Errorf("expected empty repos, got %v", repos)
	}
	if len(names) != 0 {
		t.Errorf("expected empty names, got %v", names)
	}
}
