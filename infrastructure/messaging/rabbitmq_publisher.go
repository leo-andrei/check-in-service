package messaging

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/leo-andrei/check-in-service/domain/events"

	amqp "github.com/rabbitmq/amqp091-go"
)

type RabbitMQPublisher struct {
	conn         *amqp.Connection
	channel      *amqp.Channel
	exchangeName string
}

func NewRabbitMQPublisher(rabbitURL, exchangeName string) (*RabbitMQPublisher, error) {
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	// Declare exchange
	err = ch.ExchangeDeclare(
		exchangeName, // name
		"fanout",     // type
		true,         // durable
		false,        // auto-deleted
		false,        // internal
		false,        // no-wait
		nil,          // arguments
	)
	if err != nil {
		return nil, fmt.Errorf("failed to declare exchange: %w", err)
	}

	return &RabbitMQPublisher{
		conn:         conn,
		channel:      ch,
		exchangeName: exchangeName,
	}, nil
}

func (p *RabbitMQPublisher) Publish(ctx context.Context, event events.DomainEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	return p.PublishRaw(ctx, event.EventType(), body)
}

func (p *RabbitMQPublisher) PublishRaw(ctx context.Context, eventType string, body []byte) error {
	err := p.channel.PublishWithContext(
		ctx,
		p.exchangeName, // exchange
		"",             // routing key (ignored for fanout)
		false,          // mandatory
		false,          // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent, // Make message persistent
			Type:         eventType,
		},
	)

	if err != nil {
		return fmt.Errorf("failed to publish event: %w", err)
	}

	return nil
}

func (p *RabbitMQPublisher) Close() error {
	if err := p.channel.Close(); err != nil {
		return err
	}
	return p.conn.Close()
}
