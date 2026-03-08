package shortcuts

import (
	"testing"
)

func TestBuiltinsNotEmpty(t *testing.T) {
	builtins := Builtins()
	if len(builtins) == 0 {
		t.Fatal("expected at least one built-in shortcut")
	}
}

func TestBuiltinNames(t *testing.T) {
	expected := map[string]bool{
		"sc":      true,
		"scp":     true,
		"amend":   true,
		"wip":     true,
		"undo":    true,
		"unstage": true,
		"rb":      true,
		"sync":    true,
		"pull":    true,
		"up":      true,
		"nb":      true,
		"done":    true,
		"cleanup": true,
		"review":  true,
		"pr":      true,
		"ship":    true,
		"squash":  true,
		"tidy":    true,
		"nuke":    true,
		"discard": true,
		"save":    true,
		"fixup":   true,
		"fresh":   true,
		"rename":  true,
	}

	builtins := Builtins()
	names := make(map[string]bool)
	for _, s := range builtins {
		names[s.Name] = true
	}

	for name := range expected {
		if !names[name] {
			t.Errorf("expected built-in shortcut %q not found", name)
		}
	}
}

func TestBuiltinsHaveSteps(t *testing.T) {
	for _, s := range Builtins() {
		if len(s.Steps) == 0 {
			t.Errorf("built-in %q has no steps", s.Name)
		}
	}
}

func TestBuiltinsAreMarkedBuiltin(t *testing.T) {
	for _, s := range Builtins() {
		if !s.Builtin {
			t.Errorf("built-in %q should have Builtin=true", s.Name)
		}
	}
}

func TestBuiltinsHaveDescription(t *testing.T) {
	for _, s := range Builtins() {
		if s.Description == "" {
			t.Errorf("built-in %q has empty description", s.Name)
		}
	}
}

func TestBuiltinsUseValidOps(t *testing.T) {
	validOps := map[string]bool{
		OpStage: true, OpUnstage: true, OpCommit: true,
		OpPush: true, OpPull: true, OpFetch: true,
		OpRebase: true, OpMerge: true, OpCheckout: true,
		OpBranch: true, OpReset: true, OpDelete: true,
		OpStash: true, OpStashPop: true, OpBranchRename: true,
	}

	for _, s := range Builtins() {
		for i, step := range s.Steps {
			if !validOps[step.Op] {
				t.Errorf("built-in %q step %d uses invalid op %q", s.Name, i, step.Op)
			}
		}
	}
}

func TestBuiltinsUseValidOnFail(t *testing.T) {
	valid := map[string]bool{
		OnFailStop:     true,
		OnFailContinue: true,
		OnFailAsk:      true,
	}

	for _, s := range Builtins() {
		for i, step := range s.Steps {
			if !valid[step.OnFail] {
				t.Errorf("built-in %q step %d uses invalid on_fail %q", s.Name, i, step.OnFail)
			}
		}
	}
}

func TestBuiltinsNbHasRequiredNameArg(t *testing.T) {
	for _, s := range Builtins() {
		if s.Name != "nb" {
			continue
		}
		found := false
		for _, a := range s.Args {
			if a.Name == "name" && a.Required {
				found = true
			}
		}
		if !found {
			t.Error("nb shortcut should have a required 'name' arg")
		}
	}
}
