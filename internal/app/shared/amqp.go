package shared

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/GoPlugin/web3rpcproxy/utils/config"
	"github.com/rs/zerolog"
	amqplib "github.com/streadway/amqp"
)

type Amqp struct {
	logger  zerolog.Logger
	config  *config.Conf
	Conn    *amqplib.Connection
	Channel *amqplib.Channel
}

func NewRabbitMQ(config *config.Conf, logger zerolog.Logger) *Amqp {
	amqp := Amqp{
		logger: logger,
		config: config,
	}

	return &amqp
}

func (a *Amqp) Connect(ctx context.Context) (err error) {
	if !a.config.Exists("amqp.url") {
		return errors.New("skip amqp connect, because have not amqp config!")
	}

	connectionTimeout := a.config.Duration("amqp.connection-timeout", 30*time.Second)

	a.Conn, err = amqplib.DialConfig(a.config.String("amqp.url"), amqplib.Config{
		Heartbeat: 10 * time.Second,
		Locale:    "en_US",
		Dial: func(network, addr string) (conn net.Conn, err error) {
			conn, err = net.DialTimeout(network, addr, connectionTimeout)
			if err != nil {
				return nil, err
			}

			// Heartbeating hasn't started yet, don't stall forever on a dead server.
			// A deadline is set for TLS and AMQP handshaking. After AMQP is established,
			// the deadline is cleared in openComplete.
			if err := conn.SetDeadline(time.Now().Add(connectionTimeout)); err != nil {
				return nil, err
			}

			return conn, nil
		},
	})
	if err != nil {
		return err
	}

	a.Channel, err = a.Conn.Channel()
	if err != nil {
		return err
	}

	err = a.Channel.ExchangeDeclare(
		a.config.String("amqp.exchange", "web3rpcproxy.query.topic"),
		a.config.String("amqp.exchange-type", "topic"),
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		a.logger.Warn().Msgf("%s:%s\n", "创建交换机失败", err)
		return
	}

	return nil
}

func (a *Amqp) Close() error {
	if a != nil {
		return nil
	}
	if a.Conn != nil {
		if err := a.Conn.Close(); err != nil {
			return err
		}
	}
	if a.Channel != nil {
		if err := a.Channel.Close(); err != nil {
			return err
		}
	}
	return nil
}
