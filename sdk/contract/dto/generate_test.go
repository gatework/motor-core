package dto

import (
	"reflect"
	"testing"
)

func TestObjectByIDGetID(t *testing.T) {
	single := &ObjectByID{ID: 5}
	if got := single.GetID(); got != 5 {
		t.Fatalf("GetID() = %v, want 5", got)
	}

	batch := &ObjectByID{ID: 5, IDs: []int{1, 2}}
	got := batch.GetID()
	want := []int{1, 2, 5}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetID() = %v, want %v", got, want)
	}
}

func TestObjectGetReqGetID(t *testing.T) {
	req := &ObjectGetReq{ID: 9}
	if got := req.GetID(); got != 9 {
		t.Fatalf("GetID() = %v, want 9", got)
	}
}

func TestObjectDeleteReqGetID(t *testing.T) {
	req := &ObjectDeleteReq{IDs: []int{3, 4}}
	got := req.GetID()
	want := []int{3, 4}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetID() = %v, want %v", got, want)
	}
}
