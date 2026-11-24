package memory

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func open(t *testing.T) *SQLite {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "robot.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSessionLifecycle(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	id, err := s.StartSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.AddMessage(ctx, id, Message{Role: "user", Content: "Расскажи про дубовые двери"})
	_ = s.AddMessage(ctx, id, Message{Role: "tool_call", Content: `{"query":"двери"}`, ToolName: "web_search"})
	_ = s.AddMessage(ctx, id, Message{Role: "assistant", Content: "Дуб прочный."})
	if err := s.EndSession(ctx, id, "Обсуждали дубовые двери."); err != nil {
		t.Fatal(err)
	}
	msgs, _ := s.Messages(ctx, id)
	if len(msgs) != 3 || msgs[1].ToolName != "web_search" {
		t.Fatalf("msgs=%+v", msgs)
	}
	list, _ := s.Sessions(ctx, 5)
	if len(list) != 1 || list[0].Messages != 3 || list[0].Summary == "" || list[0].Ended.IsZero() {
		t.Fatalf("sessions=%+v", list)
	}
}

func TestRecentSummariesOrderAndLimit(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	for _, sum := range []string{"первая", "вторая", "", "третья", "четвёртая"} {
		id, _ := s.StartSession(ctx)
		_ = s.EndSession(ctx, id, sum)
	}
	got, _ := s.RecentSummaries(ctx, 3)
	if strings.Join(got, ",") != "вторая,третья,четвёртая" {
		t.Fatalf("got %v", got)
	}
}

func TestFactsDeduplicate(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	_ = s.AddFacts(ctx, []string{"Пользователя зовут Алекс", " Пользователя зовут Алекс ", ""})
	_ = s.AddFacts(ctx, []string{"Есть кошка"})
	got, _ := s.Facts(ctx)
	if len(got) != 2 {
		t.Fatalf("facts=%v", got)
	}
}

func TestSearch(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	id, _ := s.StartSession(ctx)
	_ = s.AddMessage(ctx, id, Message{Role: "user", Content: "Какие диски подойдут на Тойота Камри?"})
	_ = s.AddMessage(ctx, id, Message{Role: "tool", Content: "секретный вывод инструмента про Камри"})
	_ = s.EndSession(ctx, id, "Подбирали диски для Камри")
	res, err := s.Search(ctx, "камри диски", 5)
	if err != nil || len(res) == 0 {
		t.Fatalf("res=%v err=%v", res, err)
	}
	for _, r := range res {
		if strings.Contains(r, "секретный") {
			t.Fatalf("tool output must not be searched: %v", res)
		}
	}
	if res, _ := s.Search(ctx, "100%", 5); len(res) != 0 {
		t.Fatalf("wildcard leaked: %v", res)
	}
}
