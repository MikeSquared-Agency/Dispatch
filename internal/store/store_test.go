package store

import (
	"testing"
)

func TestTaskStatusValues(t *testing.T) {
	statuses := []TaskStatus{
		StatusPending, StatusAssigned, StatusInProgress,
		StatusCompleted, StatusFailed, StatusTimedOut,
	}
	expected := []string{"pending", "assigned", "in_progress", "completed", "failed", "timed_out"}
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
		Owner:  "mike-d",
		Source: "kai",
	}
	if task.Owner == "" {
		t.Error("expected owner to be set")
	}
	if task.Source == "" {
		t.Error("expected source to be set")
	}
}
