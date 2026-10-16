package messaging

import (
	"context"
	"fmt"
	"time"

	rmq "github.com/rabbitmq/rabbitmq-amqp-go-client/pkg/rabbitmqamqp"
)

func Publish(ctx context.Context, exchangeName, routingKey, brokerURI string, body []byte) (bool, error) {
	publisher, closePublisher, err := DeclareAndBindPublisher(ctx, exchangeName, routingKey, brokerURI)
	if err != nil {
		return false, err
	}
	defer closeMessagingResource(closePublisher)

	result, err := publisher.Publish(ctx, rmq.NewMessage(body))
	if err != nil {
		return false, fmt.Errorf("publish notification: %w", err)
	}
	if result == nil {
		return false, fmt.Errorf("publish notification returned no result")
	}
	_, routed := result.Outcome.(*rmq.StateAccepted)
	return routed, nil
}

func closeMessagingResource(closeResource func(context.Context) error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = closeResource(ctx)
}
