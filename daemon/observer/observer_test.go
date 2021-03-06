package observer

import (
	"bytes"
	"context"
	"net/http/httptest"
	"testing"

	"github.com/satori/go.uuid"
)

func TestNotifier(t *testing.T) {
	t.Run("a chained notifier keeps track of total", func(t *testing.T) {
		o := New()
		id := uuid.NewV4().String()
		ctx := context.WithValue(context.Background(), CtxRequestID, id)

		parent, err := o.Notifier(ctx, 5)
		if err != nil {
			t.Errorf("unexpected error constructing notifier: %s", err)
		}

		child := parent.Notifier(6)
		grandChildA := child.Notifier(7)
		grandChildB := child.Notifier(6)

		go grandChildB.Notify(Progress, "hello", true)
		go grandChildA.Notify(Progress, "woo", true)
		go grandChildA.Notify(Progress, "hahahaha", false)

		<-o.notify
		<-o.notify
		evtC := <-o.notify

		if evtC.Total != 24 {
			t.Errorf("evtC Total does not match: %d", evtC.Total)
		}
		if evtC.Completed != 2 {
			t.Errorf("evtC Completed does not match: %d - %d", 2, evtC.Completed)
		}
	})

	// Test will timeout if this behaviour is not correct
	t.Run("a chained Notify resolves when ctx is cancelled", func(t *testing.T) {
		o := New()
		id := uuid.NewV4().String()

		parentCtx, cancel := context.WithCancel(context.Background())
		ctx := context.WithValue(parentCtx, CtxRequestID, id)

		parent, err := o.Notifier(ctx, 1)
		if err != nil {
			t.Errorf("unexpected error constructing notifier: %s", err)
		}

		child := parent.Notifier(3)

		cancel()
		child.Notify(Progress, "hahahaha", true)
	})

	// Test will timeout if this behaviour is not correct
	t.Run("a chained Notify when observer is Closed", func(t *testing.T) {
		o := New()
		id := uuid.NewV4().String()
		ctx := context.WithValue(context.Background(), CtxRequestID, id)

		parent, err := o.Notifier(ctx, 1)
		if err != nil {
			t.Errorf("unexpected error constructing notifier: %s", err)
		}

		child := parent.Notifier(1)

		o.Stop()
		child.Notify(Progress, "hello please dont timeout", true)
	})

	t.Run("panic's if total is exceeded", func(t *testing.T) {
		o := New()
		id := uuid.NewV4().String()
		ctx := context.WithValue(context.Background(), CtxRequestID, id)

		defer func() {
			if r := recover(); r == nil {
				t.Errorf("Expected a panic; did not receive one")
			}
		}()

		parent, err := o.Notifier(ctx, 1)
		if err != nil {
			t.Errorf("unexpected error constructing notifier: %s", err)
		}

		go parent.Notify(Progress, "helo", true)
		<-o.notify

		parent.Notify(Progress, "haha", true)
	})
}

type CloseNotifyResponseRecorder struct {
	httptest.ResponseRecorder
	flushed chan bool
}

func (cn *CloseNotifyResponseRecorder) CloseNotify() <-chan bool {
	return make(chan bool)
}

func (cn *CloseNotifyResponseRecorder) Flush() {
	cn.flushed <- true
}

func TestObserverServeHTTP(t *testing.T) {
	t.Run("events are sent via SSE", func(t *testing.T) {
		o := New()
		id := uuid.NewV4().String()
		ctx := context.WithValue(context.Background(), CtxRequestID, id)

		go o.Start()
		defer o.Stop()

		rw := &CloseNotifyResponseRecorder{
			ResponseRecorder: *httptest.NewRecorder(),
			flushed:          make(chan bool),
		}

		r := httptest.NewRequest("GET", "/observe", nil)

		n, err := o.Notifier(ctx, 1)
		if err != nil {
			t.Errorf("unexpected error constructing Notifier: %s", err)
		}

		go o.ServeHTTP(rw, r)
		// A single flush. When we serve HTTP requests through the observer, it
		// does an initial flush after setting the headers. After that, it
		// flushes once per Notification. This flush is to make sure the initial
		// flush is accounted for.
		<-rw.flushed

		go n.Notify(Progress, "hi", true)
		<-rw.flushed

		expected := "text/event-stream"
		seen := rw.Header().Get("Content-Type")
		if seen != expected {
			t.Errorf("Unexpected content type. got: %s want: %s", seen, expected)
		}

		expectedEvent := []byte(
			"event: progress\ndata: {\"id\":\"" + id + "\",\"message\":\"hi\",\"completed\":1,\"total\":1}\n\n",
		)
		if !bytes.Equal(rw.Body.Bytes(), expectedEvent) {
			t.Errorf("Event data does not match. got:\n%s\nwanted:\n%s", rw.Body.Bytes(), expectedEvent)
		}
	})
}

func TestNotifier_Notify(t *testing.T) {
	t.Run("it should not lock when there is no http listener", func(t *testing.T) {
		o := New()
		id := uuid.NewV4().String()
		ctx := context.WithValue(context.Background(), CtxRequestID, id)

		go o.Start()
		defer o.Stop()

		n, err := o.Notifier(ctx, 1)
		if err != nil {
			t.Errorf("unexpected error constructing Notifier: %s", err)
		}

		go n.Notify(Progress, "hi", true)
	})
}

// This benchmark is set up so we can track deadlocks for waiting on server
// boot.
func BenchmarkObserver_ServeHTTP_Notify(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		o := New()
		id := uuid.NewV4().String()
		ctx := context.WithValue(context.Background(), CtxRequestID, id)

		go o.Start()
		defer o.Stop()

		rw := &CloseNotifyResponseRecorder{
			ResponseRecorder: *httptest.NewRecorder(),
			flushed:          make(chan bool),
		}

		r := httptest.NewRequest("GET", "/observe", nil)

		n, err := o.Notifier(ctx, uint(b.N))
		if err != nil {
			b.Errorf("unexpected error constructing Notifier: %s", err)
		}

		go o.ServeHTTP(rw, r)
		<-rw.flushed

		go n.Notify(Progress, "hi", true)
		<-rw.flushed
	}
}

// This benchmark ensures the concurrent behaviour of the notifier. We should be
// able to send multiple notifications at once.
func BenchmarkObserver_Notify(b *testing.B) {
	o := New()
	id := uuid.NewV4().String()
	ctx := context.WithValue(context.Background(), CtxRequestID, id)

	go o.Start()
	defer o.Stop()

	rw := &CloseNotifyResponseRecorder{
		ResponseRecorder: *httptest.NewRecorder(),
		flushed:          make(chan bool),
	}

	r := httptest.NewRequest("GET", "/observe", nil)

	n, err := o.Notifier(ctx, uint(b.N))
	if err != nil {
		b.Errorf("unexpected error constructing Notifier: %s", err)
	}

	go o.ServeHTTP(rw, r)
	<-rw.flushed

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			go n.Notify(Progress, "hi", true)
			<-rw.flushed
		}
	})
}
