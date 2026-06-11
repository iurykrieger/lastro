// Package schemas embeds the canonical YAML JSON Schemas and enum files
// for every harness framework entity, so internal packages can validate
// inputs without depending on the repo working tree at runtime.
package schemas

import "embed"

// FS exposes every entity schema (e.g., stack-component.yaml,
// stack-manifest.yaml) and every enum file (under enums/).
// Examples and README are intentionally excluded.
//
//go:embed *.yaml enums/*.yaml core-inputs/*.yaml
var FS embed.FS
