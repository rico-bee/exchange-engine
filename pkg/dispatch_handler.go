package pkg

import (
	"context"
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/Shopify/sarama"
	"gitlab.com/sdce/exchange/engine/pkg/engine"
	"gitlab.com/sdce/exchange/engine/pkg/recorder"
	"gitlab.com/sdce/exlib/kafka/topic"
	pb "gitlab.com/sdce/protogo"
)

// EngineServer contains multiple match engines
type EngineServer struct {
	engines map[string]*engine.Engine
}

// NewEngineServer initialise engine server
// codes are supported code
func NewEngineServer(ctx context.Context, codes []string, rec recorder.EventRecorder, rstr engine.Restorer) *EngineServer {
	engines := make(map[string]*engine.Engine)
	// restore previous state from trading service at present
	for _, code := range codes {
		var eng = engine.NewEngine(code)
		err := rstr.RestoreOrderBook(code, eng)
		if err != nil {
			log.Fatalf("failed to restore engine for code %v, info: %v.\n", code, err.Error())
		}
		go eventRecord(ctx, eng.EventsOutput(), rec)

		eng.Start(ctx)
		engines[code] = eng

	}
	return &EngineServer{
		engines: engines,
	}
}

// GetTopics returns input topics used by defined match engines
func (s *EngineServer) GetTopics() []string {
	var result []string
	for k := range s.engines {
		result = append(result, topic.EngineInPrefix+k)
	}
	log.Printf("Listening to topics %v.\n", result)
	return result
}

// Dispatch reads and dispatch consumer messages to different methods in order engine
func (s *EngineServer) Dispatch(ctx context.Context, msg *sarama.ConsumerMessage) (err error) {
	switch {
	case strings.HasPrefix(msg.Topic, topic.EngineInPrefix):
		log.Infof("Dispatching engine event %s/%d/%d\t%s\n", msg.Topic, msg.Partition, msg.Offset, msg.Key)
		eventIn := &engine.InEvent{
			EventCoord:    &engine.EventCoord{msg.Partition, msg.Offset},
			EngineEventIn: &pb.EngineEventIn{},
		}
		err = eventIn.Unmarshal(msg.Value)
		if err != nil {
			err = fmt.Errorf("Cannot decode engine event message: %v", err.Error())
			log.Errorln(err.Error())
		}

		// instrument code
		code := eventIn.Code
		instance := s.engines[code]
		if instance == nil {
			err = fmt.Errorf("No match engine found for topic %v", msg.Topic)
			log.Errorln(err.Error())
			// TODO shoot an error message to kafka
			return
		}

		done := make(chan struct{}, 1)

		instance.EventsInput() <- eventIn
		done <- struct{}{}

		select {
		case <-ctx.Done():
		case <-done:
		}

	default:
		err = fmt.Errorf("No appropriate handler found for topic: %s/%d/%d\t%s\t%s", msg.Topic, msg.Partition, msg.Offset, msg.Key, msg.Value)
		log.Errorln(err.Error())
	}
	return
}

func eventRecord(ctx context.Context, out <-chan *engine.OutEvent, rec recorder.EventRecorder) {
	for {
		select {
		case event, ok := <-out:
			if ok {
				err := rec.Record(event)
				if err != nil {
					log.Errorln("recorder error: ", err.Error())
				}
			}
		case <-ctx.Done():
			return
		}
	}
}
