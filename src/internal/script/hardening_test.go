package script

import (
	"tdl-ui/internal/scriptapi"
	"testing"
	"time"
)

func TestInitializationLoopTimesOut(t *testing.T) {
	previous := loadTimeout
	loadTimeout = 100 * time.Millisecond
	defer func() { loadTimeout = previous }()
	start := time.Now()
	_, err := Load("package main\nfunc init(){for{}}\nfunc Filter(f interface{})bool{return true}")
	if err == nil || time.Since(start) > 2*time.Second {
		t.Fatal("unbounded script initialization")
	}
}

func TestSandboxBlocksFilesystemEnumerationAndBackgroundWork(t *testing.T) {
	for _, src := range []string{
		`package main; import "path/filepath"; func Filter(f interface{})bool { _,_ = filepath.Glob("*"); return true }`,
		`package main; import "time"; func Filter(f interface{})bool { time.Sleep(time.Hour); return true }`,
		`package main; func Filter(f interface{})bool { go func(){for{}}(); return true }`,
		`package main; import "example.org/local"; func Filter(f interface{})bool { return local.Secret }`,
	} {
		if _, err := Load(src); err == nil {
			t.Fatal("script capability escaped allowlist")
		}
	}
}

func TestStatefulContractsKeepWorkingAcrossCalls(t *testing.T) {
	c, err := Load(`package main; import "tdlui/api"; var count int; func Filter(f api.FileInfo)bool { count++; return count%2==1 }; func Rename(f api.FileInfo)string { return f.FileName+".ok" }`)
	if err != nil {
		t.Fatal(err)
	}
	for idx := 0; idx < 4; idx++ {
		keep, err := c.SafeFilter(scriptapi.FileInfo{})
		if err != nil || keep != (idx%2 == 0) {
			t.Fatal("stateful filter changed")
		}
	}
	if name, err := c.SafeRename(scriptapi.FileInfo{FileName: "x"}); err != nil || name != "x.ok" {
		t.Fatal(name, err)
	}
}
