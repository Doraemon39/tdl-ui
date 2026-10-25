package config

import "testing"

func TestDiskUsageLimitMigrationAndPersistence(t *testing.T) {
	m, err := NewManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.applyYAML([]byte("download:\n  threads: 4\n")); err != nil {
		t.Fatal(err)
	}
	if m.Get().DiskUsageLimit != 90 {
		t.Fatal("old settings did not receive the safe default")
	}
	s := m.Get()
	s.DiskUsageLimit = 85
	if err := m.Update(s); err != nil {
		t.Fatal(err)
	}
	loaded, err := NewManagerAt(m.DataDir())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Get().DiskUsageLimit != 85 {
		t.Fatal("limit was not persisted")
	}
	for _, limit := range []int{-1, 100, 101} {
		s.DiskUsageLimit = limit
		if err := m.Update(s); err == nil {
			t.Fatalf("invalid limit accepted: %d", limit)
		}
	}
}
