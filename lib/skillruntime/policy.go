package skillruntime

import (
	"os"
	"path/filepath"

	"github.com/iurykrieger/lastro/internal/policy"
)

// LoadPolicies resolves the effective validation policy for a use case.
// Both files are optional; missing or malformed yields an empty policy.
func LoadPolicies(policyDir, useCaseID string) *policy.EffectivePolicy {
	global := loadPolicyFile(filepath.Join(policyDir, "global.yaml"))
	local := loadPolicyFile(filepath.Join(policyDir, "local", useCaseID+".yaml"))
	return policy.Resolve(global, local)
}

func loadPolicyFile(path string) *policy.ValidationPolicy {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	p, err := policy.Load(f)
	if err != nil {
		return nil
	}
	return p
}
