package fixture

import "fmt"

// Store is the in-memory fixture index. Built via NewStore (in-memory) or
// LoadDirectory (filesystem). Satisfies FixtureStore.
type Store struct {
	byID         map[string]Fixture
	byUseCase    map[string][]string // useCaseID -> sorted []ID
	allSortedIDs []string            // sorted []ID across all use cases
}

// NewStore builds a Store from the given fixtures. Returns an error if two
// fixtures share an id. Order of input does not matter; lookup ordering is
// always deterministic (sorted by id ascending — see Tasks 6-7).
func NewStore(fixtures ...Fixture) (*Store, error) {
	s := &Store{byID: make(map[string]Fixture, len(fixtures))}
	for _, fx := range fixtures {
		if _, exists := s.byID[fx.ID]; exists {
			return nil, fmt.Errorf("fixture: duplicate id %q", fx.ID)
		}
		s.byID[fx.ID] = fx
	}
	// Per-use-case + global indices: populated in Task 7.
	return s, nil
}

// LookupFixture is implemented in Task 6.
func (s *Store) LookupFixture(string) (Fixture, bool) { return Fixture{}, false }

// FixturesForUseCase is implemented in Task 7.
func (s *Store) FixturesForUseCase(string) []Fixture { return nil }

// All is implemented in Task 7.
func (s *Store) All() []Fixture { return nil }
