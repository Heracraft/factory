package nixmenu

import (
	"errors"
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	f, err := Render(Selection{{ID: "bun"}, {ID: "nodejs", Options: map[string]string{"version": "22"}}, {ID: "python"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{Header, "pkgs.bun", "pkgs.nodejs_22", "pkgs.python312", "home.packages"} {
		if !strings.Contains(f, want) {
			t.Fatalf("missing %s in\n%s", want, f)
		}
	}
	if !IsGenerated(f) {
		t.Fatal("generated fragment not recognised")
	}
	if _, err := Render(Selection{{ID: "nope"}}); !errors.Is(err, ErrUnknownItem) {
		t.Fatalf("unknown id: %v", err)
	}
	if _, err := Render(Selection{{ID: "nodejs", Options: map[string]string{"version": "1"}}}); !errors.Is(err, ErrBadOption) {
		t.Fatalf("bad option: %v", err)
	}
	if _, err := Render(Selection{}); err != nil {
		t.Fatal(err)
	}
}

func TestEveryCatalogEntryRenders(t *testing.T) {
	for _, it := range Catalog() {
		if _, err := Render(Selection{{ID: it.ID}}); err != nil {
			t.Errorf("%s: %v", it.ID, err)
		}
		if it.Label == "" || it.Group == "" || it.Description == "" {
			t.Errorf("%s: incomplete entry", it.ID)
		}
	}
}
