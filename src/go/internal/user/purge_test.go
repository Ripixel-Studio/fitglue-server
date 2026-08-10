package user

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/fitglue/server/src/go/internal/infra"
)

type fakeStorage struct {
	objects map[string][]string // bucket -> object names
	deleted []string            // "bucket/prefix" in deletion order
	failOn  string              // prefix that returns an error
}

func (f *fakeStorage) ListByPrefix(_ context.Context, bucket, prefix string) ([]string, error) {
	var out []string
	for _, n := range f.objects[bucket] {
		if strings.HasPrefix(n, prefix) {
			out = append(out, n)
		}
	}
	return out, nil
}

func (f *fakeStorage) DeleteByPrefix(_ context.Context, bucket, prefix string) (int, error) {
	if prefix == f.failOn {
		return 0, fmt.Errorf("boom")
	}
	f.deleted = append(f.deleted, bucket+"/"+prefix)
	n := 0
	var keep []string
	for _, name := range f.objects[bucket] {
		if strings.HasPrefix(name, prefix) {
			n++
		} else {
			keep = append(keep, name)
		}
	}
	f.objects[bucket] = keep
	return n, nil
}

func testPurger(fs *fakeStorage) *ArtifactPurger {
	return NewArtifactPurger(fs, "artifacts", "showcase-assets", infra.NewLoggerWithComponent("test"))
}

func TestPurgeUserDeletesAllUserPrefixes(t *testing.T) {
	fs := &fakeStorage{objects: map[string][]string{
		"artifacts": {
			"activities/u1/a.fit",
			"enriched_events/u1/exec-1.json",
			"enriched_events/u1/exec-2.json",
			"payloads/u1/act1.json",
			"activities/u2/other.fit", // another user — untouched
		},
		"showcase-assets": {
			"showcase_data/u1/sc1_data.json",
			"activity_photos/u1/act1/p.jpg",
			"exec-1/route-thumbnail.svg",     // from enriched_events listing
			"exec-sc/route-thumbnail.svg",    // from showcase docs only
			"exec-other/route-thumbnail.svg", // another user's execution
		},
	}}
	p := testPurger(fs)

	if err := p.PurgeUser(context.Background(), "u1", []string{"exec-sc", ""}); err != nil {
		t.Fatalf("PurgeUser() error = %v", err)
	}

	var remaining []string
	for b, names := range fs.objects {
		for _, n := range names {
			remaining = append(remaining, b+"/"+n)
		}
	}
	sort.Strings(remaining)
	want := []string{
		"artifacts/activities/u2/other.fit",
		"showcase-assets/exec-other/route-thumbnail.svg",
	}
	if len(remaining) != len(want) {
		t.Fatalf("remaining = %v, want %v", remaining, want)
	}
	for i := range want {
		if remaining[i] != want[i] {
			t.Errorf("remaining[%d] = %q, want %q", i, remaining[i], want[i])
		}
	}
}

func TestPurgeUserPropagatesFailure(t *testing.T) {
	fs := &fakeStorage{
		objects: map[string][]string{"artifacts": {"payloads/u1/x.json"}},
		failOn:  "payloads/u1/",
	}
	if err := testPurger(fs).PurgeUser(context.Background(), "u1", nil); err == nil {
		t.Fatal("PurgeUser() error = nil, want propagated failure")
	}
}

func TestPurgeUserRequiresUserID(t *testing.T) {
	if err := testPurger(&fakeStorage{objects: map[string][]string{}}).PurgeUser(context.Background(), "", nil); err == nil {
		t.Fatal("PurgeUser() with empty userID should error")
	}
}
