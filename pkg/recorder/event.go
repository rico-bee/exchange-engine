package recorder

import (
	"context"
	"errors"

	"gitlab.com/sdce/exchange/engine/pkg/engine"
	"gitlab.com/sdce/exlib/kafka/topic"

	"github.com/Shopify/sarama"

	log "github.com/sirupsen/logrus"
)

//EventRecorder relay engine output events to broker
type EventRecorder interface {
	Record(*engine.OutEvent) error
}

type eventRec struct {
	ctx      context.Context
	producer sarama.AsyncProducer
}

// Record use channel to send message.
// This func can be used in concurrency
func (rec *eventRec) Record(event *engine.OutEvent) error {
	if event == nil {
		return errors.New("event parameter is nil")
	}
	encEvent, err := event.Marshal()
	if err != nil {
		return err
	}
	log.Printf("Recording event %v", event)
	rec.producer.Input() <- &sarama.ProducerMessage{
		Topic: topic.EngineOutPrefix + event.Code,
		Value: sarama.ByteEncoder(encEvent),
	}
	return nil
}

//NewEventRecorder create a recorder for writing event to broker
func NewEventRecorder(ctx context.Context, producer sarama.AsyncProducer) EventRecorder {
	if producer == nil {
		log.Errorln("empty event producer")
		return nil
	}
	go func() {
		for {
			select {

			case msg := <-producer.Successes():
				log.Infof("produced event on %v", msg.Topic)
			case err := <-producer.Errors():
				log.Errorln("event producer error: ", err.Error())
			case <-ctx.Done():
				return
			}
		}
	}()
	return &eventRec{
		ctx:      ctx,
		producer: producer,
	}
}
