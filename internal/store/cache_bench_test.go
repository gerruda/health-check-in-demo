package store_test

import (
	"testing"
	"time"

	"healthbot/internal/nlu"
	"healthbot/internal/store"
)

func prepareHistory(b *testing.B) (store.Paths, *store.Reader) {
	b.Helper()
	root := b.TempDir()
	paths := store.NewPaths(root)
	writer := store.NewWriter(paths)
	for day := 0; day < 365; day++ {
		hrv := 28 + day%10
		sleep := 6.5 + float64(day%4)*0.3
		_, err := writer.WriteCheckin(nlu.Checkin{
			Date:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, day).Format("2006-01-02"),
			Person: nlu.Athlete,
			HRV:    &hrv,
			SleepH: &sleep,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
	return paths, store.NewReader(paths)
}

func BenchmarkCheckinValuesCached(b *testing.B) {
	_, reader := prepareHistory(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := reader.CheckinValues(nlu.Athlete, 7); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCheckinValuesUncached(b *testing.B) {
	paths, _ := prepareHistory(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := store.NewReader(paths, store.WithoutCache())
		if _, err := reader.CheckinValues(nlu.Athlete, 7); err != nil {
			b.Fatal(err)
		}
	}
}
