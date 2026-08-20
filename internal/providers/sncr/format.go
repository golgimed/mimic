package sncr

import (
	"fmt"
	"time"
)

// formatPrescriptionNumber renders one server-assigned prescription number
// in the shape shown by the manual's examples (e.g. "2411.1-00.0000001" for
// notification numbers, "2602.6-53.0000001" for especial-retencao ranges):
// YYMM.<code>-<ufCode>.<7-digit sequence>.
//
// The manual documents the shape but not the exact meaning of the middle
// segments — they're an opaque server-assigned identifier from the
// consumer's point of view (GolgiMed persists the number as-is, it never
// decodes it). code/uf are folded in deterministically only so the format
// looks plausible and differs sensibly across receita/tipo and UF.
func formatPrescriptionNumber(code, uf string, seq int64) string {
	yearMonth := time.Now().UTC().Format("0601")
	return fmt.Sprintf("%s.%d-%02d.%07d", yearMonth, codeDigit(code), ufCode(uf), seq)
}

// codeDigit maps a receita/tipo enum value to a single digit for the number
// format's middle segment. Order is arbitrary — no meaning is documented.
func codeDigit(code string) int {
	order := []string{"NRA", "NRB", "NRB2", "NRR", "NRT", "RCE", "RET"}
	for i, c := range order {
		if c == code {
			return i + 1
		}
	}
	return 0
}

// ufCode folds a UF string into a stable 2-digit value for the number
// format's middle segment. No real IBGE UF code table is documented for
// this API, so this is a deterministic placeholder, not a claim about the
// real encoding.
func ufCode(uf string) int {
	sum := 0
	for _, r := range uf {
		sum += int(r)
	}
	return sum % 100
}
