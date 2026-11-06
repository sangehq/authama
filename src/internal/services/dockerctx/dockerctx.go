// Package dockerctx creates the Docker context that reaches a VM host over
// SSH: `docker context create <alias> --docker host=ssh://<alias>`, or update
// when one by that name is already there. Contexts it writes carry the
// description Marker, and it only ever removes one that does (or whose
// endpoint is exactly ssh://<alias>), never the current one, after exporting
// a backup.
package dockerctx

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Marker is the description authama gives the contexts it manages.
const Marker = "managed by authama"

// Available reports whether the docker CLI is on PATH.
func Available() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker CLI not found")
	}
	return nil
}

// Verb is what Apply would do for alias: "create", or "update" when a
// context by that name already exists.
func Verb(alias string) string {
	if exec.Command("docker", "context", "inspect", alias).Run() == nil {
		return "update"
	}
	return "create"
}

// Endpoint is the context's docker endpoint for alias.
func Endpoint(alias string) string { return "ssh://" + alias }

// Apply creates or updates the context and returns the line to report.
func Apply(alias string) (string, error) {
	endpoint := "host=" + Endpoint(alias)
	verb := Verb(alias)
	out, err := exec.Command("docker", "context", verb, alias, "--description", Marker, "--docker", endpoint).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker context %s: %v: %s", verb, err, strings.TrimSpace(string(out)))
	}
	return fmt.Sprintf("docker context %s %s -> ssh://%s (use: docker --context %s ps)",
		verb+"d", alias, alias, alias), nil
}

// Context is one entry of `docker context ls`.
type Context struct {
	Name        string `json:"Name"`
	Description string `json:"Description"`
	Endpoint    string `json:"DockerEndpoint"`
	Current     bool   `json:"Current"`
}

// Managed reports whether authama wrote c (or c points exactly where an
// authama context for its name would).
func (c Context) Managed() bool {
	return c.Description == Marker || c.Endpoint == Endpoint(c.Name)
}

// List returns every Docker context.
func List() ([]Context, error) {
	out, err := exec.Command("docker", "context", "ls", "--format", "{{json .}}").Output()
	if err != nil {
		return nil, fmt.Errorf("docker context ls: %w", err)
	}
	return ParseList(out)
}

// ParseList reads `docker context ls --format '{{json .}}'` output.
func ParseList(out []byte) ([]Context, error) {
	var cs []Context
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var c Context
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			return nil, fmt.Errorf("docker context ls: %w", err)
		}
		cs = append(cs, c)
	}
	return cs, sc.Err()
}

// Remove exports the context to backupDir and removes it. It refuses a
// context authama does not manage and the current one.
func Remove(c Context, backupDir string) (string, error) {
	if !c.Managed() {
		return "", fmt.Errorf("docker context %s was not created by authama; left alone", c.Name)
	}
	if c.Current {
		return "", fmt.Errorf("docker context %s is the current context; switch away (docker context use default) and sync again", c.Name)
	}
	if backupDir == "" {
		return "", errors.New("no backup directory")
	}
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return "", err
	}
	backup := filepath.Join(backupDir, c.Name+"-"+time.Now().UTC().Format("20060102T150405Z")+".dockercontext")
	if out, err := exec.Command("docker", "context", "export", c.Name, backup).CombinedOutput(); err != nil {
		return "", fmt.Errorf("docker context export %s (the backup taken before removal): %v: %s", c.Name, err, strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("docker", "context", "rm", c.Name).CombinedOutput(); err != nil {
		return "", fmt.Errorf("docker context rm %s: %v: %s", c.Name, err, strings.TrimSpace(string(out)))
	}
	return fmt.Sprintf("docker context %s removed (backup: %s; restore: docker context import %s %s)", c.Name, backup, c.Name, backup), nil
}
