package applock

import (
	"context"
	"testing"
	"time"
)

func TestExclusiveAndRelease(t *testing.T) {
	dir := t.TempDir()
	g, err := Acquire(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if other, err := Acquire(ctx, dir); err == nil {
		other.Close()
		t.Fatal("second owner acquired live data directory")
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	next, err := Acquire(ctx2, dir)
	if err != nil {
		t.Fatal(err)
	}
	next.Close()
}
