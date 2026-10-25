package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"tdl-ui/internal/engine"
	"tdl-ui/internal/manifest"
)

type DownloadPreview struct {
	Total              int                 `json:"total"`
	ManifestSkipped    int                 `json:"manifestSkipped"`
	LocalMatched       int                 `json:"localMatched"`
	Pending            int                 `json:"pending"`
	ManifestMessageIDs []int               `json:"manifestMessageIds"`
	Comparison         manifest.Comparison `json:"comparison"`
}

type mediaReview struct {
	owner           int64
	dialog          int64
	typ, title, dir string
	entries         []manifest.Entry
	comparison      manifest.Comparison
	at              time.Time
}

func mediaEntries(dialog int64, typ string, items []MediaItem) ([]manifest.Entry, error) {
	if dialog <= 0 || engine.PeerNamespace(typ) == "" || len(items) > manifest.MaxEntries {
		return nil, errors.New("媒体来源或数量无效")
	}
	entries := make([]manifest.Entry, 0, len(items))
	seen := map[int]bool{}
	for _, item := range items {
		if item.DialogID != dialog || item.MessageID <= 0 || int64(item.MessageID) > 2147483647 || item.MediaID == "" || item.Name == "" || item.Size < 0 || item.Date <= 0 || seen[item.MessageID] {
			return nil, errors.New("媒体信息不完整、重复或不属于此对话")
		}
		seen[item.MessageID] = true
		entries = append(entries, manifest.Entry{MessageID: item.MessageID, MediaID: item.MediaID, Name: item.Name, Size: item.Size, Date: item.Date})
	}
	return entries, nil
}

func (s *DownloadService) compare(dir string, dialog int64, typ string, items []MediaItem) (manifest.Comparison, []manifest.Entry, error) {
	if s.cfg.Get().LoggedInUserID <= 0 {
		return manifest.Comparison{}, nil, errNotLoggedIn
	}
	entries, err := mediaEntries(dialog, typ, items)
	if err != nil {
		return manifest.Comparison{}, nil, err
	}
	known, err := s.manager.KnownLocalMatches(dir)
	if err != nil {
		return manifest.Comparison{}, nil, err
	}
	cmp, err := manifest.Compare(dir, entries, known, func(e manifest.Entry) string {
		return engine.PeerNamespace(typ) + ":" + fmtCoordinate(dialog, e.MessageID)
	})
	return cmp, entries, err
}

func fmtCoordinate(dialog int64, id int) string {
	return strconv.FormatInt(dialog, 10) + ":" + strconv.Itoa(id)
}

func (s *DownloadService) CompareLocalMedia(dir string, dialog int64, typ string, items []MediaItem) (manifest.Comparison, error) {
	cmp, _, err := s.compare(dir, dialog, typ, items)
	return cmp, err
}

func (s *DownloadService) ReviewLocalMedia(dir string, dialog int64, typ, title string, items []MediaItem) (manifest.Comparison, error) {
	owner := s.cfg.Get().LoggedInUserID
	cmp, entries, err := s.compare(dir, dialog, typ, items)
	if err != nil {
		return cmp, err
	}
	if owner != s.cfg.Get().LoggedInUserID {
		return cmp, errors.New("账号已切换，请重新扫描")
	}
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return cmp, err
	}
	cmp.Token = hex.EncodeToString(b[:])
	s.reviewsMu.Lock()
	defer s.reviewsMu.Unlock()
	if s.reviews == nil {
		s.reviews = map[string]mediaReview{}
	}
	for token, r := range s.reviews {
		if time.Since(r.at) > 30*time.Minute {
			delete(s.reviews, token)
		}
	}
	if len(s.reviews) >= 8 {
		oldest := ""
		for token, r := range s.reviews {
			if oldest == "" || r.at.Before(s.reviews[oldest].at) {
				oldest = token
			}
		}
		delete(s.reviews, oldest)
	}
	s.reviews[cmp.Token] = mediaReview{owner: owner, dialog: dialog, typ: typ, title: title, dir: dir, entries: entries, comparison: cmp, at: time.Now()}
	return cmp, nil
}

