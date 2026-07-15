package taxonomy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func TestRefreshUsesCachedRevisionAndMarksUnchangedSuccess(t *testing.T) {
	t.Parallel()
	cached := contracts.TaxonomySnapshot{ProviderRef: contracts.ProviderRef{ID: "h1", Type: contracts.ProviderHugo}, Revision: "r1", Complete: true, Terms: []contracts.TaxonomyTerm{{Kind: "tag", Key: "go", Name: "Go"}}}
	store := &memoryStore{snapshot: cached, found: true}
	provider := &fakeProvider{snapshot: contracts.TaxonomySnapshot{ProviderRef: cached.ProviderRef, Revision: "r1", Complete: true, Unchanged: true}}
	service := NewService(store, func() time.Time { return time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC) })
	result, err := service.Refresh(context.Background(), "w1", cached.ProviderRef, provider)
	if err != nil {
		t.Fatal(err)
	}
	if provider.cursor.Revision != "r1" || !store.success || store.replaced || len(result.Terms) != 1 {
		t.Fatalf("未正确复用 taxonomy 缓存: result=%+v store=%+v cursor=%+v", result, store, provider.cursor)
	}
}

func TestRefreshFailureKeepsCachedSnapshotAndRecordsDiagnostic(t *testing.T) {
	t.Parallel()
	cached := contracts.TaxonomySnapshot{ProviderRef: contracts.ProviderRef{ID: "h1", Type: contracts.ProviderHugo}, Revision: "r1", Complete: true, Terms: []contracts.TaxonomyTerm{{Kind: "tag", Key: "go"}}}
	store := &memoryStore{snapshot: cached, found: true}
	provider := &fakeProvider{err: errors.New("读取失败")}
	result, err := NewService(store, time.Now).Refresh(context.Background(), "w1", cached.ProviderRef, provider)
	if err == nil || result.Revision != "r1" || store.failureCode == "" || store.replaced {
		t.Fatalf("失败刷新未保留缓存: result=%+v err=%v store=%+v", result, err, store)
	}
}

func TestPlanAndApplyChangePersistProviderSnapshot(t *testing.T) {
	t.Parallel()
	ref := contracts.ProviderRef{ID: "h1", Type: contracts.ProviderHugo}
	store := &memoryStore{}
	provider := &fakeProvider{
		change:  contracts.TaxonomyChangeSet{ProviderRef: ref, ExpectedRevision: "r1", Files: []contracts.TaxonomyFileChange{{RelativePath: "content/categories/go/_index.md", After: "---\ntitle: Go\n---\n"}}},
		applied: contracts.TaxonomySnapshot{ProviderRef: ref, Revision: "r2", Complete: true, Terms: []contracts.TaxonomyTerm{{Kind: "category", Key: "go", Name: "Go", CanonicalName: "Go"}}},
	}
	service := NewService(store, time.Now)
	command := contracts.TaxonomyCommand{Kind: contracts.TaxonomyCreateTerm, ExpectedRevision: "r1", Term: contracts.TaxonomyTerm{Kind: "category", Key: "go", Name: "Go"}}
	change, err := service.PlanChange(context.Background(), ref, provider, command)
	if err != nil || len(change.Files) != 1 {
		t.Fatalf("规划 taxonomy 变更: change=%+v err=%v", change, err)
	}
	snapshot, err := service.ApplyChange(context.Background(), "w1", ref, provider, command)
	if err != nil || !store.replaced || snapshot.Revision != "r2" || provider.planCalls != 2 || provider.applyCalls != 1 {
		t.Fatalf("应用 taxonomy 变更: snapshot=%+v err=%v store=%+v provider=%+v", snapshot, err, store, provider)
	}
}

type memoryStore struct {
	snapshot    contracts.TaxonomySnapshot
	found       bool
	replaced    bool
	success     bool
	failureCode string
}

func (s *memoryStore) FindSnapshot(context.Context, string, string) (contracts.TaxonomySnapshot, bool, error) {
	return s.snapshot, s.found, nil
}
func (s *memoryStore) ReplaceSnapshot(_ context.Context, _ string, snapshot contracts.TaxonomySnapshot, _ time.Time) error {
	s.snapshot, s.replaced = snapshot, true
	return nil
}
func (s *memoryStore) MarkRefreshSucceeded(context.Context, string, string, time.Time) error {
	s.success = true
	return nil
}
func (s *memoryStore) MarkRefreshFailed(_ context.Context, _, _, code, _ string, _ time.Time) error {
	s.failureCode = code
	return nil
}

type fakeProvider struct {
	snapshot   contracts.TaxonomySnapshot
	err        error
	cursor     contracts.TaxonomyCursor
	change     contracts.TaxonomyChangeSet
	applied    contracts.TaxonomySnapshot
	planCalls  int
	applyCalls int
}

func (p *fakeProvider) Descriptor() contracts.TaxonomyDescriptor {
	return contracts.TaxonomyDescriptor{Descriptor: contracts.Descriptor{Type: contracts.ProviderHugo}, Writable: true}
}
func (*fakeProvider) Validate(context.Context) error { return nil }
func (p *fakeProvider) Discover(_ context.Context, cursor contracts.TaxonomyCursor) (contracts.TaxonomySnapshot, error) {
	p.cursor = cursor
	return p.snapshot, p.err
}
func (p *fakeProvider) PlanChange(context.Context, contracts.TaxonomyCommand) (contracts.TaxonomyChangeSet, error) {
	p.planCalls++
	return p.change, nil
}
func (p *fakeProvider) ApplyChange(context.Context, contracts.TaxonomyChangeSet) (contracts.TaxonomySnapshot, error) {
	p.applyCalls++
	return p.applied, nil
}
func (*fakeProvider) Watch(context.Context, chan<- contracts.TaxonomyChange) error { return nil }
