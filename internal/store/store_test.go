package store

import (
	"testing"
)

func TestTaskStatusValues(t *testing.T) {
	statuses := []TaskStatus{
		StatusPending, StatusAssigned, StatusRunning,
		StatusCompleted, StatusFailed, StatusCancelled, StatusTimeout,
	}
	expected := []string{"pending", "assigned", "running", "completed", "failed", "cancelled", "timeout"}
	for i, s := range statuses {
		if string(s) != expected[i] {
			t.Errorf("expected %s, got %s", expected[i], s)
		}
	}
}

func TestTaskFilterDefaults(t *testing.T) {
	f := TaskFilter{}
	if f.Limit != 0 {
		t.Errorf("expected 0 default limit, got %d", f.Limit)
	}
	if f.Status != nil {
		t.Error("expected nil status filter")
	}
}
