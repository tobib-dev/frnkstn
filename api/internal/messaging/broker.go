package messaging

import (
	"context"
	"fmt"
	"time"

	rmq "github.com/rabbitmq/rabbitmq-amqp-go-client/pkg/rabbitmqamqp"
)

func Publish(
	ctx context.Context,
	exchangeName, routingKey,
	brokerURI string, body []byte,
) (bool, error) {
	publisher, closePublisher, err := declarePublisher(
		ctx, exchangeName, routingKey, brokerURI, "", "",
	)
	if err != nil {
		return false, err
	}
	return publishMessage(
		ctx, publisher, closePublisher, body)
}

// PublishDurable declares the named queue and binding before sending persistent messages.
func PublishDurable(
	ctx context.Context,
	exchange, routingKey,
	uri, queueName,
	binding string, body []byte,
) (bool, error) {
	if queueName == "" || binding == "" {
		return false, fmt.Errorf(
			"durable queue name and binding are required")
	}
	publisher, closePublisher, err := declarePublisher(
		ctx, exchange, routingKey, uri, queueName, binding,
	)
	if err != nil {
		return false, err
	}
	return publishMessage(
		ctx, publisher, closePublisher, body)
}

func publishMessage(
	ctx context.Context,
	publisher *rmq.Publisher,
	closePublisher func(
		context.Context,
	) error,
	body []byte,
) (bool, error) {
	defer closeMessagingResource(closePublisher)

	// The installed AMQP client sets Header.Durable=true when no header is supplied.
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
