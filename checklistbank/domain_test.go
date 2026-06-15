package checklistbank

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the URI driver's pure string functions
// and the host wiring (mint, body, resolve), which need no network. The client's
// HTTP behaviour is covered in checklistbank_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "checklistbank" {
		t.Errorf("Scheme = %q, want checklistbank", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "checklistbank" {
		t.Errorf("Identity.Binary = %q, want checklistbank", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct{ in, typ, id string }{
		{"abc123", "taxon", "abc123"},
		{"  xyz-789  ", "taxon", "xyz-789"},
		{"R_3454", "taxon", "R_3454"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil || typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.typ, tc.id)
		}
	}
}

func TestClassifyEmpty(t *testing.T) {
	_, _, err := Domain{}.Classify("")
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestLocate(t *testing.T) {
	got, err := Domain{}.Locate("taxon", "abc123")
	want := "https://api.checklistbank.org/nameusage/abc123"
	if err != nil || got != want {
		t.Errorf("Locate = (%q, %v), want (%q, nil)", got, err, want)
	}

	got, err = Domain{}.Locate("checklist", "9")
	want = "https://www.checklistbank.org/dataset/9"
	if err != nil || got != want {
		t.Errorf("Locate = (%q, %v), want (%q, nil)", got, err, want)
	}
}

func TestLocateUnknownType(t *testing.T) {
	_, err := Domain{}.Locate("species", "abc")
	if err == nil {
		t.Error("expected error for unknown resource type")
	}
}

// TestHostWiring mounts the driver in a kit Host (the runtime ant drives) and
// checks the round trip: a record mints to its URI, its body is readable, and a
// bare id resolves back to the same URI. The init in domain.go registers the
// domain, so kit.Open finds it.
func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	tx := &Taxon{ID: "abc123", ScientificName: "Homo sapiens", Rank: "species", Label: "Homo sapiens Linnaeus, 1758"}
	u, err := h.Mint(tx)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if want := "checklistbank://taxon/abc123"; u.String() != want {
		t.Errorf("Mint = %q, want %q", u.String(), want)
	}

	got, err := h.ResolveOn("checklistbank", "def456")
	if err != nil || got.String() != "checklistbank://taxon/def456" {
		t.Errorf("ResolveOn = (%q, %v), want checklistbank://taxon/def456", got.String(), err)
	}
}
