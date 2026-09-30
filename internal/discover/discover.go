// Package discover asks the providers which models they serve and says
// which of those the catalog lacks. Two sources: each OpenAI-compatible
// provider the layers define, asked at its baseURL for /models, and
// `opencode models` run with OpenCode's plain config, which lists what
// the tool knows from its own logins. The set's config is not used for the
// second, since under a policy it would list only what is allowed.
//
// Claude Code has no command that lists its models, so those are added
// by hand.
package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jonathanleek/mi6/internal/layer"
	"github.com/jonathanleek/mi6/internal/merge"
)

// Found is one model a source serves.
type Found struct {
	// Key is provider/model, the catalog key it would have.
	Key string
	// Source is where it was seen: a provider's baseURL, or "opencode models".
	Source string
}

// Report is what discovery found.
type Report struct {
	// Found is every model seen, sorted by key.
	Found []Found
	// Missing are the found models the catalog lacks.
	Missing []Found
	// Notes say what could not be asked, and why.
	Notes []string
}

// Options controls Run. Zero values ask the real providers and run the
// real command.
type Options struct {
	Client *http.Client
	// OpenCode runs `opencode models` and returns its lines.
	OpenCode func() ([]string, error)
}

// Run discovers models for the merged stack.
func Run(m *merge.Merged, opts Options) *Report {
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	runOpenCode := opts.OpenCode
	if runOpenCode == nil {
		runOpenCode = openCodeModels
	}
	r := &Report{}
	seen := map[string]bool{}
	add := func(key, source string) {
		if !seen[key] {
			seen[key] = true
			r.Found = append(r.Found, Found{key, source})
		}
	}

	for _, id := range providerIDs(m.OpenCode) {
		base, key := providerURL(m.OpenCode, id)
		if base == "" {
			continue
		}
		ids, err := listModels(client, base, key)
		if err != nil {
			r.Notes = append(r.Notes, fmt.Sprintf("%s: %v", id, err))
			continue
		}
		for _, model := range ids {
			add(id+"/"+model, base)
		}
	}

	lines, err := runOpenCode()
	if err != nil {
		r.Notes = append(r.Notes, "opencode models: "+err.Error())
	}
	for _, line := range lines {
		if keyLine.MatchString(line) {
			add(line, "opencode models")
		}
	}

	sort.Slice(r.Found, func(i, j int) bool { return r.Found[i].Key < r.Found[j].Key })
	for _, f := range r.Found {
		if _, ok := m.Policy.Models[f.Key]; !ok {
			r.Missing = append(r.Missing, f)
		}
	}
	return r
}

// keyLine is a line of `opencode models`: provider/model and nothing else.
var keyLine = regexp.MustCompile(`^[A-Za-z0-9._-]+/\S+$`)

// providerIDs lists the providers in the merged opencode.json, sorted.
func providerIDs(cfg layer.Object) []string {
	providers, _ := cfg["provider"].(map[string]any)
	ids := make([]string, 0, len(providers))
	for id := range providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// providerURL returns a provider's options.baseURL and options.apiKey, or
// "" for a provider with no address, such as a built-in one.
func providerURL(cfg layer.Object, id string) (base, key string) {
	providers, _ := cfg["provider"].(map[string]any)
	entry, _ := providers[id].(map[string]any)
	options, _ := entry["options"].(map[string]any)
	base, _ = options["baseURL"].(string)
	key, _ = options["apiKey"].(string)
	return strings.TrimRight(base, "/"), key
}

// listModels asks an OpenAI-compatible server for its models.
func listModels(client *http.Client, base, key string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", nil)
	if err != nil {
		return nil, err
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("unreachable at %s: %v", base, unwrap(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s/models answered %s", base, resp.Status)
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("%s/models did not answer with a model list: %v", base, err)
	}
	var ids []string
	for _, d := range body.Data {
		if d.ID != "" {
			ids = append(ids, d.ID)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// unwrap strips the request from a client error, which repeats the URL.
func unwrap(err error) error {
	for {
		u, ok := err.(interface{ Unwrap() error })
		if !ok || u.Unwrap() == nil {
			return err
		}
		err = u.Unwrap()
	}
}

// openCodeModels runs `opencode models` with the tool's plain config: the
// set's variables and mi6's own are removed from its environment.
func openCodeModels() ([]string, error) {
	path, err := exec.LookPath("opencode")
	if err != nil {
		return nil, fmt.Errorf("opencode is not on your PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "models")
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "OPENCODE_") || strings.HasPrefix(name, "MI6_") {
			continue
		}
		cmd.Env = append(cmd.Env, kv)
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n"), nil
}
