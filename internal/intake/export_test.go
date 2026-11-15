package intake

import "time"

// ParseCheckinForTest — доступ к правилам чек-ина для тестов.
func ParseCheckinForTest(text string, now time.Time) (Extracted, bool) {
	return parseCheckinText(text, now)
}
