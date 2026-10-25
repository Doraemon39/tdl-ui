package config

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"tdl-ui/internal/store"
	"testing"
)

func put(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
}
func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("protected file missing: %s: %v", path, err)
	}
}

func TestCacheCleanupPreservesDataDownloadsAndCustomDirs(t *testing.T) {
	m, err := NewManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	media := filepath.Join(m.DataDir(), "cache", "thumbs", "x.jpg")
	put(t, media)
	private := filepath.Join(m.KVDir(), "session")
	put(t, private)
	defaultPart := filepath.Join(m.DataDir(), "tmp", "video", "default.part")
	put(t, defaultPart)
	unknown := filepath.Join(m.DataDir(), "important.txt")
	put(t, unknown)
	custom := filepath.Join(t.TempDir(), "thumbs", "user.jpg")
	put(t, custom)
	if err := m.ScheduleCleanup("clean_cache"); err != nil {
		t.Fatal(err)
	}
	if err := m.Cleanup("clean_cache"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(media); !os.IsNotExist(err) {
		t.Fatal("default media cache retained")
	}
	for _, p := range []string{private, unknown, custom, m.configPath()} {
		assertExists(t, p)
	}
}

func TestWipePreservesUnknownAndCustomDirectory(t *testing.T) {
	m, err := NewManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := m.Get()
	s.CacheDir = filepath.Join(t.TempDir(), "my-cache")
	if err := m.Update(s); err != nil {
		t.Fatal(err)
	}
	custom := filepath.Join(s.CacheDir, "thumbs", "keep.jpg")
	unknown := filepath.Join(m.DataDir(), "notes.txt")
	put(t, custom)
	put(t, unknown)
	private := filepath.Join(m.KVDir(), "session")
	put(t, private)
	defaultPart := filepath.Join(m.DataDir(), "tmp", "video", "default.part")
	put(t, defaultPart)
	if err := m.ScheduleCleanup("wipe_all"); err != nil {
		t.Fatal(err)
	}
	if err := RunPendingCleanup(m.DataDir()); err != nil {
		t.Fatal(err)
	}
	assertExists(t, custom)
	assertExists(t, unknown)
	if _, err := os.Stat(defaultPart); !os.IsNotExist(err) {
		t.Fatal("default sibling cache retained during wipe")
	}
	if _, err := os.Stat(private); !os.IsNotExist(err) {
		t.Fatal("session retained")
	}
	if HasPendingCleanup(m.DataDir()) {
		t.Fatal("successful cleanup remained pending")
	}
}

func TestLegacyNestedCustomDirectoryBlocksResetUntilMoved(t *testing.T) {
	m, err := NewManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m.settings.CacheDir = filepath.Join(m.DataDir(), "tmp", "private-cache")
	custom := filepath.Join(m.settings.CacheDir, "keep")
	put(t, custom)
	if err := m.ScheduleCleanup("wipe_all"); err == nil {
		t.Fatal("reset accepted a nested custom directory")
	}
	s := m.Get()
	s.CacheDir = ""
	if err := m.UpdatePreferences(s); err == nil {
		t.Fatal("forgot a nonempty legacy custom directory")
	}
	assertExists(t, custom)
}

func TestWipeRejectsCurrentAndHistoricalDownloads(t *testing.T) {
	m, err := NewManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := m.Get()
	s.DownloadDir = filepath.Join(m.DataDir(), "downloads")
	if err := m.Update(s); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CleanupPlan("wipe_all"); err == nil {
		t.Fatal("current downloads overlap accepted")
	}
	s.DownloadDir = t.TempDir()
	if err := m.Update(s); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(m.DataDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.InsertTask(context.Background(), store.Task{ID: "history", Dir: filepath.Join(m.DataDir(), "old-downloads"), Status: "done"}, nil); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := m.CleanupPlan("wipe_all"); err == nil {
		t.Fatal("historical downloads overlap accepted")
	}
}

func TestCleanupNeverExecutesTargetsFromPendingFile(t *testing.T) {
	m, err := NewManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "keep")
	put(t, outside)
	if err := m.ScheduleCleanup("clean_cache"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(m.DataDir(), pendingCleanupName)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var plan CleanupSpec
	if err := json.Unmarshal(b, &plan); err != nil {
		t.Fatal(err)
	}
	plan.Targets = []string{outside}
	b, _ = json.Marshal(plan)
	os.WriteFile(path, b, 0o600)
	if err := RunPendingCleanup(m.DataDir()); err != nil {
		t.Fatal(err)
	}
	assertExists(t, outside)
}

func TestPreferencesCannotForgeIdentityOrAliasMaps(t *testing.T) {
	m := newTestManager(t)
	s := m.Get()
	s.LoggedInUserID = 42
	s.EnabledScripts = []string{"trusted.go"}
	s.RecentDirs = map[string][]string{"download": {t.TempDir()}}
	if err := m.Update(s); err != nil {
		t.Fatal(err)
	}
	stale := m.Get()
	stale.LoggedInUserID = 99
	stale.EnabledScripts = []string{"evil.go"}
	stale.RecentDirs["download"][0] = "forged"
	if err := m.UpdatePreferences(stale); err != nil {
		t.Fatal(err)
	}
	got := m.Get()
	if got.LoggedInUserID != 42 || got.EnabledScripts[0] != "trusted.go" || got.RecentDirs["download"][0] == "forged" {
		t.Fatal("backend metadata overwritten")
	}
	got.EnabledScripts[0] = "mutated"
	if m.Get().EnabledScripts[0] != "trusted.go" {
		t.Fatal("Get exposed backing slice")
	}
}

func TestSessionLeasesExcludeCredentialReplacement(t *testing.T) {
	m := newTestManager(t)
	read, err := m.UseSession()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.ChangeSession(); err == nil {
		t.Fatal("changed live session")
	}
	read()
	change, err := m.ChangeSession()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.UseSession(); err == nil {
		t.Fatal("used transitioning session")
	}
	change()
}

func TestConfirmedWipeRecoversFromCorruptMetadata(t *testing.T) {
	for _, runner := range []string{"helper", "startup"} {
		for _, corrupt := range []string{"config.yaml", "tasks.db"} {
			t.Run(runner+"/"+corrupt, func(t *testing.T) {
				m, err := NewManagerAt(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				private := filepath.Join(m.KVDir(), "session")
				put(t, private)
				unknown := filepath.Join(m.DataDir(), "keep.txt")
				put(t, unknown)
				downloaded := filepath.Join(t.TempDir(), "media.mp4")
				put(t, downloaded)
				s := m.Get()
				s.DownloadDir = filepath.Dir(downloaded)
				if err := m.Update(s); err != nil {
					t.Fatal(err)
				}
				history := filepath.Join(t.TempDir(), "historical-media.mp4")
				put(t, history)
				db, err := store.Open(filepath.Join(m.DataDir(), "tasks.db"))
				if err != nil {
					t.Fatal(err)
				}
				if err := db.InsertTask(context.Background(), store.Task{ID: "history", Dir: filepath.Dir(history), Status: "done"}, nil); err != nil {
					db.Close()
					t.Fatal(err)
				}
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
				if err := m.ScheduleCleanup("wipe_all"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(m.DataDir(), corrupt), []byte("invalid: [metadata"), 0o600); err != nil {
					t.Fatal(err)
				}
				if runner == "helper" {
					err = RunPendingCleanup(m.DataDir())
				} else {
					_, err = NewManagerAt(m.DataDir())
				}
				if err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(private); !os.IsNotExist(err) {
					t.Fatal("confirmed wipe did not remove session")
				}
				assertExists(t, unknown)
				assertExists(t, downloaded)
				assertExists(t, history)
				if HasPendingCleanup(m.DataDir()) {
					t.Fatal("completed wipe remains pending")
				}
			})
		}
	}
}

func TestCorruptDatabaseWithoutConfirmedScopeDoesNotAllowWipe(t *testing.T) {
	m, err := NewManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	private := filepath.Join(m.KVDir(), "session")
	put(t, private)
	if err := os.WriteFile(filepath.Join(m.DataDir(), "tasks.db"), []byte("corrupt SQLite"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.ScheduleCleanup("wipe_all"); err == nil {
		t.Fatal("unverified cleanup scope was accepted")
	}
	assertExists(t, private)
	if HasPendingCleanup(m.DataDir()) {
		t.Fatal("unsafe cleanup was scheduled")
	}
}

func TestCorruptConfigCannotUseLegacyOrOverlappingRecoveryScope(t *testing.T) {
	for _, kind := range []string{"legacy", "overlapping"} {
		t.Run(kind, func(t *testing.T) {
			m, err := NewManagerAt(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			private := filepath.Join(m.KVDir(), "session")
			put(t, private)
			if err := m.ScheduleCleanup("wipe_all"); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(m.DataDir(), pendingCleanupName)
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var plan CleanupSpec
			if err := json.Unmarshal(b, &plan); err != nil {
				t.Fatal(err)
			}
			if kind == "legacy" {
				plan.Version = 0
				plan.ProtectedDirs = nil
			} else {
				plan.ProtectedDirs = []string{m.KVDir()}
			}
			b, _ = json.Marshal(plan)
			if err := os.WriteFile(path, b, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(m.configPath(), []byte("bad: ["), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := RunPendingCleanup(m.DataDir()); err == nil {
				t.Fatal("unsafe recovery scope accepted")
			}
			assertExists(t, private)
		})
	}
}

func TestLiveDownloadProtectionStillBlocksCorruptDatabaseRecovery(t *testing.T) {
	m, err := NewManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	private := filepath.Join(m.KVDir(), "keep-media")
	put(t, private)
	if err := m.ScheduleCleanup("wipe_all"); err != nil {
		t.Fatal(err)
	}
	s := m.Get()
	s.DownloadDir = m.KVDir()
	if err := m.Update(s); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m.DataDir(), "tasks.db"), []byte("corrupt SQLite"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunPendingCleanup(m.DataDir()); err == nil {
		t.Fatal("saved scope overrode known live download protection")
	}
	assertExists(t, private)
}

func TestCorruptMetadataRecoveryIgnoresSuppliedTargets(t *testing.T) {
	m, err := NewManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "keep")
	put(t, outside)
	if err := m.ScheduleCleanup("wipe_all"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(m.DataDir(), pendingCleanupName)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var plan CleanupSpec
	if err := json.Unmarshal(b, &plan); err != nil {
		t.Fatal(err)
	}
	plan.Targets = []string{outside}
	b, _ = json.Marshal(plan)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.configPath(), []byte("bad: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunPendingCleanup(m.DataDir()); err != nil {
		t.Fatal(err)
	}
	assertExists(t, outside)
}
