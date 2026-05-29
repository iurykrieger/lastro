package sensor

import (
	"fmt"
	"sort"

	"github.com/iurykrieger/lastro/internal/enums"
)

// Store is the in-memory sensor index, built via NewStore (in-memory)
// or LoadDirectory (filesystem). All accessor methods return sensors
// sorted by id ascending so consumers get deterministic output.
type Store struct {
	byID         map[string]Sensor
	byUseCase    map[string][]string              // useCaseID -> sorted []ID
	byScope      map[enums.SensorScope][]string   // scope -> sorted []ID
	allSortedIDs []string                         // sorted []ID across all sensors
}

// NewStore builds a Store from the given sensors. Returns an error if
// two sensors share an id. Order of input does not matter; lookup
// ordering is always deterministic (sorted by id ascending).
func NewStore(sensors ...Sensor) (*Store, error) {
	s := &Store{
		byID:      make(map[string]Sensor, len(sensors)),
		byUseCase: make(map[string][]string),
		byScope:   make(map[enums.SensorScope][]string),
	}
	for _, sn := range sensors {
		if _, exists := s.byID[sn.ID]; exists {
			return nil, fmt.Errorf("sensor: duplicate id %q", sn.ID)
		}
		s.byID[sn.ID] = sn
		s.byUseCase[sn.UseCaseID] = append(s.byUseCase[sn.UseCaseID], sn.ID)
		scope := sn.Scope
		if scope == "" {
			scope = enums.ScopeUseCase
		}
		s.byScope[scope] = append(s.byScope[scope], sn.ID)
		s.allSortedIDs = append(s.allSortedIDs, sn.ID)
	}
	for uc := range s.byUseCase {
		sort.Strings(s.byUseCase[uc])
	}
	for sc := range s.byScope {
		sort.Strings(s.byScope[sc])
	}
	sort.Strings(s.allSortedIDs)
	return s, nil
}

// LookupSensor returns the sensor with the given id and ok=true if it
// exists; otherwise the zero Sensor and ok=false.
func (s *Store) LookupSensor(id string) (Sensor, bool) {
	sn, ok := s.byID[id]
	return sn, ok
}

// ForUseCase returns all sensors owned by useCaseID, sorted by id
// ascending. Returns an empty slice (never nil) when no sensors match.
func (s *Store) ForUseCase(useCaseID string) []Sensor {
	ids := s.byUseCase[useCaseID]
	out := make([]Sensor, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.byID[id])
	}
	return out
}

// All returns every sensor in the store, sorted by id ascending.
func (s *Store) All() []Sensor {
	out := make([]Sensor, 0, len(s.allSortedIDs))
	for _, id := range s.allSortedIDs {
		out = append(out, s.byID[id])
	}
	return out
}

// ForScope returns all sensors with the given scope, sorted by id
// ascending. An absent scope on a stored sensor counts as use-case.
func (s *Store) ForScope(scope enums.SensorScope) []Sensor {
	ids := s.byScope[scope]
	out := make([]Sensor, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.byID[id])
	}
	return out
}
