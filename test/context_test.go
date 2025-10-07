package test

import (
	"context"
	"testing"
	"time"

	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/protocol/pull"
	"go.nanomsg.org/mangos/v3/protocol/push"
	"go.nanomsg.org/mangos/v3/protocol/req"
	"go.nanomsg.org/mangos/v3/protocol/rep"
	_ "go.nanomsg.org/mangos/v3/transport/inproc"
)

func TestRecvMsgContextTimeout(t *testing.T) {
	pullSock, err := pull.NewSocket()
	if err != nil {
		t.Fatalf("NewSocket: %v", err)
	}
	defer pullSock.Close()

	if err := pullSock.Listen("inproc://test_context_timeout"); err != nil {
		t.Fatalf("Listen: %v", err)
	}

	// Create a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Try to receive - should timeout
	start := time.Now()
	_, err = pullSock.RecvMsgContext(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Expected timeout error, got nil")
	}

	if elapsed < 40*time.Millisecond || elapsed > 200*time.Millisecond {
		t.Fatalf("Expected timeout around 50ms, got %v", elapsed)
	}

	// Verify we get the protocol timeout error for backwards compatibility
	if err != mangos.ErrRecvTimeout {
		t.Fatalf("Expected ErrRecvTimeout, got %v", err)
	}
}

func TestRecvMsgContextCancellation(t *testing.T) {
	pullSock, err := pull.NewSocket()
	if err != nil {
		t.Fatalf("NewSocket: %v", err)
	}
	defer pullSock.Close()

	if err := pullSock.Listen("inproc://test_context_cancel"); err != nil {
		t.Fatalf("Listen: %v", err)
	}

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// Try to receive - should be cancelled
	start := time.Now()
	_, err = pullSock.RecvMsgContext(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Expected cancellation error, got nil")
	}

	if elapsed < 40*time.Millisecond || elapsed > 200*time.Millisecond {
		t.Fatalf("Expected cancellation around 50ms, got %v", elapsed)
	}

	if err != context.Canceled {
		t.Fatalf("Expected context.Canceled, got %v", err)
	}
}

func TestContextWithMessage(t *testing.T) {
	pushSock, err := push.NewSocket()
	if err != nil {
		t.Fatalf("NewSocket push: %v", err)
	}
	defer pushSock.Close()

	pullSock, err := pull.NewSocket()
	if err != nil {
		t.Fatalf("NewSocket pull: %v", err)
	}
	defer pullSock.Close()

	if err := pullSock.Listen("inproc://test_context_msg"); err != nil {
		t.Fatalf("Listen: %v", err)
	}

	if err := pushSock.Dial("inproc://test_context_msg"); err != nil {
		t.Fatalf("Dial: %v", err)
	}

	time.Sleep(50 * time.Millisecond) // Let connection establish

	// Send a message
	ctx := context.Background()
	msg := []byte("hello context")
	if err := pushSock.SendContext(ctx, msg); err != nil {
		t.Fatalf("SendContext: %v", err)
	}

	// Receive with context
	received, err := pullSock.RecvContext(ctx)
	if err != nil {
		t.Fatalf("RecvContext: %v", err)
	}

	if string(received) != string(msg) {
		t.Fatalf("Expected %q, got %q", msg, received)
	}
}

func TestReqRepWithContext(t *testing.T) {
	reqSock, err := req.NewSocket()
	if err != nil {
		t.Fatalf("NewSocket req: %v", err)
	}
	defer reqSock.Close()

	repSock, err := rep.NewSocket()
	if err != nil {
		t.Fatalf("NewSocket rep: %v", err)
	}
	defer repSock.Close()

	if err := repSock.Listen("inproc://test_reqrep_context"); err != nil {
		t.Fatalf("Listen: %v", err)
	}

	if err := reqSock.Dial("inproc://test_reqrep_context"); err != nil {
		t.Fatalf("Dial: %v", err)
	}

	time.Sleep(50 * time.Millisecond) // Let connection establish

	// Send request with context
	ctx := context.Background()
	request := []byte("ping")
	if err := reqSock.SendContext(ctx, request); err != nil {
		t.Fatalf("SendContext: %v", err)
	}

	// Receive request
	received, err := repSock.RecvContext(ctx)
	if err != nil {
		t.Fatalf("RecvContext req: %v", err)
	}

	if string(received) != string(request) {
		t.Fatalf("Expected %q, got %q", request, received)
	}

	// Send reply
	reply := []byte("pong")
	if err := repSock.SendContext(ctx, reply); err != nil {
		t.Fatalf("SendContext reply: %v", err)
	}

	// Receive reply
	receivedReply, err := reqSock.RecvContext(ctx)
	if err != nil {
		t.Fatalf("RecvContext reply: %v", err)
	}

	if string(receivedReply) != string(reply) {
		t.Fatalf("Expected %q, got %q", reply, receivedReply)
	}
}
