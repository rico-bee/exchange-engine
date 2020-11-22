package engine

import (
	"container/heap"
	"context"
	"fmt"
	"strconv"

	cluster "github.com/bsm/sarama-cluster"
	log "github.com/sirupsen/logrus"
	"gitlab.com/sdce/exchange/engine/pkg/api"
	"gitlab.com/sdce/exlib/kafka/topic"
	pb "gitlab.com/sdce/protogo"
)

// Coords records topic by every partition(int32 to string) and the corresponding offset(int64)
type Coords map[string]int64

// Restorer interface load book data into engine and set msg offset
type Restorer interface {
	// Do fill engine with restored data
	RestoreOrderBook(code string, engine *Engine) error
	ResetOffset(*cluster.Consumer) error
}

type restorer struct {
	Apis          *api.Server
	EventInCoords map[string]Coords // topic:Coords
}

func (rstr *restorer) RestoreOrderBook(code string, engine *Engine) error {
	// get instrument by code
	ctx := context.Background()
	// restore orderbook from api
	ob, err := rstr.getSlaveOrderBook(ctx, code)
	if err != nil {
		return err
	}
	err = rstr.restore(ob, engine.Orderbook)
	if err != nil {
		return err
	}
	rstr.EventInCoords[topic.EngineInPrefix+code] = ob.InOffset
	log.Infof("Restored orderbook for %v:\n%v", code, formatOrderbookToString(ob))
	return nil
}

func (rstr *restorer) ResetOffset(consumer *cluster.Consumer) error {
	for topic, coords := range rstr.EventInCoords {
		for k, v := range coords {
			partition, err := strconv.Atoi(k)
			if err != nil {
				return fmt.Errorf("partition decode error: %v", err.Error())
			}
			consumer.ResetPartitionOffset(topic, int32(partition), v, "no use")
		}
	}
	return nil
}

func (rstr *restorer) GetCoords(topic string) (Coords, error) {
	coords, ok := rstr.EventInCoords[topic]
	if !ok {
		return nil, fmt.Errorf("GetCoords: topic %v not found", topic)
	}
	return coords, nil
}

func (rstr *restorer) getSlaveOrderBook(ctx context.Context, code string) (*pb.OrderBook, error) {
	ob, err := rstr.Apis.Trading.DoGetSlaveOrderBook(ctx, &pb.GetSlaveOrderBookRequest{
		Code: code,
	})
	if err != nil {
		return nil, err
	}
	return ob, nil
}

func (rstr *restorer) restore(from *pb.OrderBook, to *OrderBook) error {
	for _, bid := range from.Bids {
		order := OrderFromProto(bid)
		if order == nil {
			return fmt.Errorf("cannot covert bid order proto %v", *bid)
		}
		to.Bids = append(to.Bids, order)
	}
	for _, ask := range from.Asks {
		order := OrderFromProto(ask)
		if order == nil {
			return fmt.Errorf("cannot covert ask order proto %v", *ask)
		}
		to.Asks = append(to.Asks, order)
	}
	heap.Init(&to.Bids)
	heap.Init(&to.Asks)
	return nil
}

// NewEngineRestorer create a instqnce of loader
func NewEngineRestorer(apis *api.Server) Restorer {
	return &restorer{Apis: apis, EventInCoords: make(map[string]Coords)}
}
