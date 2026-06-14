// internal/environment/parse.go
package environment

// Parse runs every deterministic parser over repoDir and assembles RawFacts.
// It never interprets — classification is the skill's job. Missing infra files
// degrade to empty maps, never errors.
func Parse(repoDir string) (RawFacts, error) {
	scripts, err := parsePackageScripts(repoDir)
	if err != nil {
		return RawFacts{}, err
	}
	compose, composeFile, err := parseCompose(repoDir)
	if err != nil {
		return RawFacts{}, err
	}
	return RawFacts{
		Scripts:         scripts,
		MakeTargets:     parseMakeTargets(repoDir),
		ProcfileEntries: parseProcfile(repoDir),
		ComposeServices: compose,
		ComposeFile:     composeFile,
		EnvKeys:         parseDotenvKeys(repoDir),
	}, nil
}
