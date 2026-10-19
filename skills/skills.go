// Package skills bundles visor's agent skills, one folder each in the
// `npx skills` layout, so the binary can install them.
package skills

import "embed"

// FS holds one folder per skill, each with a SKILL.md.
//
//go:embed visor-*
var FS embed.FS
