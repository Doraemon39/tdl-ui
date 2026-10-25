// Package manifest stores local, credential-free records of completed media.
package manifest

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"tdl-ui/internal/safeio"
)

const Version = 3
const MaxEntries = 100000
const maxBytes = 64 << 20

type Header struct {
	Version    int    `json:"version"`
	AccountID  int64  `json:"accountId"`
	DialogID   int64  `json:"dialogId"`
	DialogType string `json:"dialogType"`
	Title      string `json:"title"`
	CreatedAt  string `json:"createdAt"`
}

type Entry struct {
	SHA256         string `json:"sha256,omitempty"`
	MessageID      int    `json:"messageId"`
	MediaID        string `json:"mediaId"`
	Name           string `json:"name"`
	Size           int64  `json:"size"`
	Date           int64  `json:"date"`
	SavedName      string `json:"savedName,omitempty"`
	State          string `json:"state"`
	Evidence       string `json:"evidence"`
	DuplicateCount int    `json:"duplicateCount,omitempty"`
}

type document struct {
	Header  Header  `json:"header"`
	Entries []Entry `json:"entries"`
}
type Info struct {
	records int
	Path    string  `json:"path"`
	Header  Header  `json:"header"`
	Entries []Entry `json:"entries"`
	Total   int     `json:"total"`
	Done    int     `json:"done"`
	Ignored int     `json:"ignored"`
}

func Namespace(typ string) string {
	switch typ {
	case "private", "group", "channel":
		return typ
	case "supergroup":
		return "channel"
	}
	return ""
}

func validEntry(e Entry) bool {
	return e.MessageID > 0 && int64(e.MessageID) <= 2147483647 && e.MediaID != "" && e.Name != "" && len(e.Name) <= 4096 && e.Size >= 0 && e.Date > 0 && e.State == "done" && (e.Evidence == "source" || e.Evidence == "local_unique" || e.Evidence == "manual")
}

func read(path string, account int64) (Info, error) {
	var info Info
	if err := safeio.CheckPath(path); err != nil {
		return info, err
	}
	f, err := os.Open(path)
	if err != nil {
		return info, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return info, err
	}
	if !st.Mode().IsRegular() || st.Size() > maxBytes {
		return info, errors.New("清单必须是小于 64 MiB 的普通文件")
	}
	var doc document
	d := json.NewDecoder(io.LimitReader(f, maxBytes+1))
	if err := d.Decode(&doc); err != nil {
		return info, err
	}
	if doc.Header.Version == Version {
		return readJournal(path, account, doc.Header, io.MultiReader(d.Buffered(), f))
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return info, errors.New("清单含多余数据")
	}
	h := doc.Header
	if h.Version < 1 || h.Version > Version || h.AccountID <= 0 || h.AccountID != account || h.DialogID <= 0 || Namespace(h.DialogType) == "" {
		return info, errors.New("清单版本、账号或对话信息无效")
	}
	if len(doc.Entries) > MaxEntries {
		return info, errors.New("清单超过 100000 项上限")
	}
	info = Info{Path: path, Header: h, Entries: []Entry{}}
	seen := map[int]bool{}
	counts := map[int]int{}
	for _, e := range doc.Entries {
		counts[e.MessageID]++
	}
	for _, e := range doc.Entries {
		if h.Version < 2 || !validEntry(e) || seen[e.MessageID] || counts[e.MessageID] != 1 {
			info.Ignored++
			continue
		}
		seen[e.MessageID] = true
		info.Entries = append(info.Entries, e)
	}
	info.Total, info.Done = len(info.Entries), len(info.Entries)
	return info, nil
}

type Session struct {
	mu       sync.Mutex
	info     Info
	entries  map[int]Entry
	index    map[int]int
	size     int64
	modified time.Time
}
type Registry struct {
	mu       sync.Mutex
	sessions map[string]*Session
}

func NewRegistry() *Registry { return &Registry{sessions: map[string]*Session{}} }
func (r *Registry) Clear()   { r.mu.Lock(); r.sessions = map[string]*Session{}; r.mu.Unlock() }

func (r *Registry) Open(path string, account int64) (*Session, error) {
	if account <= 0 {
		return nil, errors.New("请先登录当前账号")
	}
	if !filepath.IsAbs(path) {
		return nil, errors.New("清单必须使用绝对路径")
	}
	path = filepath.Clean(path)
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, s := range r.sessions {
		if safeio.Equal(key, path) {
			s.mu.Lock()
			defer s.mu.Unlock()
			info, err := read(path, account)
			if err != nil {
				return nil, err
			}
			s.set(info)
			return s, nil
		}
	}
	info, err := read(path, account)
	if err != nil {
		return nil, err
	}
	s := &Session{}
	s.set(info)
	if len(r.sessions) >= 256 {
		return nil, errors.New("清单会话数量已达上限，请重启应用后重试")
	}
	r.sessions[path] = s
	return s, nil
}

