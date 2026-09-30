package discover

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/jonathanleek/mi6/internal/layer"
	"github.com/jonathanleek/mi6/internal/merge"
)

func obj(t *testing.T, s string) layer.Object {
	t.Helper()
	o, err := layer.ParseObject([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func TestRun(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			auth = r.Header.Get("Authorization")
			w.Write([]byte(`{"object": "list", "data": [{"id": "qwen3-coder-30b"}, {"id": "gpt-oss-120b"}, {"id": ""}]}`))
		case "/broken/models":
			w.Write([]byte(`not json`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	l := &layer.Layer{Path: "/l", OpenCode: obj(t, `{"provider": {
		"lmstudio": {"npm": "x", "options": {"baseURL": "`+srv.URL+`/v1/", "apiKey": "k"}},
		"broken": {"options": {"baseURL": "`+srv.URL+`/broken"}},
		"gone": {"options": {"baseURL": "`+srv.URL+`/gone"}},
		"down": {"options": {"baseURL": "http://127.0.0.1:1/v1"}},
		"anthropic": {}
	}}`), Models: obj(t, `{"providers": {"lmstudio": {}, "anthropic": {}}, "models": {"lmstudio/qwen3-coder-30b": {}, "anthropic/claude-sonnet-5": {"claude": "sonnet"}}}`)}
	m := merge.Stack([]*layer.Layer{l}, func(p string) string { return p })

	r := Run(m, Options{OpenCode: func() ([]string, error) {
		return []string{"anthropic/claude-sonnet-5", "anthropic/claude-opus-5", "TOOL=opencode", "ARGS=models", "", "lmstudio/gpt-oss-120b"}, nil
	}})
	wantFound := []Found{
		{"anthropic/claude-opus-5", "opencode models"},
		{"anthropic/claude-sonnet-5", "opencode models"},
		{"lmstudio/gpt-oss-120b", srv.URL + "/v1"},
		{"lmstudio/qwen3-coder-30b", srv.URL + "/v1"},
	}
	if !reflect.DeepEqual(r.Found, wantFound) {
		t.Errorf("found %v", r.Found)
	}
	wantMissing := []Found{{"anthropic/claude-opus-5", "opencode models"}, {"lmstudio/gpt-oss-120b", srv.URL + "/v1"}}
	if !reflect.DeepEqual(r.Missing, wantMissing) {
		t.Errorf("missing %v", r.Missing)
	}
	if auth != "Bearer k" {
		t.Errorf("apiKey not sent: %q", auth)
	}
	if len(r.Notes) != 3 {
		t.Errorf("notes %v", r.Notes)
	}
	for _, want := range []string{"broken: ", "did not answer with a model list", "gone: ", "answered 404", "down: unreachable at http://127.0.0.1:1/v1"} {
		found := false
		for _, n := range r.Notes {
			if strings.Contains(n, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("notes lack %q: %v", want, r.Notes)
		}
	}

	r = Run(m, Options{OpenCode: func() ([]string, error) { return nil, errors.New("not on your PATH") }})
	if len(r.Notes) != 4 || !strings.Contains(r.Notes[3], "opencode models: not on your PATH") {
		t.Errorf("notes %v", r.Notes)
	}
}
