package intake

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseDateTextFromScreenshotHeader(t *testing.T) {
	today := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	assert.Equal(t, "2026-11-13", parseDateText("13 нояб. 2026 г, 05:01", today))
	assert.Equal(t, "2026-11-13", parseDateText("13 ноября 2026", today))
	assert.Equal(t, "2026-11-11", parseDateText("11 нояб. @ 12:55", today))
}
