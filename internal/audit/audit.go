// Package audit appends one line per launch to a log, so there is a record
// of what was allowed where. What was used is in the tools' own history;
// the set and directory in each line join the two.
package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jonathanleek/mi6/internal/models"
)

// File is the log's name in the state directory.
const File = "audit.jsonl"

// Outcomes.
const (
	Started = "started"
	Refused = "refused"
	Bare    = "bare"
)

// Entry is one line of the log.
type Entry struct {
	Time    string   `json:"time"`
	MI6     string   `json:"mi6"`
	Tool    string   `json:"tool"`
	Dir     string   `json:"dir"`
	Set     string   `json:"set,omitempty"`
	Layers  []string `json:"layers"`
	Policy  string   `json:"policy,omitempty"`
	Deny    []Rule   `json:"deny"`
	Allow   []Allow  `json:"allow"`
	Allowed []string `json:"allowed"`
	Removed []Remove `json:"removed"`
	Default string   `json:"default,omitempty"`
	Outcome string   `json:"outcome"`
	Reasons []string `json:"reasons,omitempty"`
}

// Rule is a deny in force.
type Rule struct {
	Tag  string `json:"tag"`
	From string `json:"from"`
}

// Allow is one layer's allow.
type Allow struct {
	Tags []string `json:"tags"`
	From string   `json:"from"`
}

// Remove is a catalog model the tool may not use.
type Remove struct {
	Model string `json:"model"`
	Why   string `json:"why"`
}

// Path is the log's path under the state directory.
func Path(stateDir string) string { return filepath.Join(stateDir, File) }

// New starts an entry for a launch, timed now.
func New(version, tool, dir string) *Entry {
	return &Entry{
		Time: time.Now().Format(time.RFC3339), MI6: version, Tool: tool, Dir: dir,
		Layers: []string{}, Deny: []Rule{}, Allow: []Allow{}, Allowed: []string{}, Removed: []Remove{},
	}
}

// WithPolicy fills the entry from the policy and the tool's result under
// it. Outside a policy only the catalog is described.
func (e *Entry) WithPolicy(p *models.Policy, r *models.Result) *Entry {
	if p == nil {
		return e
	}
	if len(p.Models) > 0 || p.Active() {
		e.Policy = p.Hash()
	}
	for _, d := range p.Deny {
		e.Deny = append(e.Deny, Rule{d.Tag, d.From})
	}
	for _, a := range p.Allows {
		e.Allow = append(e.Allow, Allow{a.Tags, a.From})
	}
	if r != nil {
		for _, a := range r.Allowed {
			e.Allowed = append(e.Allowed, a.Key)
		}
		for _, x := range r.Removed {
			e.Removed = append(e.Removed, Remove{x.Key, x.Why})
		}
	}
	return e
}

// WithDefault records the tool's default model from its merged settings.
func (e *Entry) WithDefault(d string) *Entry {
	e.Default = d
	return e
}

// Append writes the entry as one line at the end of path, creating the
// file and its directory. The caller refuses the launch on an error.
func Append(path string, e *Entry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("cannot write the audit log: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("cannot write the audit log: %w", err)
	}
	defer f.Close()
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("cannot write the audit log %s: %w", path, err)
	}
	return nil
}

// Writable reports whether a line could be appended at path, for doctor.
func Writable(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}
