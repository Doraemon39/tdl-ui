package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"tdl-ui/internal/safeio"
)

type LocalMatch struct {
	Entry Entry  `json:"entry"`
	Path  string `json:"path"`
}
type LocalFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
	Date int64  `json:"date"`
}
type DuplicateGroup struct {
	Name    string      `json:"name"`
	Entries []Entry     `json:"entries"`
	Files   []LocalFile `json:"files"`
}
type LocalChoice struct {
	MessageID int    `json:"messageId"`
	Path      string `json:"path"`
	MediaID   string `json:"mediaId"`
	Date      int64  `json:"date"`
	Confirmed bool   `json:"confirmed"`
}
type Comparison struct {
	Token      string           `json:"token"`
	Total      int              `json:"total"`
	Matched    int              `json:"matched"`
	Unmatched  int              `json:"unmatched"`
	Ambiguous  int              `json:"ambiguous"`
	Matches    []LocalMatch     `json:"matches"`
	Duplicates []DuplicateGroup `json:"duplicates"`
}

// Compare uses exact original basenames and byte sizes. Duplicate source names
// are never inferred, even when just one local file has a matching size.
func Compare(dir string, entries []Entry, known map[string]LocalMatch, key func(Entry) string) (Comparison, error) {
	out := Comparison{Total: len(entries), Matches: []LocalMatch{}, Duplicates: []DuplicateGroup{}}
	if len(entries) > MaxEntries {
		return out, errors.New("媒体数量超过上限")
	}
	if err := safeio.CheckPath(dir); err != nil {
		return out, err
	}
	root, err := os.OpenRoot(dir)
	if os.IsNotExist(err) {
		out.Unmatched = len(entries)
		return out, nil
	}
	if err != nil {
		return out, err
	}
	defer root.Close()
	byName := map[string][]LocalFile{}
	count := 0
	err = fs.WalkDir(root.FS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		count++
		if count > 200000 {
			return errors.New("本地目录超过 200000 项，请选择更小的目录")
		}
		if err := safeio.CheckPath(filepath.Join(dir, path)); err != nil {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		st, err := d.Info()
		if err != nil {
			return err
		}
		if !st.Mode().IsRegular() {
			return nil
		}
		byName[d.Name()] = append(byName[d.Name()], LocalFile{path, st.Size(), st.ModTime().Unix()})
		return nil
	})
	if err != nil {
		return out, err
	}
	groups := map[string][]Entry{}
	for _, e := range entries {
		groups[e.Name] = append(groups[e.Name], e)
	}
	var names []string
	for n := range groups {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		group, files := groups[name], byName[name]
		ambiguous := len(group) > 1 || len(files) > 1
		if ambiguous {
			out.Duplicates = append(out.Duplicates, DuplicateGroup{name, group, files})
		}
		for _, e := range group {
			if match, ok := known[key(e)]; ok && match.Entry.MediaID == e.MediaID && match.Entry.Name == e.Name && match.Entry.Size == e.Size && VerifyDigest(root, match.Path, e.Size, match.Entry.SHA256) {
				sum := match.Entry.SHA256
				match.Entry = e
				match.Entry.SHA256 = sum
				match.Entry.State = "done"
				match.Entry.Evidence = "source"
				out.Matches = append(out.Matches, match)
				continue
			}
			if !ambiguous && len(files) == 1 && files[0].Size == e.Size {
				e.State = "done"
				e.Evidence = "local_unique"
				e.SavedName = filepath.Base(files[0].Path)
				out.Matches = append(out.Matches, LocalMatch{e, files[0].Path})
			} else if ambiguous {
				out.Ambiguous++
			} else {
				out.Unmatched++
			}
		}
	}
	out.Matched = len(out.Matches)
	return out, nil
}

func EntryKey(e Entry) string { return strconv.Itoa(e.MessageID) }

func VerifyDigest(root *os.Root, path string, size int64, digest string) bool {
	if len(digest) != 64 || !RegularFile(root, path, size) {
		return false
	}
	f, err := root.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	return err == nil && n == size && hex.EncodeToString(h.Sum(nil)) == digest
}
