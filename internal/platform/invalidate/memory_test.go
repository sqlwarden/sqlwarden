package invalidate

import (
	"bytes"
	"context"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestMemoryDeliversInSubscriptionOrder(t *testing.T) {
	m := NewMemory(nil)
	var got []string
	m.Subscribe(TopicAuthz, func(k string) { got = append(got, "a:"+k) })
	m.Subscribe(TopicAuthz, func(k string) { got = append(got, "b:"+k) })

	if err := m.Publish(context.Background(), TopicAuthz, "k1"); err != nil {
		t.Fatal(err)
	}
	if err := m.Publish(context.Background(), TopicAuthz, "k2"); err != nil {
		t.Fatal(err)
	}

	want := []string{"a:k1", "b:k1", "a:k2", "b:k2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMemoryUnsubscribeStopsDeliveryAndIsIdempotent(t *testing.T) {
	m := NewMemory(nil)
	var a, b int
	unsubA := m.Subscribe(TopicSchema, func(string) { a++ })
	m.Subscribe(TopicSchema, func(string) { b++ })

	_ = m.Publish(context.Background(), TopicSchema, "k")
	unsubA()
	unsubA()
	_ = m.Publish(context.Background(), TopicSchema, "k")

	if a != 1 || b != 2 {
		t.Fatalf("a=%d b=%d, want 1 and 2", a, b)
	}
}

func TestMemoryTopicIsolation(t *testing.T) {
	m := NewMemory(nil)
	var authz, schema int
	m.Subscribe(TopicAuthz, func(string) { authz++ })
	m.Subscribe(TopicSchema, func(string) { schema++ })

	_ = m.Publish(context.Background(), TopicAuthz, "k")

	if authz != 1 || schema != 0 {
		t.Fatalf("authz=%d schema=%d, want 1 and 0", authz, schema)
	}
}

func TestMemoryPanickingSubscriberIsRecoveredAndLogged(t *testing.T) {
	var buf bytes.Buffer
	m := NewMemory(slog.New(slog.NewTextHandler(&buf, nil)))
	var after int
	m.Subscribe(TopicCompletion, func(string) { panic("boom") })
	m.Subscribe(TopicCompletion, func(string) { after++ })

	if err := m.Publish(context.Background(), TopicCompletion, "conn-1"); err != nil {
		t.Fatalf("publish returned %v", err)
	}

	if after != 1 {
		t.Fatalf("later subscriber ran %d times, want 1", after)
	}
	out := buf.String()
	if !strings.Contains(out, "completion") || !strings.Contains(out, "conn-1") {
		t.Fatalf("log missing topic or key: %s", out)
	}
}

func TestMemoryPublishWithCanceledContextStillDelivers(t *testing.T) {
	m := NewMemory(nil)
	var n int
	m.Subscribe(TopicAuthz, func(string) { n++ })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := m.Publish(ctx, TopicAuthz, "k"); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("delivered %d times, want 1", n)
	}
}

func TestMemoryUnsubscribeInsideCallback(t *testing.T) {
	m := NewMemory(nil)
	var n int
	var unsub func()
	unsub = m.Subscribe(TopicPolicy, func(string) {
		n++
		unsub()
	})

	_ = m.Publish(context.Background(), TopicPolicy, "k")
	_ = m.Publish(context.Background(), TopicPolicy, "k")

	if n != 1 {
		t.Fatalf("callback ran %d times, want 1", n)
	}
}

func TestMemoryConcurrentPublishAndSubscribe(t *testing.T) {
	m := NewMemory(nil)
	var delivered sync.WaitGroup
	var mu sync.Mutex
	var count int
	stable := m.Subscribe(TopicAuthz, func(string) {
		mu.Lock()
		count++
		mu.Unlock()
	})
	defer stable()

	const workers, iterations = 8, 200
	for i := 0; i < workers; i++ {
		delivered.Add(2)
		go func() {
			defer delivered.Done()
			for j := 0; j < iterations; j++ {
				_ = m.Publish(context.Background(), TopicAuthz, "k")
			}
		}()
		go func() {
			defer delivered.Done()
			for j := 0; j < iterations; j++ {
				unsub := m.Subscribe(TopicAuthz, func(string) {})
				unsub()
			}
		}()
	}
	delivered.Wait()

	mu.Lock()
	defer mu.Unlock()
	if count != workers*iterations {
		t.Fatalf("stable subscriber saw %d, want %d", count, workers*iterations)
	}
}
