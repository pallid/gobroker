package pubsub

import (
	"log"
	"time"

	"github.com/febytanzil/gobroker"
	"github.com/nsqio/go-nsq"
)

type nsqWorker struct {
	channel     *nsq.Consumer
	concurrent  int
	lookupd     string
	retry       int
	contentType string
}

func newNSQWorker(c *config, s *SubHandler) *nsqWorker {
	cfg := nsq.NewConfig()
	cfg.Set("max_attempts", s.MaxRequeue)
	cfg.Set("msg_timeout", s.Timeout)
	cfg.Set("max_in_flight", s.MaxInFlight)
	con, err := nsq.NewConsumer(s.Topic, s.Name, cfg)
	if nil != err {
		log.Fatal("failed to initialize nsq consumer:", err)
	}

	return &nsqWorker{
		channel:     con,
		concurrent:  s.Concurrent,
		lookupd:     c.serverURL,
		retry:       c.retry,
		contentType: c.contentType,
	}
}

func (n *nsqWorker) Consume(name, topic string, maxRequeue int, handler gobroker.Handler) {
	retries := 0
	if 0 >= n.concurrent {
		n.concurrent = 1
	}
	n.channel.AddConcurrentHandlers(n.nsqMiddleware(handler), n.concurrent)

	for {
		err := n.channel.ConnectToNSQLookupd(n.lookupd)
		if nil != err {
			log.Printf("worker failed to initialize retried [%d] %s \n", retries, err)
			if 0 == n.retry || n.retry > retries {
				retries++
				continue
			}
			retries++
		} else {
			// reset retry counter for next possible disconnect
			retries = 0
		}
	}
}

func (n *nsqWorker) Stop() error {
	n.channel.Stop()
	return nil
}

func (n *nsqWorker) nsqMiddleware(h gobroker.Handler) nsq.HandlerFunc {
	return func(m *nsq.Message) error {
		err := h(&gobroker.Message{
			Body:        m.Body,
			Attempts:    int(m.Attempts),
			ContentType: n.contentType,
		})
		if nil != err {
			m.RequeueWithoutBackoff(time.Minute)
			return err
		}

		m.Finish()
		return nil
	}
}

// ChangeMaxInFlight sets a new maximum number of messages this comsumer instance
// will allow in-flight, and updates all existing connections as appropriate.
//
// For example, ChangeMaxInFlight(0) would pause message flow
//
// If already connected, it updates the reader RDY state for each connection.
//
// link https://github.com/nsqio/go-nsq/blob/bf9a77050449936dcfabaab30776812f04886ce6/consumer.go#L310
func (n *nsqWorker) ChangeMaxInFlight(mif int) {
	n.channel.ChangeMaxInFlight(mif)
}
