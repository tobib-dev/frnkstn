package messaging

import (
	"context"
	"fmt"

	rmq "github.com/rabbitmq/rabbitmq-amqp-go-client/pkg/rabbitmqamqp"
)

func DeclareAndBindPublisher(
	ctx context.Context,
	exchangeName, routingKey, brokerURI string,
) (*rmq.Publisher, func(context.Context) error, error) {
	env := rmq.NewEnvironment(brokerURI, nil)
	conn, err := env.NewConnection(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("create AMQP connection: %w", err)
	}

	if _, err = conn.Management().DeclareExchange(
		ctx,
		&rmq.TopicExchangeSpecification{Name: exchangeName},
	); err != nil {
		_ = conn.Close(ctx)
		return nil, nil, fmt.Errorf("declare exchange: %w", err)
	}

	pub, err := conn.NewPublisher(ctx, &rmq.ExchangeAddress{
		Exchange: exchangeName,
		Key:      routingKey,
	}, nil)
	if err != nil {
		_ = conn.Close(ctx)
		return nil, nil, fmt.Errorf("create publisher: %w", err)
	}

	closerFunc := func(closeCtx context.Context) error {
		// close resources before owning connection
		pubErr := pub.Close(closeCtx)
		connErr := conn.Close(closeCtx)

		if pubErr != nil {
			return fmt.Errorf("close publisher: %w", pubErr)
		}
		if connErr != nil {
			return fmt.Errorf("close connection: %w", connErr)
		}
		return nil
	}

	return pub, closerFunc, nil
}
