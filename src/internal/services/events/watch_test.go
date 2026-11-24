package events

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// StartWatch notices a write under a watched tree after the debounce, and
// a file created in a subdirectory that did not exist when the watch started.
func TestStartWatchSeesAWriteAndANewSubdir(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "profiles")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	got := make(chan struct{}, 8)
	mode := StartWatch(ctx, []Tracked{{Path: root, Area: AreaSSH, Dir: true}}, 50*time.Millisecond, func() {
		select {
		case got <- struct{}{}:
		default:
		}
	})
	if mode != ModeWatching {
		t.Fatalf("mode = %q, want %s", mode, ModeWatching)
	}
	if err := os.WriteFile(filepath.Join(sub, "key.pub"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitWatch(t, got, "the first write")
	drain(got)
	nested := filepath.Join(sub, "personal")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	// Give the watch a moment to add the new directory before the write.
	time.Sleep(20 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(nested, "id.pub"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitWatch(t, got, "the write in the new subdirectory")
}

func TestStartWatchReturnsPollingWhenThereIsNothingToWatch(t *testing.T) {
	if mode := StartWatch(context.Background(), nil, time.Millisecond, func() {}); mode != ModePolling {
		t.Errorf("empty roots: mode = %q, want %s", mode, ModePolling)
	}
}

func TestExtraRootsNameGitconfigAndDockerContexts(t *testing.T) {
	p, _ := home(t)
	home := filepath.Dir(p.SSHDir)
	want := []string{
		filepath.Join(home, ".gitconfig"),
		filepath.Join(home, ".config", "git"),
		filepath.Join(home, ".docker", "contexts"),
	}
	got := ExtraRoots(p)
	if len(got) != len(want) {
		t.Fatalf("ExtraRoots = %+v, want %v", got, want)
	}
	for i, r := range got {
		if r.Path != want[i] {
			t.Errorf("ExtraRoots[%d].Path = %q, want %q", i, r.Path, want[i])
		}
	}
}

func waitWatch(t *testing.T, got <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-got:
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func drain(got <-chan struct{}) {
	for {
		select {
		case <-got:
		default:
			return
		}
	}
}

func TestIncludeRootsNamesDirectoriesOutsideSSHDir(t *testing.T) {
	p, _ := home(t)
	outside := filepath.Join(t.TempDir(), "orb", "ssh")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "config"), []byte("Host orb\n  HostName 127.0.0.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(p.SSHDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.SSHDir, "config"), []byte("Include "+outside+"/config\nHost x\n  HostName x.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := IncludeRoots(p)
	found := false
	for _, r := range got {
		if r.Path == outside && r.Dir {
			found = true
		}
		if r.Path == p.SSHDir || (len(r.Path) >= len(p.SSHDir) && r.Path[:len(p.SSHDir)] == p.SSHDir) {
			t.Errorf("IncludeRoots should omit paths under ~/.ssh, got %s", r.Path)
		}
	}
	if !found {
		t.Fatalf("IncludeRoots = %+v, want %s", got, outside)
	}
}