func (s *DownloadService) ExportReviewedManifest(token, dir string, choices []manifest.LocalChoice) (manifest.Info, error) {
	var empty manifest.Info
	s.reviewsMu.Lock()
	r, ok := s.reviews[token]
	s.reviewsMu.Unlock()
	if !ok || time.Since(r.at) > 30*time.Minute || r.owner != s.cfg.Get().LoggedInUserID {
		return empty, errors.New("扫描结果已失效，请重新扫描")
	}
	// Re-scan before export; deleted files, changed sizes and newly introduced
	// duplicate names invalidate earlier automatic matches.
	known, err := s.manager.KnownLocalMatches(r.dir)
	if err != nil {
		return empty, err
	}
	cmp, err := manifest.Compare(r.dir, r.entries, known, func(e manifest.Entry) string {
		return engine.PeerNamespace(r.typ) + ":" + fmtCoordinate(r.dialog, e.MessageID)
	})
	if err != nil {
		return empty, err
	}
	confirmed := map[int]manifest.Entry{}
	for _, m := range cmp.Matches {
		confirmed[m.Entry.MessageID] = m.Entry
	}
	byID := map[int]manifest.Entry{}
	for _, e := range r.entries {
		byID[e.MessageID] = e
	}
	root, err := os.OpenRoot(r.dir)
	if err != nil {
		return empty, err
	}
	defer root.Close()
	usedChoices := map[int]bool{}
	for _, choice := range choices {
		e, ok := byID[choice.MessageID]
		if !ok || !choice.Confirmed || usedChoices[choice.MessageID] || choice.MediaID != e.MediaID || choice.Date != e.Date {
			return empty, errors.New("重名确认缺少对应消息、日期或明确确认")
		}
		usedChoices[choice.MessageID] = true
		allowed := false
		for _, group := range cmp.Duplicates {
			if group.Name != e.Name {
				continue
			}
			for _, f := range group.Files {
				if f.Path == choice.Path && f.Size == e.Size {
					allowed = true
				}
			}
		}
		if !allowed || !manifest.RegularFile(root, choice.Path, e.Size) {
			return empty, errors.New("确认文件已变化或不属于此次扫描的重名组")
		}
		e.State = "done"
		e.Evidence = "manual"
		e.SavedName = filepath.Base(choice.Path)
		confirmed[e.MessageID] = e
	}
	if len(confirmed) == 0 {
		return empty, errors.New("没有确认匹配项，不生成清单")
	}
	entries := make([]manifest.Entry, 0, len(confirmed))
	for _, e := range r.entries {
		if match, ok := confirmed[e.MessageID]; ok {
			entries = append(entries, match)
		}
	}
	if s.cfg.Get().LoggedInUserID != r.owner {
		return empty, errors.New("账号已切换，请重新扫描")
	}
	info, err := s.manager.Manifests().Create(dir, manifest.Header{AccountID: r.owner, DialogID: r.dialog, DialogType: r.typ, Title: r.title}, entries)
	if err == nil {
		_, _ = s.cfg.AddRecentDir("manifest", dir)
	}
	return info, err
}

func (s *DownloadService) PreviewDownload(dir string, dialog int64, typ string, items []MediaItem, path string) (DownloadPreview, error) {
	cmp, entries, err := s.compare(dir, dialog, typ, items)
	p := DownloadPreview{Total: len(entries), Comparison: cmp, ManifestMessageIDs: []int{}}
	if err != nil {
		return p, err
	}
	var session *manifest.Session
	if path != "" {
		session, err = s.manager.Manifests().Open(path, s.cfg.Get().LoggedInUserID)
		if err != nil {
			return p, err
		}
		if !session.MatchesSource(dialog, typ) {
			return p, errors.New("清单与当前对话不匹配")
		}
	}
	local := map[int]bool{}
	for _, match := range cmp.Matches {
		if match.Entry.Evidence == "source" {
			local[match.Entry.MessageID] = true
		}
	}
	for _, e := range entries {
		if session != nil && session.ShouldSkip(dialog, typ, e.MessageID, e.MediaID, e.Size, "completed") {
			p.ManifestSkipped++
			p.ManifestMessageIDs = append(p.ManifestMessageIDs, e.MessageID)
		} else if local[e.MessageID] {
			p.LocalMatched++
		} else {
			p.Pending++
		}
	}
	return p, nil
}

func (s *DownloadService) NewDownloadManifest(dir string, dialog int64, typ, title string) (manifest.Info, error) {
	return s.manager.Manifests().Create(dir, manifest.Header{AccountID: s.cfg.Get().LoggedInUserID, DialogID: dialog, DialogType: typ, Title: title}, nil)
}
func (s *DownloadService) ReadManifest(path string) (manifest.Info, error) {
	session, err := s.manager.Manifests().Open(path, s.cfg.Get().LoggedInUserID)
	if err != nil {
		return manifest.Info{}, err
	}
	return session.Info(), nil
}
func (s *DownloadService) ChooseManifest() (*manifest.Info, error) {
	ctx := s.emitter.Ctx()
	if ctx == nil {
		return nil, errors.New("应用尚未就绪")
	}
	path, err := runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{Title: "导入已下载清单", Filters: []runtime.FileFilter{{DisplayName: "媒体清单 (*.jsonl;*.json)", Pattern: "*.jsonl;*.json"}}})
	if err != nil || path == "" {
		return nil, err
	}
	info, err := s.ReadManifest(path)
	if err != nil {
		return nil, err
	}
	return &info, nil
}
