package ethernetip

import (
	"testing"

	"github.com/anviod/edgeCore/internal/model"

	go_ethernet_ip "github.com/anviod/ethernet-ip"
)

func TestENIPScheduler_resolveLogixTagName(t *testing.T) {
	s := &ENIPScheduler{}
	cases := []struct {
		in, want string
	}{
		{"Program:Main.IntTag", "IntTag"},
		{"Controller:DintTag", "DintTag"},
		{"SimpleTag", "SimpleTag"},
	}
	for _, tc := range cases {
		if got := s.resolveLogixTagName(tc.in); got != tc.want {
			t.Fatalf("resolveLogixTagName(%q)=%q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestENIPScheduler_getLogixClass2AttrID(t *testing.T) {
	s := &ENIPScheduler{}
	id, ok := s.getLogixClass2AttrID("IntTag")
	if !ok || id != 3 {
		t.Fatalf("IntTag attr = %d ok=%v", id, ok)
	}
	if _, ok := s.getLogixClass2AttrID("UnknownTag"); ok {
		t.Fatal("expected unknown tag to miss")
	}
}

func TestNewENIPScheduler_BatchConfig(t *testing.T) {
	s := NewENIPScheduler(nil, nil, map[string]any{"batch_read_max": 80})
	if s.batchReadMax != 50 {
		t.Fatalf("batch_read_max should clamp to 50, got %d", s.batchReadMax)
	}
	s2 := NewENIPScheduler(nil, nil, map[string]any{"batch_read_max": 20})
	if s2.batchReadMax != 20 {
		t.Fatalf("batch_read_max = %d, want 20", s2.batchReadMax)
	}
}

func TestENIPScheduler_groupTags(t *testing.T) {
	s := NewENIPScheduler(nil, nil, map[string]any{"batch_read_max": 2})
	points := []pointWithTag{
		{Point: model.Point{ID: "a"}},
		{Point: model.Point{ID: "b"}},
		{Point: model.Point{ID: "c"}},
	}
	groups := s.groupTags(points)
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	if len(groups[0]) != 2 || len(groups[1]) != 1 {
		t.Fatalf("group sizes = %d, %d", len(groups[0]), len(groups[1]))
	}
}

func TestENIPScheduler_processTagValue_NilValue(t *testing.T) {
	s := &ENIPScheduler{}
	results := make(map[string]model.Value)

	s.processTagValue(tagWithPoint{
		tag:  &go_ethernet_ip.Tag{},
		pwt:  pointWithTag{Point: model.Point{ID: "pt-nil"}},
		name: "NilTag",
	}, results)

	val, ok := results["pt-nil"]
	if !ok {
		t.Fatal("expected result entry")
	}
	if val.Quality != "Bad" {
		t.Fatalf("quality = %q, want Bad", val.Quality)
	}
	_, _, failures := s.GetStats()
	if failures != 1 {
		t.Fatalf("failure count = %d, want 1", failures)
	}
}

func TestENIPScheduler_processTagValue_WithValue(t *testing.T) {
	tag := &go_ethernet_ip.Tag{}
	tag.Type = go_ethernet_ip.INT
	tag.SetValue([]byte{0x39, 0x30}) // 12345 little-endian

	s := &ENIPScheduler{}
	results := make(map[string]model.Value)
	s.processTagValue(tagWithPoint{
		tag:  tag,
		pwt:  pointWithTag{Point: model.Point{ID: "pt-int"}},
		name: "Program:Main.IntTag",
	}, results)

	val, ok := results["pt-int"]
	if !ok {
		t.Fatal("expected result entry")
	}
	if val.Quality != "Good" {
		t.Fatalf("quality = %q, want Good", val.Quality)
	}
	if val.Value == nil {
		t.Fatal("expected non-nil value")
	}
	if val.TS.IsZero() {
		t.Fatal("expected timestamp")
	}
	_, successes, _ := s.GetStats()
	if successes != 1 {
		t.Fatalf("success count = %d, want 1", successes)
	}
}
