package disposition

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestServiceApplyNormalizesAndValidatesBatch(t *testing.T) {
	t.Parallel()
	store := &capturingStore{result: Result{Processed: 2, Changed: 2}}
	service := NewService(store)
	got, err := service.Apply(context.Background(), Command{
		Operation: OperationPublished,
		Articles: []ArticleVersion{
			{ID: "a2", ContentVersion: "v2"},
			{ID: "a1", ContentVersion: "v1"},
			{ID: "a1", ContentVersion: "v1"},
		},
		Channels: []string{"wechat", "hugo", "wechat"},
	})
	if err != nil || got.Processed != 2 {
		t.Fatalf("Apply() = %+v, %v", got, err)
	}
	wantArticles := []ArticleVersion{{ID: "a1", ContentVersion: "v1"}, {ID: "a2", ContentVersion: "v2"}}
	if !reflect.DeepEqual(store.command.Articles, wantArticles) || !reflect.DeepEqual(store.command.Channels, []string{"hugo", "wechat"}) {
		t.Fatalf("normalized command = %+v", store.command)
	}
}

func TestServiceApplyRejectsInvalidCommands(t *testing.T) {
	t.Parallel()
	tooMany := make([]ArticleVersion, 101)
	for index := range tooMany {
		tooMany[index] = ArticleVersion{ID: string(rune(index + 1)), ContentVersion: "v"}
	}
	tests := []Command{
		{},
		{Operation: OperationIgnored, Articles: tooMany},
		{Operation: OperationIgnored, Articles: []ArticleVersion{{ID: "a1"}}},
		{Operation: "unknown", Articles: []ArticleVersion{{ID: "a1", ContentVersion: "v1"}}},
		{Operation: OperationPublished, Articles: []ArticleVersion{{ID: "a1", ContentVersion: "v1"}}},
		{Operation: OperationIgnored, Articles: []ArticleVersion{{ID: "a1", ContentVersion: "v1"}}, Channels: []string{"hugo"}},
		{Operation: OperationRestore, Articles: []ArticleVersion{{ID: "a1", ContentVersion: "v1"}}, Channels: []string{"wechat"}},
		{Operation: OperationPublished, Articles: []ArticleVersion{{ID: "a1", ContentVersion: "v1"}}, Channels: []string{"email"}},
	}
	for _, command := range tests {
		_, err := NewService(&capturingStore{}).Apply(context.Background(), command)
		if !errors.Is(err, ErrInvalidCommand) {
			t.Fatalf("Apply(%+v) error = %v", command, err)
		}
	}
}

type capturingStore struct {
	command Command
	result  Result
	err     error
}

func (s *capturingStore) Apply(_ context.Context, command Command) (Result, error) {
	s.command = command
	return s.result, s.err
}
