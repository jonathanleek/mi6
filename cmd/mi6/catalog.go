package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jonathanleek/mi6/internal/catalog"
	"github.com/jonathanleek/mi6/internal/discover"
	"github.com/jonathanleek/mi6/internal/layer"
	"github.com/jonathanleek/mi6/internal/merge"
	"github.com/jonathanleek/mi6/internal/models"
	"github.com/jonathanleek/mi6/internal/resolve"
	"github.com/jonathanleek/mi6/internal/tool"
)

const catalogUsage = `usage:
  mi6 models                                     the catalog, and what is allowed here
  mi6 models discover                            models the providers serve that the catalog lacks
  mi6 models add <provider/model> [--tag t]... [--<tool> name]...
  mi6 models export [file]                       the merged catalog as one file, no rules
  mi6 models import <file>                       merge a catalog file in; adds only
  mi6 tags                                       the defined tags
  mi6 tags add <tag> "<meaning>"
  mi6 tag <model|provider> <tag>...
  mi6 untag <model|provider> <tag>...

A command that writes edits ~/.mi6/models.json, or the layer named with
--layer <dir>. Rules are hand-edits: put "allow" or "deny" in the
models.json of the layer where the restriction should start.
`

// stack is the current directory's stack, loaded and merged without
// building a set.
type stack struct {
	home   string
	st     *resolve.Stack
	layers []*layer.Layer
	merged *merge.Merged
}

func (s *stack) show(p string) string { return resolve.DisplayPath(p, s.home) }

func loadStack() (*stack, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	s := &stack{home: home}
	if s.st, err = resolve.Resolve(resolve.Options{Home: home}); err != nil {
		return nil, err
	}
	for _, l := range s.st.Layers {
		loaded, err := layer.Load(l.Path)
		if err != nil {
			return nil, err
		}
		s.layers = append(s.layers, loaded)
	}
	s.merged = merge.Stack(s.layers, s.show)
	return s, nil
}

// options splits args into positional words and --name value pairs. A
// name may repeat.
func options(args []string) (words []string, opts map[string][]string, err error) {
	opts = map[string][]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "--") {
			words = append(words, a)
			continue
		}
		name, value, has := strings.Cut(strings.TrimPrefix(a, "--"), "=")
		if !has {
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--%s needs a value", name)
			}
			i++
			value = args[i]
		}
		opts[name] = append(opts[name], value)
	}
	return words, opts, nil
}

// openLayer opens the models.json to edit: --layer, or ~/.mi6.
func openLayer(s *stack, opts map[string][]string) (*catalog.File, error) {
	dir := filepath.Join(s.home, resolve.LayerDir)
	if v := opts["layer"]; len(v) > 0 {
		dir = v[len(v)-1]
	}
	f, err := catalog.Open(dir)
	if err != nil {
		return nil, err
	}
	f.Name = s.show(f.Path)
	return f, nil
}

// save writes the file and reports it.
func save(s *stack, f *catalog.File) int {
	if err := f.Save(s.layers, s.show); err != nil {
		fmt.Fprintln(os.Stderr, "mi6:", err)
		return 1
	}
	fmt.Printf("wrote %s\n", s.show(f.Path))
	for _, n := range f.Notes {
		fmt.Printf("note  %s\n", n)
	}
	return 0
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "mi6:", err)
	return 1
}

func runModels(args []string) int {
	if len(args) == 0 {
		return listModels()
	}
	switch args[0] {
	case "add":
		return addModel(args[1:])
	case "discover":
		return discoverModels(args[1:])
	case "export":
		return exportModels(args[1:])
	case "import":
		return importModels(args[1:])
	}
	fmt.Fprint(os.Stderr, catalogUsage)
	return 2
}

func runTags(args []string) int {
	if len(args) == 0 {
		return listTags()
	}
	if args[0] == "add" {
		return addTag(args[1:])
	}
	fmt.Fprint(os.Stderr, catalogUsage)
	return 2
}

