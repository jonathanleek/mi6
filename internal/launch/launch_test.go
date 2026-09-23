package launch

import (
	"reflect"
	"testing"
)

func TestWithVars(t *testing.T) {
	env := []string{"PATH=/bin", "CLAUDE_CONFIG_DIR=/old", "HOME=/h", "EMPTY="}
	got := withVars(env, []string{"CLAUDE_CONFIG_DIR=/new", "MI6_TOOL=claude"})
	want := []string{"PATH=/bin", "HOME=/h", "EMPTY=", "CLAUDE_CONFIG_DIR=/new", "MI6_TOOL=claude"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestWithVarsKeyPrefixIsNotAMatch(t *testing.T) {
	got := withVars([]string{"OPENCODE_CONFIG_DIR=/keep"}, []string{"OPENCODE_CONFIG=/x"})
	want := []string{"OPENCODE_CONFIG_DIR=/keep", "OPENCODE_CONFIG=/x"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
