package engine

import (
	"fmt"
	"sort"
	"strings"

	"tdl-ui/internal/manifest"
)

// Record only files that have been verified, synced and published. An imported
// journal is always appended to; other sources each get their own local journal.
func (t *Task) recordCompletion(dialogID int64, typ string, entry manifest.Entry) (string, error) {
	t.manifestMu.Lock()
	defer t.manifestMu.Unlock()
	s := t.manifest
	if s == nil || !s.MatchesSource(dialogID, typ) {
		key := fmt.Sprintf("%s:%d", manifest.Namespace(typ), dialogID)
		s = t.completionManifests[key]
		if s == nil {
			var err error
			// Automatic journals belong to this run. Do not consume the bounded
			// registry used by explicit manifest imports for the app's lifetime.
			s, err = manifest.NewRegistry().OpenTask(t.opts.Dir, t.ID, manifest.Header{
				AccountID: t.opts.AccountID, DialogID: dialogID, DialogType: typ, Title: t.opts.Label,
			})
			if err != nil {
				return "", t.recordManifestError(err)
			}
			if t.completionManifests == nil {
				t.completionManifests = map[string]*manifest.Session{}
			}
			t.completionManifests[key] = s
		}
	}
	path := s.Path()
	if err := s.MarkDone(entry); err != nil {
		return path, t.recordManifestError(err)
	}
	return path, nil
}

func (t *Task) recordManifestError(err error) error {
	t.mu.Lock()
	t.manifestError = err.Error()
	t.mu.Unlock()
	return err
}

func (t *Task) diskStopNotice(reason error) string {
	t.manifestMu.Lock()
	defer t.manifestMu.Unlock()
	paths := map[string]bool{}
	if t.manifest != nil {
		paths[t.manifest.Path()] = true
	}
	for _, s := range t.completionManifests {
		paths[s.Path()] = true
	}
	list := make([]string, 0, len(paths))
	for path := range paths {
		list = append(list, path)
	}
	sort.Strings(list)
	if len(list) == 0 {
		return reason.Error() + "\n本次尚无新增完成记录。"
	}
	return reason.Error() + "\n已下载清单：\n" + strings.Join(list, "\n")
}
