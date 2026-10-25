package publisher

import (
	"context"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// QueueName is the name of the durable RabbitMQ queue to which job events are published.
const QueueName = "jobs"

// RabbitMQPublisher is a Publisher backed by a single RabbitMQ connection and channel.
// It uses publisher confirms to guarantee the broker acknowledged each message before returning.
type RabbitMQPublisher struct {
	conn    *amqp.Connection
	channel *amqp.Channel
}

// NewRabbitMQPublisher dials RabbitMQ, opens a channel, declares the durable jobs queue,
// and enables publisher confirms. It returns an error if any step fails.
func NewRabbitMQPublisher(url string) (*RabbitMQPublisher, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("dial rabbitmq: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("open channel: %w", err)
	}

	_, err = ch.QueueDeclare(QueueName, true, false, false, false, nil)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("declare queue %q: %w", QueueName, err)
	}

	if err := ch.Confirm(false); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("set confirm mode: %w", err)
	}

	return &RabbitMQPublisher{conn: conn, channel: ch}, nil
}

// Publish sends body to the jobs queue and waits for broker acknowledgement.
// Not safe for concurrent use — the outbox relay calls it from a single goroutine.
func (p *RabbitMQPublisher) Publish(ctx context.Context, body []byte) error {
	confirm, err := p.channel.PublishWithDeferredConfirmWithContext(ctx, "", QueueName, false, false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		},
	)
	if err != nil {
		return fmt.Errorf("publish to %q: %w", QueueName, err)
	}
	if ok := confirm.Wait(); !ok {
		return fmt.Errorf("message not acknowledged by broker")
	}
	return nil
}

// Close gracefully closes the channel and the underlying connection.
func (p *RabbitMQPublisher) Close() error {
	if err := p.channel.Close(); err != nil {
		return err
	}
	return p.conn.Close()
}
