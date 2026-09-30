package catalog

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jonathanleek/mi6/internal/layer"
	"github.com/jonathanleek/mi6/internal/models"
)

func obj(t *testing.T, s string) layer.Object {
	t.Helper()
	o, err := layer.ParseObject([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func display(p string) string { return p }

func TestOpenAcceptsLayerOrParent(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{dir, filepath.Join(dir, ".mi6")} {
		f, err := Open(d)
		if err != nil {
			t.Fatal(err)
		}
		if f.Path != filepath.Join(dir, ".mi6", "models.json") || len(f.Obj) != 0 {
			t.Errorf("%s: path %s obj %v", d, f.Path, f.Obj)
		}
	}
}

func TestEditsThenSave(t *testing.T) {
	dir := t.TempDir()
	f, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	// A tag nobody defines is refused, and nothing is written.
	f.Tag("lmstudio/qwen", []string{"chinese"})
	err = f.Save(nil, display)
	if err == nil || !strings.Contains(err.Error(), `tag "chinese" in model lmstudio/qwen is not defined`) {
		t.Errorf("save with an undefined tag: %v", err)
	}
	if _, statErr := os.Stat(f.Path); statErr == nil {
		t.Error("a refused save wrote the file")
	}

	f, _ = Open(dir)
	f.DefineTag("chinese", "Made in China")
	f.DefineTag("network", "On the LAN")
	if err := f.AddModel("lmstudio/qwen", []string{"chinese"}, map[string]string{"claude": "q"}); err != nil {
		t.Fatal(err)
	}
	if err := f.AddModel("lmstudio/qwen", nil, nil); err == nil {
		t.Error("adding a model twice should fail")
	}
	if err := f.AddModel("noslash", nil, nil); err == nil {
		t.Error("a key without a provider should fail")
	}
	f.Tag("lmstudio", []string{"network"})
	f.Tag("lmstudio/qwen", []string{"chinese", "network"}) // one already there
	if err := f.Untag("lmstudio/qwen", []string{"network"}); err != nil {
		t.Fatal(err)
	}
	if err := f.Untag("lmstudio/qwen", []string{"network"}); err == nil || !strings.Contains(err.Error(), "does not attach network") {
		t.Errorf("untag of a tag not in the file: %v", err)
	}
	if err := f.Untag("other/x", []string{"chinese"}); err == nil {
		t.Error("untag of a model not in the file should fail")
	}
	if len(f.Notes) != 1 || !strings.Contains(f.Notes[0], "provider lmstudio") {
		t.Errorf("notes %v", f.Notes)
	}
	if err := f.Save(nil, display); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(f.Path)
	want := "{\n\t\"models\": {\n\t\t\"lmstudio/qwen\": {\n\t\t\t\"claude\": \"q\",\n\t\t\t\"tags\": [\n\t\t\t\t\"chinese\"\n\t\t\t]\n\t\t}\n\t},\n\t\"providers\": {\n\t\t\"lmstudio\": {\n\t\t\t\"tags\": [\n\t\t\t\t\"network\"\n\t\t\t]\n\t\t}\n\t},\n\t\"tags\": {\n\t\t\"chinese\": \"Made in China\",\n\t\t\"network\": \"On the LAN\"\n\t}\n}\n"
	if string(b) != want {
		t.Errorf("file:\n%s", b)
	}
	// It loads as a layer and the policy sees provider tags on the model.
	l, err := layer.Load(filepath.Join(dir, ".mi6"))
	if err != nil {
		t.Fatal(err)
	}
	p := models.Merge([]*layer.Layer{l}, display)
	if got := p.Models["lmstudio/qwen"].Tags; !reflect.DeepEqual(got, []string{"chinese", "network"}) || len(p.Errors) > 0 {
		t.Errorf("tags %v errors %v", got, p.Errors)
	}
}

func TestSaveChecksAgainstTheStack(t *testing.T) {
	// A tag defined in a higher layer is fine to attach in a lower one.
	home := &layer.Layer{Path: "/home/.mi6", Models: obj(t, `{"tags": {"chinese": "x"}, "providers": {"lmstudio": {}}, "models": {"lmstudio/qwen": {}}}`)}
	dir := t.TempDir()
	f, _ := Open(dir)
	f.Tag("lmstudio/qwen", []string{"chinese"})
	if err := f.Save([]*layer.Layer{home}, display); err != nil {
		t.Errorf("attaching a tag from a higher layer: %v", err)
	}
	// Editing a layer that is in the stack replaces its content for the check.
	self := &layer.Layer{Path: filepath.Join(dir, ".mi6"), Models: obj(t, `{"tags": {"old": "x"}}`)}
	f, _ = Open(dir)
	f.DefineTag("new", "y")
	f.Tag("lmstudio/qwen", []string{"old"})
	err := f.Save([]*layer.Layer{home, self}, display)
	if err == nil || !strings.Contains(err.Error(), `tag "old"`) {
		t.Errorf("the stack's stale copy of this layer should not count: %v", err)
	}
}

func TestExportAndImport(t *testing.T) {
	stackLayer := &layer.Layer{Path: "/home/.mi6", Models: obj(t, `{
		"tags": {"chinese": "Made in China", "network": "On the LAN"},
		"providers": {"lmstudio": {"tags": ["network"]}, "anthropic": {}},
		"models": {"lmstudio/qwen": {"tags": ["chinese"]}, "anthropic/claude-sonnet-5": {"claude": "sonnet"}},
		"deny": ["chinese"]}`)}
	p := models.Merge([]*layer.Layer{stackLayer}, display)
	exported := Export(p)
	if _, ok := exported["deny"]; ok {
		t.Error("export carried a rule")
	}
	if got := exported["models"].(map[string]any)["lmstudio/qwen"].(map[string]any)["tags"]; !reflect.DeepEqual(got, []any{"chinese"}) {
		t.Errorf("export attaches the model's own tags, not the provider's: %v", got)
	}

	// Importing the export back adds nothing.
	f, _ := Open(t.TempDir())
	if c := f.Import(exported, p); len(c) > 0 || len(f.Obj) > 0 {
		t.Errorf("re-import: conflicts %v obj %v", c, f.Obj)
	}

	// A file with new things, a changed meaning, a changed tool name, and a rule.
	other := obj(t, `{
		"tags": {"network": "Nearby", "fast": "Quick"},
		"providers": {"lmstudio": {"tags": ["fast"]}, "ollama": {}},
		"models": {"lmstudio/qwen": {"tags": ["fast"], "claude": "q"}, "anthropic/claude-sonnet-5": {"claude": "sonnet-5"}, "ollama/llama": {}},
		"deny": ["fast"]}`)
	f, _ = Open(t.TempDir())
	conflicts := f.Import(other, p)
	wantConflicts := []string{
		`anthropic/claude-sonnet-5 is "sonnet" for claude in the stack and "sonnet-5" in the file; kept ours`,
		`tag network means "On the LAN" in /home/.mi6 and "Nearby" in the file; kept ours`,
	}
	if !reflect.DeepEqual(conflicts, wantConflicts) {
		t.Errorf("conflicts %v", conflicts)
	}
	if got := f.Obj["tags"].(map[string]any); len(got) != 1 || got["fast"] != "Quick" {
		t.Errorf("tags %v", got)
	}
	if got := f.Obj["providers"].(map[string]any); len(got) != 2 || !reflect.DeepEqual(got["lmstudio"].(map[string]any)["tags"], []any{"fast"}) {
		t.Errorf("providers %v", got)
	}
	ms := f.Obj["models"].(map[string]any)
	if len(ms) != 2 || ms["lmstudio/qwen"].(map[string]any)["claude"] != "q" || !reflect.DeepEqual(ms["lmstudio/qwen"].(map[string]any)["tags"], []any{"fast"}) {
		t.Errorf("models %v", ms)
	}
	if _, ok := ms["ollama/llama"]; !ok {
		t.Errorf("new model not added: %v", ms)
	}
	if !strings.Contains(strings.Join(f.Notes, "\n"), "deny was ignored") {
		t.Errorf("notes %v", f.Notes)
	}
	// Saved on top of the stack, the result is valid.
	if err := f.Save([]*layer.Layer{stackLayer}, display); err != nil {
		t.Errorf("save: %v", err)
	}

	// A file with the wrong shape is refused whole.
	f, _ = Open(t.TempDir())
	if c := f.Import(obj(t, `{"tags": ["x"]}`), p); len(c) != 1 || !strings.Contains(c[0], "tags must be an object") {
		t.Errorf("bad shape: %v", c)
	}
}