func (s *Session) set(info Info) {
	s.info = info
	s.entries = map[int]Entry{}
	s.index = map[int]int{}
	for idx, e := range info.Entries {
		s.entries[e.MessageID] = e
		s.index[e.MessageID] = idx
	}
	if st, err := os.Stat(info.Path); err == nil {
		s.size = st.Size()
		s.modified = st.ModTime()
	}
}
func (s *Session) Info() Info {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.info
	i.Entries = append([]Entry{}, i.Entries...)
	return i
}

func (s *Session) Path() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.info.Path
}
func (s *Session) MatchesSource(dialog int64, typ string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return dialog == s.info.Header.DialogID && Namespace(typ) != "" && Namespace(typ) == Namespace(s.info.Header.DialogType)
}
func (s *Session) ShouldSkip(dialog int64, typ string, id int, media string, size int64, mode string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[id]
	return ok && dialog == s.info.Header.DialogID && Namespace(typ) != "" && Namespace(typ) == Namespace(s.info.Header.DialogType) && e.MediaID == media && media != "" && e.Size == size && validEntry(e)
}

// HasCompletion checks whether a task's already completed file needs a journal
// repair. It does not validate a new local-file match or read media contents.
func (s *Session) HasCompletion(dialog int64, typ string, entry Entry) bool {
	if s == nil || len(entry.SHA256) != 64 || !validEntry(entry) {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[entry.MessageID]
	return ok && dialog == s.info.Header.DialogID && Namespace(typ) != "" && Namespace(typ) == Namespace(s.info.Header.DialogType) && validEntry(e) &&
		e.MediaID == entry.MediaID && e.Name == entry.Name && e.Size == entry.Size && e.Date == entry.Date && e.SHA256 == entry.SHA256 && e.SavedName == entry.SavedName
}

func (s *Session) MarkDone(e Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e.State = "done"
	if e.Evidence == "" {
		e.Evidence = "source"
	}
	if !validEntry(e) {
		return errors.New("完成记录缺少有效媒体来源")
	}
	return s.appendRecord(e)
}

// Create chooses a fresh name and never overwrites an existing user file.
func (r *Registry) Create(dir string, h Header, entries []Entry) (Info, error) {
	return r.create(dir, h, entries, "")
}

// OpenTask keeps automatic completion journals stable across pause/resume and
// application restarts without needing a second source of task metadata.
func (r *Registry) OpenTask(dir, taskID string, h Header) (*Session, error) {
	return r.openTask(dir, taskID, h, true)
}

// OpenExistingTask never creates an empty journal during a resume check.
func (r *Registry) OpenExistingTask(dir, taskID string, h Header) (*Session, error) {
	return r.openTask(dir, taskID, h, false)
}

func (r *Registry) openTask(dir, taskID string, h Header, create bool) (*Session, error) {
	if taskID == "" || filepath.Base(taskID) != taskID || !filepath.IsLocal(taskID) {
		return nil, errors.New("自动清单任务标识无效")
	}
	name := "tdl-media-" + Namespace(h.DialogType) + "-" + strconv.FormatInt(h.DialogID, 10) + "-" + taskID + ".jsonl"
	path := filepath.Join(dir, name)
	s, err := r.Open(path, h.AccountID)
	if os.IsNotExist(err) && create {
		_, err = r.create(dir, h, nil, name)
		if err != nil && !os.IsExist(err) {
			return nil, err
		}
		s, err = r.Open(path, h.AccountID)
	}
	if err != nil {
		return nil, err
	}
	if !s.MatchesSource(h.DialogID, h.DialogType) {
		return nil, errors.New("自动清单来源不一致，拒绝追加")
	}
	return s, nil
}

func (r *Registry) create(dir string, h Header, entries []Entry, name string) (Info, error) {
	var empty Info
	if err := safeio.CheckPath(dir); err != nil {
		return empty, err
	}
	if h.AccountID <= 0 || h.DialogID <= 0 || Namespace(h.DialogType) == "" {
		return empty, errors.New("清单来源无效")
	}
	h.Version = Version
	h.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	if entries == nil {
		entries = []Entry{}
	}
	if len(entries) > MaxEntries {
		return empty, errors.New("清单项目过多")
	}
	for _, e := range entries {
		if !validEntry(e) {
			return empty, errors.New("仅可导出确认匹配的完成记录")
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return empty, err
	}
	b, err := marshalJournal(h, entries)
	if err != nil {
		return empty, err
	}
	var f *os.File
	if name == "" {
		f, err = os.CreateTemp(dir, "tdl-media-"+strconv.FormatInt(h.DialogID, 10)+"-*.jsonl")
	} else {
		f, err = os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	}
	if err != nil {
		return empty, err
	}
	path := f.Name()
	ok := false
	defer func() {
		if !ok {
			os.Remove(path)
		}
	}()
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	cerr := f.Close()
	if err != nil {
		return empty, err
	}
	if cerr != nil {
		return empty, cerr
	}
	s, err := r.Open(path, h.AccountID)
	if err != nil {
		return empty, err
	}
	ok = true
	return s.Info(), nil
}

func RegularFile(root *os.Root, path string, size int64) bool {
	if root == nil || !filepath.IsLocal(path) {
		return false
	}
	st, err := root.Lstat(path)
	return err == nil && st.Mode().IsRegular() && st.Size() == size
}
