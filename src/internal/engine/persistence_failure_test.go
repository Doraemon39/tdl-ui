package engine

import (
	"context"
	"errors"
	"testing"
)

type failingRemovalRepo struct{ TaskRepo }

func (f failingRemovalRepo) DeleteTask(context.Context, string) error {
	return errors.New("synthetic database failure")
}
func (f failingRemovalRepo) DeleteFilesByPath(context.Context, string, []string) error {
	return errors.New("synthetic database failure")
}

func TestFailedRecordRemovalKeepsTaskAndFileVisible(t *testing.T) {
	m := NewManager(Deps{})
	m.deps.Store = failingRemovalRepo{newMemRepo()}
	task := &Task{ID: "source", mgr: m, status: StatusDone, files: []TaskFile{{Path: "skipped://channel:7:1", State: "skipped"}}}
	m.tasks[task.ID] = task
	m.order = []string{task.ID}
	if err := m.Remove(task.ID); err == nil {
		t.Fatal("task delete falsely succeeded")
	}
	if _, err := m.get(task.ID); err != nil || task.removed {
		t.Fatal("failed removal hid the task")
	}
	if err := m.DeleteFileRecord(task.ID, task.files[0].Path); err == nil {
		t.Fatal("file record delete falsely succeeded")
	}
	if len(task.files) != 1 {
		t.Fatal("failed file record delete changed memory")
	}
	if err := m.ClearFinished(); err == nil {
		t.Fatal("bulk removal falsely succeeded")
	}
	if len(m.tasks) != 1 || task.removed {
		t.Fatal("bulk failure hid the task")
	}
}
