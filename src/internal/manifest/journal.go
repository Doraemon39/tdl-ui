package manifest

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"

	"tdl-ui/internal/applock"
	"tdl-ui/internal/safeio"
)

func marshalJournal(h Header, entries []Entry) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	h.Version = Version
	if err := enc.Encode(document{Header: h}); err != nil {
		return nil, err
	}
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			return nil, err
		}
	}
	if b.Len() > maxBytes {
		return nil, errors.New("清单超过 64 MiB 上限")
	}
	return b.Bytes(), nil
}

func readJournal(path string, owner int64, h Header, reader io.Reader) (Info, error) {
	info := Info{Path: path, Header: h, Entries: []Entry{}}
	if h.AccountID <= 0 || h.AccountID != owner || h.DialogID <= 0 || Namespace(h.DialogType) == "" {
		return info, errors.New("清单账号或对话无效")
	}
	index := map[int]int{}
	scan := bufio.NewScanner(reader)
	scan.Buffer(make([]byte, 4096), 1<<20)
	for scan.Scan() {
		line := bytes.TrimSpace(scan.Bytes())
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil || !validEntry(e) {
			info.Ignored++
			continue
		}
		if idx, ok := index[e.MessageID]; ok {
			info.Entries[idx] = e
		} else {
			if len(info.Entries) >= MaxEntries {
				return info, errors.New("清单超过 100000 项上限")
			}
			index[e.MessageID] = len(info.Entries)
			info.Entries = append(info.Entries, e)
		}
		info.records++
	}
	if err := scan.Err(); err != nil {
		return info, err
	}
	info.Total = len(info.Entries)
	info.Done = info.Total
	return info, nil
}

// New manifests use one complete JSON record per line. Appending a completed
// item is O(1); interrupted trailing records are ignored and compacted before
// the next write. A per-file OS lock also coordinates distinct app instances.
func (s *Session) appendRecord(e Entry) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	guard, err := applock.AcquireFile(ctx, s.info.Path+".lock")
	if err != nil {
		return err
	}
	defer guard.Close()
	if err := safeio.CheckPath(s.info.Path); err != nil {
		return err
	}
	st, err := os.Stat(s.info.Path)
	if err != nil {
		return err
	}
	if st.Size() != s.size || !st.ModTime().Equal(s.modified) {
		current, err := read(s.info.Path, s.info.Header.AccountID)
		if err != nil {
			return err
		}
		if current.Header.DialogID != s.info.Header.DialogID || Namespace(current.Header.DialogType) != Namespace(s.info.Header.DialogType) {
			return errors.New("清单来源已改变，拒绝写入")
		}
		s.set(current)
	}
	lastFile, err := os.Open(s.info.Path)
	if err != nil {
		return err
	}
	var tail [1]byte
	_, tailErr := lastFile.ReadAt(tail[:], st.Size()-1)
	lastFile.Close()
	needsRepair := tailErr != nil || tail[0] != '\n'
	if s.info.Header.Version != Version || s.info.Ignored > 0 || needsRepair || s.size > maxBytes*3/4 && s.info.records > 2*max(100, len(s.info.Entries)) {
		b, err := marshalJournal(s.info.Header, s.info.Entries)
		if err != nil {
			return err
		}
		if err := safeio.WriteFile(s.info.Path, b, 0o600); err != nil {
			return err
		}
		s.info.Header.Version = Version
		s.info.Ignored = 0
		s.info.records = len(s.info.Entries)
		s.set(s.info)
	}
	if old, ok := s.entries[e.MessageID]; ok && old == e {
		return nil
	}
	if _, ok := s.entries[e.MessageID]; !ok && len(s.entries) >= MaxEntries {
		return errors.New("清单已达到上限")
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if s.size+int64(len(b)) > maxBytes {
		return errors.New("清单超过 64 MiB 上限")
	}
	f, err := os.OpenFile(s.info.Path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	n, err := f.Write(b)
	if err == nil && n != len(b) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = f.Sync()
	}
	latest, statErr := f.Stat()
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if statErr != nil {
		return statErr
	}
	if closeErr != nil {
		return closeErr
	}
	if idx, ok := s.index[e.MessageID]; ok {
		s.info.Entries[idx] = e
	} else {
		s.index[e.MessageID] = len(s.info.Entries)
		s.info.Entries = append(s.info.Entries, e)
	}
	s.entries[e.MessageID] = e
	s.info.Total = len(s.info.Entries)
	s.info.Done = s.info.Total
	s.info.records++
	s.size = latest.Size()
	s.modified = latest.ModTime()
	return nil
}
