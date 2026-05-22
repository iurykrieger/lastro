package fixture

import (
	"fmt"
	"sort"
)

// Store is the in-memory fixture index. Built via NewStore (in-memory) or
// LoadDirectory (filesystem). Satisfies FixtureStore.
type Store struct {
	byID         map[string]Fixture
	byUseCase    map[string][]string // useCaseID -> sorted []ID
	allSortedIDs []string            // sorted []ID across all use cases
}

// NewStore builds a Store from the given fixtures. Returns an error if two
// fixtures share an id. Order of input does not matter; lookup ordering is
// always deterministic (sorted by id ascending).
func NewStore(fixtures ...Fixture) (*Store, error) {
	s := &Store{
		byID:      make(map[string]Fixture, len(fixtures)),
		byUseCase: make(map[string][]string),
	}
	for _, fx := range fixtures {
		if _, exists := s.byID[fx.ID]; exists {
			return nil, fmt.Errorf("fixture: duplicate id %q", fx.ID)
		}
		s.byID[fx.ID] = fx
		s.byUseCase[fx.UseCaseID] = append(s.byUseCase[fx.UseCaseID], fx.ID)
		s.allSortedIDs = append(s.allSortedIDs, fx.ID)
	}
	// Sort each use case's id slice and the global id slice for deterministic iteration.
	for uc := range s.byUseCase {
		sort.Strings(s.byUseCase[uc])
	}
	sort.Strings(s.allSortedIDs)
	return s, nil
}

// LookupFixture returns the fixture with the given id and ok=true if it
// exists; otherwise the zero Fixture and ok=false.
func (s *Store) LookupFixture(id string) (Fixture, bool) {
	fx, ok := s.byID[id]
	return fx, ok
}

// FixturesForUseCase returns all fixtures owned by useCaseID, sorted by id
// ascending. Returns an empty slice (never nil) when no fixtures match.
func (s *Store) FixturesForUseCase(useCaseID string) []Fixture {
	ids := s.byUseCase[useCaseID]
	out := make([]Fixture, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.byID[id])
	}
	return out
}

// All returns every fixture in the store, sorted by id ascending.
func (s *Store) All() []Fixture {
	out := make([]Fixture, 0, len(s.allSortedIDs))
	for _, id := range s.allSortedIDs {
		out = append(out, s.byID[id])
	}
	return out
}
