package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The dev container's default base image is pinned by tag and digest (owner,
// 2026-11-16: "Pin tag+digest, deprecate override"), so a rebuild gets the bytes
// that were reviewed and Dependabot's docker updater can move both together.
// AUTHAMA_GO_VERSION still picks an unpinned golang:<version> base, through a
// stage compose selects only when the variable is set: deprecated in place.

// dockerfile is the parsed shape of a Dockerfile that matters here: the ARGs
// declared before the first FROM, with their defaults, and each stage's FROM.
type dockerfile struct {
	args   map[string]string
	stages []dockerStage
}

type dockerStage struct{ from, name string }

var (
	argLine  = regexp.MustCompile(`^ARG\s+([A-Za-z_][A-Za-z0-9_]*)(?:=(\S*))?\s*$`)
	fromLine = regexp.MustCompile(`^FROM\s+(?:--\S+\s+)*(\S+)(?:\s+AS\s+(\S+))?\s*$`)
	argRef   = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}|\$([A-Za-z_][A-Za-z0-9_]*)`)
	pinned   = regexp.MustCompile(`^[a-z0-9./_-]+:[A-Za-z0-9._-]+@sha256:[0-9a-f]{64}$`)
)

func parseDockerfile(t *testing.T, text string) dockerfile {
	t.Helper()
	d := dockerfile{args: map[string]string{}}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if m := fromLine.FindStringSubmatch(line); m != nil {
			d.stages = append(d.stages, dockerStage{from: m[1], name: m[2]})
			continue
		}
		if m := argLine.FindStringSubmatch(line); m != nil && len(d.stages) == 0 {
			d.args[m[1]] = m[2]
		}
	}
	return d
}

// baseImage follows the last stage's FROM, through any stage it names, to the
// image the build starts from, with the global ARGs set to args over their
// defaults, as `docker build --build-arg` would.
func (d dockerfile) baseImage(t *testing.T, args map[string]string) string {
	t.Helper()
	if len(d.stages) == 0 {
		t.Fatal("the Dockerfile has no FROM")
	}
	expand := func(s string) string {
		return argRef.ReplaceAllStringFunc(s, func(ref string) string {
			name := strings.Trim(ref, "${}")
			if v, ok := args[name]; ok {
				return v
			}
			return d.args[name]
		})
	}
	ref := expand(d.stages[len(d.stages)-1].from)
	for hops := 0; hops < len(d.stages); hops++ {
		next := ""
		for _, s := range d.stages {
			if s.name != "" && strings.EqualFold(s.name, ref) {
				next = expand(s.from)
			}
		}
		if next == "" {
			return ref
		}
		ref = next
	}
	t.Fatalf("the stages name each other in a loop at %q", ref)
	return ""
}

// composeArgs reads the build args compose.yaml passes, interpolated as Compose
// does for ${VAR:-default} and ${VAR:+alternate}, with env as the environment.
func composeArgs(t *testing.T, text string, env map[string]string) map[string]string {
	t.Helper()
	interp := regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:-|:\+)([^}]*)\}`)
	out := map[string]string{}
	inArgs := false
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "args:" {
			inArgs = true
			continue
		}
		if !inArgs || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, val, ok := strings.Cut(trimmed, ":")
		if !ok || !strings.HasPrefix(line, "        ") {
			break
		}
		out[key] = interp.ReplaceAllStringFunc(strings.TrimSpace(val), func(m string) string {
			p := interp.FindStringSubmatch(m)
			v := env[p[1]]
			if p[2] == ":-" {
				if v == "" {
					return p[3]
				}
				return v
			}
			if v != "" {
				return p[3]
			}
			return ""
		})
	}
	if len(out) == 0 {
		t.Fatal("compose.yaml passes no build args: the parser no longer finds them")
	}
	return out
}

func readRepoFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "..", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestTheDevContainerBaseIsPinnedByDigest(t *testing.T) {
	d := parseDockerfile(t, readRepoFile(t, "Dockerfile"))
	if len(d.stages) < 2 {
		t.Fatalf("parsed %d FROM lines; the pinned base and the deprecated override need a stage each", len(d.stages))
	}
	// `docker build .` with no build args.
	if img := d.baseImage(t, nil); !pinned.MatchString(img) {
		t.Errorf("the default base is %q, not name:tag@sha256:<digest>", img)
	}
	// `docker compose build` with AUTHAMA_GO_VERSION unset.
	compose := readRepoFile(t, "compose.yaml")
	if img := d.baseImage(t, composeArgs(t, compose, nil)); !pinned.MatchString(img) {
		t.Errorf("compose builds from %q by default, not a pinned base", img)
	}
	// The deprecated override still works: AUTHAMA_GO_VERSION picks that Go.
	if img := d.baseImage(t, composeArgs(t, compose, map[string]string{"AUTHAMA_GO_VERSION": "1.26"})); img != "golang:1.26" {
		t.Errorf("AUTHAMA_GO_VERSION=1.26 builds from %q, want golang:1.26", img)
	}
}
