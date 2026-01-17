package messaging

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/leo-andrei/check-in-service/infrastructure/config"

	amqp "github.com/rabbitmq/amqp091-go"
)

type MessageHandler func(ctx context.Context, body []byte) error

type RabbitMQConsumer struct {
	conn      *amqp.Connection
	channel   *amqp.Channel
	queueName string
}

func NewRabbitMQConsumer(rabbitURL, exchangeName, queueName string) (*RabbitMQConsumer, error) {
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	// Declare dead letter exchange for DLQ
	dlqExchangeName := queueName + "-dlx"
	err = ch.ExchangeDeclare(
		dlqExchangeName,
		"direct", // type
		true,     // durable
		false,    // auto-delete
		false,    // internal
		false,    // no-wait
		nil,      // args
	)
	if err != nil {
		return nil, fmt.Errorf("failed to declare DLX: %w", err)
	}

	// Declare DLQ
	dlqName := queueName + "-dlq"
	_, err = ch.QueueDeclare(
		dlqName,
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // no additional args for DLQ
	)
	if err != nil {
		return nil, fmt.Errorf("failed to declare DLQ: %w", err)
	}

	// Bind DLQ to DLX
	err = ch.QueueBind(
		dlqName,
		dlqName, // routing key
		dlqExchangeName,
		false,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to bind DLQ: %w", err)
	}

	dlqTTL := config.Cfg.RabbitMQ.DLQTTL
	prefetchCount := config.Cfg.RabbitMQ.PrefetchCount

	// Declare main queue with DLX and TTL
	args := amqp.Table{
		"x-dead-letter-exchange":    dlqExchangeName,
		"x-dead-letter-routing-key": dlqName,
		"x-message-ttl":             int64(dlqTTL),
	}

	_, err = ch.QueueDeclare(
		queueName,
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		args,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to declare queue: %w", err)
	}

	// Bind queue to exchange
	err = ch.QueueBind(
		queueName,
		"",           // routing key
		exchangeName, // exchange
		false,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to bind queue: %w", err)
	}

	// Set prefetch count (QoS)
	err = ch.Qos(
		prefetchCount, // prefetch count
		0,             // prefetch size
		false,         // global
	)
	if err != nil {
		return nil, fmt.Errorf("failed to set QoS: %w", err)
	}

	return &RabbitMQConsumer{
		conn:      conn,
		channel:   ch,
		queueName: queueName,
	}, nil
}

func (c *RabbitMQConsumer) Consume(ctx context.Context, handler MessageHandler) error {
	msgs, err := c.channel.Consume(
		c.queueName,
		"",    // consumer tag
		false, // auto-ack (we'll manually ack)
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		return fmt.Errorf("failed to register consumer: %w", err)
	}

	config.Logger.Info("Consumer started", zap.String("queue", c.queueName))

	for {
		select {
		case <-ctx.Done():
			config.Logger.Info("Consumer shutting down", zap.String("queue", c.queueName))
			return ctx.Err()

		case msg, ok := <-msgs:
			if !ok {
				return fmt.Errorf("channel closed")
			}

			// Process message
			err := handler(ctx, msg.Body)
			if err != nil {
				config.Logger.Error("Error processing message", zap.Error(err), zap.String("queue", c.queueName))
				// Reject and requeue - message will stay in queue until TTL expires, then move to DLQ
				msg.Nack(false, true)
			} else {
				// Acknowledge successful processing
				msg.Ack(false)
			}
		}
	}
}

func (c *RabbitMQConsumer) Close() error {
	if err := c.channel.Close(); err != nil {
		return err
	}
	return c.conn.Close()
}
