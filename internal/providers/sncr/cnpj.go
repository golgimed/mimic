package sncr

// validCNPJ implements the standard Brazilian CNPJ check-digit algorithm
// (mod-11 over two weighted sums) against a raw 14-digit string. The manual
// requires the request's cnpj to "validate (check-digit)" without spelling
// out the algorithm — this is the one universally used for CNPJ.
func validCNPJ(cnpj string) bool {
	if len(cnpj) != 14 {
		return false
	}
	digits := make([]int, 14)
	for i, r := range cnpj {
		if r < '0' || r > '9' {
			return false
		}
		digits[i] = int(r - '0')
	}

	allSame := true
	for _, d := range digits[1:] {
		if d != digits[0] {
			allSame = false
			break
		}
	}
	if allSame {
		return false
	}

	weights1 := []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	weights2 := []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}

	if cnpjCheckDigit(digits[:12], weights1) != digits[12] {
		return false
	}
	if cnpjCheckDigit(digits[:13], weights2) != digits[13] {
		return false
	}
	return true
}

func cnpjCheckDigit(digits, weights []int) int {
	sum := 0
	for i, w := range weights {
		sum += digits[i] * w
	}
	r := sum % 11
	if r < 2 {
		return 0
	}
	return 11 - r
}
