package fixturebinder

import (
	"sort"

	"github.com/iurykrieger/lastro/internal/usecase/template"
)

// CollectFixtureRefs parses run plus every with value and returns the sorted,
// deduped set of fixture ids referenced via ${{ fixtures.<id> }}.
func CollectFixtureRefs(run string, with map[string]string) ([]string, error) {
	seen := map[string]bool{}
	add := func(src string) error {
		segs, err := template.Parse(src)
		if err != nil {
			return err
		}
		_, refs, err := template.Compile(segs)
		if err != nil {
			return err
		}
		for _, id := range refs.Fixtures {
			seen[id] = true
		}
		return nil
	}
	if err := add(run); err != nil {
		return nil, err
	}
	for _, v := range with {
		if err := add(v); err != nil {
			return nil, err
		}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}
