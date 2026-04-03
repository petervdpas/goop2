package chat

import "testing"

func TestRingBuffer_Empty(t *testing.T) {
	r := &RingBuffer{}
	if msgs := r.All(); msgs != nil {
		t.Fatalf("expected nil, got %v", msgs)
	}
	if msgs := r.Recent(5); msgs != nil {
		t.Fatalf("expected nil, got %v", msgs)
	}
}

func TestRingBuffer_AddAndAll(t *testing.T) {
	r := &RingBuffer{}
	r.Add(Message{ID: "1", Text: "hello"})
	r.Add(Message{ID: "2", Text: "world"})

	msgs := r.All()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].ID != "1" || msgs[1].ID != "2" {
		t.Fatalf("wrong order: %v", msgs)
	}
}

func TestRingBuffer_Recent(t *testing.T) {
	r := &RingBuffer{}
	for i := 0; i < 10; i++ {
		r.Add(Message{ID: string(rune('a' + i))})
	}

	msgs := r.Recent(3)
	if len(msgs) != 3 {
		t.Fatalf("expected 3, got %d", len(msgs))
	}
	if msgs[0].ID != string(rune('a'+7)) {
		t.Fatalf("expected 'h', got %q", msgs[0].ID)
	}
}

func TestRingBuffer_Overflow(t *testing.T) {
	r := &RingBuffer{}
	for i := 0; i < maxHistory+20; i++ {
		r.Add(Message{ID: string(rune(i))})
	}

	msgs := r.All()
	if len(msgs) != maxHistory {
		t.Fatalf("expected %d messages, got %d", maxHistory, len(msgs))
	}
	if msgs[0].ID != string(rune(20)) {
		t.Fatalf("oldest message should be index 20, got %q", msgs[0].ID)
	}
}
