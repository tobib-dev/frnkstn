package messaging

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	rmq "github.com/rabbitmq/rabbitmq-amqp-go-client/pkg/rabbitmqamqp"
)

func TestDurableQueueRetainsMessagesWithoutConsumer(t *testing.T) {
	uri := os.Getenv("RABBITMQ_TEST_URI")
	if uri == "" {
		t.Skip("set RABBITMQ_TEST_URI to run the durable queue integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	name := fmt.Sprintf("frnkstn-durable-test-%d", time.Now().UnixNano())
	conn, err := rmq.NewEnvironment(uri, nil).NewConnection(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = conn.Management().DeleteQueue(cleanupCtx, name)
		_ = conn.Management().DeleteExchange(cleanupCtx, name)
		_ = conn.Close(cleanupCtx)
	})
	// No consumer or queue exists yet: publishing must declare the durable route.
	accepted, err := PublishDurable(ctx, name, "purge.server", uri, name, "purge.#",
		[]byte("purge job"))
	if err != nil || !accepted {
		t.Fatalf("publish: accepted=%v err=%v", accepted, err)
	}
	consumer, closeConsumer, err := DeclareAndBindDurableConsumer(
		ctx, name, "purge.#", uri, name,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeMessagingResource(closeConsumer) })
	delivery, err := consumer.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if delivery == nil || delivery.Message() == nil {
		t.Fatal("persistent job was not retained")
	}
	message := delivery.Message()
	if string(message.GetData()) != "purge job" ||
		message.Header == nil || !message.Header.Durable {
		t.Fatal("job body or persistent message header was lost")
	}
	if err := delivery.Accept(ctx); err != nil {
		t.Fatal(err)
	}
}
