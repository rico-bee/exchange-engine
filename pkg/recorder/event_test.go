package recorder

import (
	"context"
	"testing"

	"gitlab.com/sdce/exchange/engine/pkg/engine"

	"github.com/Shopify/sarama"

	"github.com/golang/mock/gomock"
	"gitlab.com/sdce/exlib/mock"
	pb "gitlab.com/sdce/protogo"
)

func TestRecorder_eventRec_Record(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	producer := mock.NewMockAsyncProducer(ctrl)

	ch := make(chan *sarama.ProducerMessage)
	go func() {
		<-ch
	}()
	producer.EXPECT().Input().Return(ch)
	ctx := context.Background()
	rec := NewEventRecorder(ctx, producer)

	args := []struct {
		name  string
		event *engine.OutEvent
		isErr bool
	}{
		{
			name: "normal",
			event: &engine.OutEvent{
				Code: "0000",
				EngineEventOut: &pb.EngineEventOut{
					InOffset: 0,
				},
			},
			isErr: false,
		},
		{
			name:  "null parameter",
			event: nil,
			isErr: true,
		},
	}

	for _, arg := range args {
		err := rec.Record(arg.event)

		if (err != nil) != arg.isErr {
			t.FailNow()
		}
	}
}
