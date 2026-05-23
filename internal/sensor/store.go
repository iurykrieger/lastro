package sensor

import (
	"fmt"
	"sort"
)

// Store is the in-memory sensor index, built via NewStore (in-memory)
// or LoadDirectory (filesystem). All accessor methods return sensors
// sorted by id ascending so consumers get deterministic output.
type Store struct {
	byID         map[string]Sensor
	byUseCase    map[string][]string // useCaseID -> sorted []ID
	allSortedIDs []string            // sorted []ID across all sensors
}

// NewStore builds a Store from the given sensors. Returns an error if
// two sensors share an id. Order of input does not matter; lookup
// ordering is always deterministic (sorted by id ascending).
func NewStore(sensors ...Sensor) (*Store, error) {
	s := &Store{
		byID:      make(map[string]Sensor, len(sensors)),
		byUseCase: make(map[string][]string),
	}
	for _, sn := range sensors {
		if _, exists := s.byID[sn.ID]; exists {
			return nil, fmt.Errorf("sensor: duplicate id %q", sn.ID)
		}
		s.byID[sn.ID] = sn
		s.byUseCase[sn.UseCaseID] = append(s.byUseCase[sn.UseCaseID], sn.ID)
		s.allSortedIDs = append(s.allSortedIDs, sn.ID)
	}
	for uc := range s.byUseCase {
		sort.Strings(s.byUseCase[uc])
	}
	sort.Strings(s.allSortedIDs)
	return s, nil
}
