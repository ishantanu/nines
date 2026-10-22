// Package skills distributes the versioned, server-owned agent guidance.
package skills

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
)

//go:embed reliability-investigation/SKILL.md
var investigation string

//go:embed journey-proposal/SKILL.md
var proposal string

type Skill struct {
	ID           string `json:"id"`
	Version      string `json:"version"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Digest       string `json:"digest"`
	Instructions string `json:"instructions"`
}

func Catalog() []Skill {
	out := []Skill{
		{ID: "reliability-investigation", Version: "1.1.0", Name: "Reliability investigation", Description: "Read-only journey, SLO and agent-run evidence investigation", Instructions: investigation},
		{ID: "journey-proposal", Version: "1.0.0", Name: "Journey proposal", Description: "Exact-manifest proposal requiring explicit revision-checked approval", Instructions: proposal},
	}
	for i := range out {
		sum := sha256.Sum256([]byte(out[i].Instructions))
		out[i].Digest = hex.EncodeToString(sum[:])
	}
	return out
}
