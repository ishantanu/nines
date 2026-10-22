package skills

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
)

//go:embed nines-cli/SKILL.md
var cliInstructions string

func CLISkill() Skill {
	s := Skill{ID: "nines-cli", Version: "1.0.0", Name: "Nines CLI", Description: "Deterministic offline and tenant-scoped CLI execution", Instructions: cliInstructions}
	h := sha256.Sum256([]byte(s.Instructions))
	s.Digest = hex.EncodeToString(h[:])
	return s
}