func listModels() int {
	s, err := loadStack()
	if err != nil {
		return fail(err)
	}
	p := s.merged.Policy
	if len(p.Models) == 0 {
		fmt.Println("no models in the catalog. Add one with: mi6 models add <provider/model> --tag <tag>")
		return printErrors(p)
	}
	if p.Active() {
		fmt.Printf("policy %s\n", p.Hash())
		for _, d := range p.Deny {
			fmt.Printf("  deny   %-10s %s\n", d.Tag, d.From)
		}
		for _, a := range p.Allows {
			fmt.Printf("  allow  %-10s %s\n", strings.Join(a.Tags, ","), a.From)
		}
	} else {
		fmt.Println("no rule applies here; every model in the catalog is allowed")
	}
	results := map[string]*models.Result{}
	for _, t := range tool.All() {
		if e, ok := t.(tool.Enforcer); ok {
			results[t.Name()] = p.Evaluate(t.Name(), e.ID())
		}
	}
	fmt.Println()
	for _, key := range p.ModelKeys() {
		m := p.Models[key]
		fmt.Printf("%-32s %s\n", key, strings.Join(m.Tags, ", "))
		for _, t := range tool.All() {
			r, ok := results[t.Name()]
			if !ok {
				continue
			}
			fmt.Printf("  %-9s %s\n", t.Name(), standing(r, key))
		}
	}
	return printErrors(p)
}

// standing says what a tool may do with a model.
func standing(r *models.Result, key string) string {
	for _, e := range r.Allowed {
		if e.Key == key {
			if e.ID != key {
				return "allowed as " + e.ID
			}
			return "allowed"
		}
	}
	for _, x := range r.Removed {
		if x.Key == key {
			return "removed: " + x.Why
		}
	}
	return ""
}

func printErrors(p *models.Policy) int {
	for _, e := range p.Errors {
		fmt.Printf("error  %s\n", e)
	}
	if len(p.Errors) > 0 {
		return 1
	}
	return 0
}

// discoverModels asks the providers what they serve and prints what the
// catalog lacks, with the source of each, so a tagging pass has a list.
func discoverModels(args []string) int {
	if len(args) > 0 {
		fmt.Fprint(os.Stderr, "usage: mi6 models discover\n")
		return 2
	}
	s, err := loadStack()
	if err != nil {
		return fail(err)
	}
	r := discover.Run(s.merged, discover.Options{})
	for _, n := range r.Notes {
		fmt.Printf("note  %s\n", n)
	}
	if len(r.Found) == 0 {
		fmt.Println("no models found. A provider in a layer's opencode.json is asked at its baseURL; opencode models lists the rest.")
		return 0
	}
	if len(r.Missing) == 0 {
		fmt.Printf("%d models served, all in the catalog\n", len(r.Found))
		return 0
	}
	fmt.Printf("%d models served, %d not in the catalog:\n", len(r.Found), len(r.Missing))
	for _, f := range r.Missing {
		fmt.Printf("  %-40s %s\n", f.Key, f.Source)
	}
	fmt.Println("add one with: mi6 models add <provider/model> --tag <tag>")
	return 0
}

func listTags() int {
	s, err := loadStack()
	if err != nil {
		return fail(err)
	}
	p := s.merged.Policy
	if len(p.Tags) == 0 {
		fmt.Println("no tags defined. Define one with: mi6 tags add <tag> \"<meaning>\"")
		return printErrors(p)
	}
	counts := map[string]int{}
	for _, m := range p.Models {
		for _, t := range m.Tags {
			counts[t]++
		}
	}
	fmt.Printf("%-12s %-7s %-32s %s\n", "tag", "models", "defined in", "meaning")
	for _, n := range p.TagNames() {
		t := p.Tags[n]
		fmt.Printf("%-12s %-7d %-32s %s\n", n, counts[n], t.From, t.Meaning)
	}
	return printErrors(p)
}

