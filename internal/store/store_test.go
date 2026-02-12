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
	if f.Owner != "" {
		t.Error("expected empty owner filter")
	}
}

func TestTaskFields(t *testing.T) {
	task := Task{
		Owner:     "550e8400-e29b-41d4-a716-446655440000",
		Submitter: "main-agent",
	}
	if task.Owner == "" {
		t.Error("expected owner to be set")
	}
	if task.Submitter == "" {
		t.Error("expected submitter to be set")
	}
}
