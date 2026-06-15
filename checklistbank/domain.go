package checklistbank

import (
	"context"
	"fmt"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes checklistbank as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/checklistbank-cli/checklistbank"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then dereferences
// checklistbank:// URIs by routing to the operations Register installs. The same
// Domain also builds the standalone checklistbank binary (see cli.NewApp), so the
// binary and a host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the checklistbank driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against, and
// the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "checklistbank",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "checklistbank",
			Short:  "A command line for ChecklistBank taxonomic names.",
			Long: `A command line for ChecklistBank.

checklistbank reads public ChecklistBank data over plain HTTPS, shapes it into
clean records, and prints output that pipes into the rest of your tools. No API
key, nothing to run alongside it. ChecklistBank hosts 432M+ taxonomic names
from 64k+ checklists.`,
			Site: Host,
			Repo: "https://github.com/tamnd/checklistbank-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{Name: "search", Group: "read", List: true,
		Summary: "Search taxonomic names",
		Args:    []kit.Arg{{Name: "query", Help: "search query (e.g. Homo sapiens)"}}}, searchTaxa)

	kit.Handle(app, kit.OpMeta{Name: "taxon", Group: "read", Single: true,
		Summary: "Get a taxon by usage ID within a dataset", URIType: "taxon", Resolver: true,
		Args: []kit.Arg{{Name: "id", Help: "usage ID within the dataset"}}}, getTaxon)

	kit.Handle(app, kit.OpMeta{Name: "datasets", Group: "read", List: true,
		Summary: "List available checklists"}, listDatasets)
}

// newClient builds the client from the host-resolved config, so a host and the
// standalone binary pace and identify themselves the same way.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.HTTP.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- inputs ---

type searchInput struct {
	Query  string  `kit:"arg"          help:"search query"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Offset int     `kit:"flag"         help:"result offset"`
	Client *Client `kit:"inject"`
}

type taxonInput struct {
	ID         string  `kit:"arg"   help:"usage ID within the dataset"`
	Dataset    int     `kit:"flag"  help:"dataset key (required)"`
	Client     *Client `kit:"inject"`
}

type datasetsInput struct {
	Query  string  `kit:"flag"         help:"filter by title"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Client *Client `kit:"inject"`
}

// --- handlers ---

func searchTaxa(ctx context.Context, in searchInput, emit func(*Taxon) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	taxa, _, err := in.Client.SearchTaxa(ctx, in.Query, limit, in.Offset)
	if err != nil {
		return mapErr(err)
	}
	for i := range taxa {
		if err := emit(&taxa[i]); err != nil {
			return err
		}
	}
	return nil
}

func getTaxon(ctx context.Context, in taxonInput, emit func(*Taxon) error) error {
	if in.Dataset == 0 {
		return errs.Usage("--dataset is required")
	}
	t, err := in.Client.GetTaxon(ctx, in.Dataset, in.ID)
	if err != nil {
		return mapErr(err)
	}
	return emit(t)
}

func listDatasets(ctx context.Context, in datasetsInput, emit func(*Checklist) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	lists, _, err := in.Client.ListDatasets(ctx, in.Query, limit)
	if err != nil {
		return mapErr(err)
	}
	for i := range lists {
		if err := emit(&lists[i]); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver: the URI-native string functions, pure and network-free ---

// Classify turns any accepted input into the canonical (type, id).
// Any non-empty string is accepted as a taxon ID.
func (Domain) Classify(input string) (uriType, id string, err error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", "", errs.Usage("taxon ID required")
	}
	return "taxon", s, nil
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "taxon":
		return fmt.Sprintf("https://api.checklistbank.org/nameusage/%s", id), nil
	case "checklist":
		return fmt.Sprintf("https://www.checklistbank.org/dataset/%s", id), nil
	default:
		return "", errs.Usage("checklistbank has no resource type %q", uriType)
	}
}

// mapErr converts a library error into the kit error kind that carries the right
// exit code.
func mapErr(err error) error {
	return err
}