func addTag(args []string) int {
	words, opts, err := options(args)
	if err != nil {
		return fail(err)
	}
	if len(words) != 2 {
		fmt.Fprint(os.Stderr, "usage: mi6 tags add <tag> \"<meaning>\" [--layer dir]\n")
		return 2
	}
	s, err := loadStack()
	if err != nil {
		return fail(err)
	}
	f, err := openLayer(s, opts)
	if err != nil {
		return fail(err)
	}
	f.DefineTag(words[0], words[1])
	return save(s, f)
}

func addModel(args []string) int {
	words, opts, err := options(args)
	if err != nil {
		return fail(err)
	}
	if len(words) != 1 {
		fmt.Fprint(os.Stderr, "usage: mi6 models add <provider/model> [--tag t]... [--<tool> name]... [--layer dir]\n")
		return 2
	}
	s, err := loadStack()
	if err != nil {
		return fail(err)
	}
	f, err := openLayer(s, opts)
	if err != nil {
		return fail(err)
	}
	ids := map[string]string{}
	for name, values := range opts {
		switch name {
		case "tag", "layer":
			continue
		}
		if _, err := tool.Lookup(name); err != nil {
			return fail(fmt.Errorf("--%s is not a tool; the tools are %s", name, strings.Join(tool.Names(), ", ")))
		}
		ids[name] = values[len(values)-1]
	}
	if err := f.AddModel(words[0], opts["tag"], ids); err != nil {
		return fail(err)
	}
	return save(s, f)
}

func runTag(args []string, remove bool) int {
	words, opts, err := options(args)
	if err != nil {
		return fail(err)
	}
	if len(words) < 2 {
		verb := "tag"
		if remove {
			verb = "untag"
		}
		fmt.Fprintf(os.Stderr, "usage: mi6 %s <model|provider> <tag>... [--layer dir]\n", verb)
		return 2
	}
	s, err := loadStack()
	if err != nil {
		return fail(err)
	}
	f, err := openLayer(s, opts)
	if err != nil {
		return fail(err)
	}
	if remove {
		if err := f.Untag(words[0], words[1:]); err != nil {
			return fail(err)
		}
	} else {
		f.Tag(words[0], words[1:])
	}
	return save(s, f)
}

func exportModels(args []string) int {
	if len(args) > 1 {
		fmt.Fprint(os.Stderr, "usage: mi6 models export [file]\n")
		return 2
	}
	s, err := loadStack()
	if err != nil {
		return fail(err)
	}
	if code := printErrors(s.merged.Policy); code != 0 {
		return code
	}
	p := s.merged.Policy
	b, err := (&catalog.File{Obj: catalog.Export(p)}).Encode()
	if err != nil {
		return fail(err)
	}
	if len(args) == 0 {
		os.Stdout.Write(b)
		return 0
	}
	if err := os.WriteFile(args[0], b, 0o644); err != nil {
		return fail(err)
	}
	fmt.Printf("wrote %s: %d tags, %d providers, %d models\n", args[0], len(p.Tags), len(p.Providers), len(p.Models))
	return 0
}

func importModels(args []string) int {
	words, opts, err := options(args)
	if err != nil {
		return fail(err)
	}
	if len(words) != 1 {
		fmt.Fprint(os.Stderr, "usage: mi6 models import <file> [--layer dir]\n")
		return 2
	}
	b, err := os.ReadFile(words[0])
	if err != nil {
		return fail(err)
	}
	other, err := layer.ParseObject(b)
	if err != nil {
		return fail(fmt.Errorf("%s: %w", words[0], err))
	}
	s, err := loadStack()
	if err != nil {
		return fail(err)
	}
	f, err := openLayer(s, opts)
	if err != nil {
		return fail(err)
	}
	conflicts := f.Import(other, s.merged.Policy)
	for _, c := range conflicts {
		fmt.Printf("conflict  %s\n", c)
	}
	if code := save(s, f); code != 0 {
		return code
	}
	if len(conflicts) > 0 {
		return 1
	}
	return 0
}
