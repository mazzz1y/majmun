package natsort

import (
	"strconv"
	"unicode"
)

func Less(a, b string) bool {
	for {
		if len(a) == 0 && len(b) == 0 {
			return false
		}
		if len(a) == 0 {
			return true
		}
		if len(b) == 0 {
			return false
		}

		aIsDigit := unicode.IsDigit(rune(a[0]))
		bIsDigit := unicode.IsDigit(rune(b[0]))

		if aIsDigit && bIsDigit {
			aNum, aLen := extractNumber(a)
			bNum, bLen := extractNumber(b)

			if aNum != bNum {
				return aNum < bNum
			}

			a = a[aLen:]
			b = b[bLen:]
		} else if aIsDigit {
			return true
		} else if bIsDigit {
			return false
		} else {
			if a[0] != b[0] {
				return a[0] < b[0]
			}
			a = a[1:]
			b = b[1:]
		}
	}
}

func extractNumber(s string) (int64, int) {
	i := 0
	for i < len(s) && unicode.IsDigit(rune(s[i])) {
		i++
	}

	num, err := strconv.ParseInt(s[:i], 10, 64)
	if err != nil {
		return 0, i
	}

	return num, i
}
