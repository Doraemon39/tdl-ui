package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"tdl-ui/internal/manifest"
	"tdl-ui/internal/safeio"
)

var errOutputSkipped = errors.New("本地或队列已存在同名文件，已跳过")

type outputCoordinator struct {
	mu       sync.Mutex
	reserved map[string]bool
}

func (*outputCoordinator) invalidateOrigins() {}

func cleanOutputName(name string) (string, error) {
	if strings.TrimSpace(name) == "" || strings.ContainsAny(name, "\x00\r\n") || strings.Contains(name, ":") || strings.Contains(name, "\\") && runtime.GOOS != "windows" {
		return "", errors.New("文件名包含无效字符")
	}
	name = filepath.Clean(name)
	if !filepath.IsLocal(name) || name == "." {
		return "", errors.New("文件名超出下载目录")
	}
	for _, part := range strings.FieldsFunc(name, func(r rune) bool { return r == '/' || r == '\\' }) {
		if strings.ContainsAny(part, `<>:"|?*`) || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return "", errors.New("文件名包含不支持的字符或后缀")
		}
		stem := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || stem == "CONIN$" || stem == "CONOUT$" || len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && strings.ContainsRune("123456789¹²³", rune(stem[3])) {
			return "", errors.New("文件名是系统保留名称")
		}
	}
	return name, nil
}

func outputKey(dir, name string) string {
	key := filepath.Join(dir, name)
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	return key
}
func numbered(name string, n int) string {
	ext := filepath.Ext(name)
	return strings.TrimSuffix(name, ext) + fmt.Sprintf(" (%d)", n) + ext
}

func (i *iter) allocate(name, original string, actions ...string) (*os.File, string, error) {
	action := i.opts.ConflictAction
	if len(actions) > 0 {
		action = actions[0]
	}
	i.outputs.mu.Lock()
	defer i.outputs.mu.Unlock()
	if i.outputs.reserved == nil {
		i.outputs.reserved = map[string]bool{}
	}
	base := name
	for n := 0; n < 100000; n++ {
		if n > 0 {
			name = numbered(base, n)
		}
		key := outputKey(i.opts.Dir, name)
		st, err := i.root.Lstat(name)
		exists := err == nil
		if err != nil && !os.IsNotExist(err) {
			return nil, "", err
		}
		busy := i.outputs.reserved[key]
		if busy && action == "overwrite" {
			return nil, "", errors.New("目标文件正在由其他任务写入，请稍后重试")
		}
		if busy || exists && action != "overwrite" {
			if action == "skip" {
				return nil, name, errOutputSkipped
			}
			continue
		}
		if exists && !st.Mode().IsRegular() {
			return nil, "", errors.New("拒绝覆盖非普通文件")
		}
		if err := safeio.CheckPath(filepath.Join(i.opts.Dir, name)); err != nil {
			return nil, "", err
		}
		parent := filepath.Dir(name)
		if err := i.root.MkdirAll(parent, 0o755); err != nil {
			return nil, "", err
		}
		var b [12]byte
		if _, err := rand.Read(b[:]); err != nil {
			return nil, "", err
		}
		tmp := filepath.Join(parent, ".tdl-part-"+hex.EncodeToString(b[:])+tempExt)
		f, err := i.root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
		if err != nil {
			return nil, "", err
		}
		i.outputs.reserved[key] = true
		return f, name, nil
	}
	return nil, "", errors.New("同名文件过多，无法分配目标文件名")
}

func (i *iter) release(name string) {
	i.outputs.mu.Lock()
	delete(i.outputs.reserved, outputKey(i.opts.Dir, name))
	i.outputs.mu.Unlock()
}

func (i *iter) publish(e *iterElem, name string) (string, error) {
	action := e.outputAction
	if action == "" {
		action = i.opts.ConflictAction
	}
	name, err := cleanOutputName(name)
	if err != nil {
		return "", err
	}
	tmp, err := filepath.Rel(i.opts.Dir, e.to.Name())
	if err != nil || !filepath.IsLocal(tmp) {
		return "", errors.New("临时文件路径越界")
	}
	i.outputs.mu.Lock()
	defer i.outputs.mu.Unlock()
	base := name
	for n := 0; n < 100000; n++ {
		if n > 0 {
			name = numbered(base, n)
		}
		if name != e.targetName && i.outputs.reserved[outputKey(i.opts.Dir, name)] {
			continue
		}
		if err := safeio.CheckPath(filepath.Join(i.opts.Dir, name)); err != nil {
			return "", err
		}
		if action == "overwrite" {
			if st, err := i.root.Lstat(name); err == nil && !st.Mode().IsRegular() {
				return "", errors.New("拒绝覆盖非普通文件")
			} else if err != nil && !os.IsNotExist(err) {
				return "", err
			}
			err = i.root.Rename(tmp, name)
		} else {
			err = publishExclusive(i.root, i.opts.Dir, tmp, name)
		}
		if os.IsExist(err) {
			if action == "skip" {
				return "", errOutputSkipped
			}
			continue
		}
		if err != nil {
			return "", err
		}
		return filepath.Join(i.opts.Dir, name), nil
	}
	return "", errors.New("无法分配唯一的保存名称")
}

func (m *Manager) KnownLocalMatches(dir string) (map[string]manifest.LocalMatch, error) {
	out := map[string]manifest.LocalMatch{}
	if m.deps.Store == nil {
		return out, nil
	}
	account := int64(0)
	if m.deps.Cfg != nil {
		account = m.deps.Cfg.Get().LoggedInUserID
	}
	if account <= 0 {
		return out, nil
	}
	tasks, err := m.deps.Store.LoadAllTasks(context.Background())
	if err != nil {
		return nil, err
	}
	for _, t := range tasks {
		if !safeio.Equal(dir, t.Dir) {
			continue
		}
		files, err := m.deps.Store.ListFiles(context.Background(), t.ID)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			if f.State != "done" || f.AccountID != account || f.MediaID == "" || f.SHA256 == "" || f.OriginalName == "" || PeerNamespace(f.DialogType) == "" {
				continue
			}
			rel, err := filepath.Rel(dir, f.Path)
			if err != nil || !filepath.IsLocal(rel) {
				continue
			}
			out[resumeKey(f.DialogID, f.MessageID, f.DialogType)] = manifest.LocalMatch{Path: rel, Entry: manifest.Entry{SHA256: f.SHA256, MessageID: f.MessageID, MediaID: f.MediaID, Name: f.OriginalName, Size: f.Size, State: "done", Evidence: "source"}}
		}
	}
	return out, nil
}
