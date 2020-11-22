package engine

import (
	"container/heap"
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"gitlab.com/sdce/exlib/exutil"

	pb "gitlab.com/sdce/protogo"
)

type Trade pb.TradeDefined
type comparator func(float64, float64) bool

// OrderBook is book keeps all orders on both sides
type OrderBook struct {
	Code string
	Bids OrderList
	Asks OrderList
}

// ob (order book) gets orderbook at specified side
func (o *OrderBook) ob(side pb.OrderSide) *OrderList {
	if side == pb.OrderSide_ASK {
		return &o.Asks
	} else if side == pb.OrderSide_BID {
		return &o.Bids
	}
	log.Fatalln("Invalid order side.")
	return nil
}

// cob (counter order book) gets orderbook opposite to the specified side
func (o *OrderBook) cob(side pb.OrderSide) *OrderList {
	if side == pb.OrderSide_ASK {
		return &o.Bids
	} else if side == pb.OrderSide_BID {
		return &o.Asks
	}
	log.Fatalln("Invalid order side.")
	return nil
}

//Engine matching engine which processes orders inside the orderbook
type Engine struct {
	Orderbook   *OrderBook
	inEvents    chan *InEvent
	outEvents   chan *OutEvent
	eventMapper map[string]bool
	eventQue    []string
}

// NewEngine initializes engine's state
func NewEngine(code string) *Engine {
	return &Engine{
		Orderbook: &OrderBook{
			Code: code,
			Bids: OrderList{},
			Asks: OrderList{}},
		inEvents:    make(chan *InEvent),
		outEvents:   make(chan *OutEvent),
		eventMapper: map[string]bool{},
		eventQue:    []string{},
	}
}

// Start creates a channel
// Should not make buffered channel, since we should wait until processed to continue
func (e *Engine) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case event, ok := <-e.inEvents:
				if ok {
					e.processEvent(event)
				}
			case <-ctx.Done():
				log.Infoln("received done, exiting")
				return
			}
		}
	}()
}

// Close closes all underlying channels
func (e *Engine) Close() {
	close(e.inEvents)
	close(e.outEvents)
}

// EventsInput exposes write only channel for incoming events
// Includes create, update and cancel orders
func (e *Engine) EventsInput() chan<- *InEvent {
	return e.inEvents
}

// EventsOutput exposes read only channel for completed events
func (e *Engine) EventsOutput() <-chan *OutEvent {
	return e.outEvents
}

// matchOrder tries to calculate the volume can be fullfilled
// There really shouldn't be any multiplication/division in this function.
func (e *Engine) matchOrder(newOrder, orderInBook *Order) (*Trade, error) {
	var ask, bid *Order = newOrder, orderInBook
	if newOrder.Side == pb.OrderSide_BID {
		ask, bid = bid, ask
	}

	if ask.Side != pb.OrderSide_ASK || bid.Side != pb.OrderSide_BID {
		return nil, fmt.Errorf("order side mismatch. Expected ask got %v, expected bid got %v", ask.Side, bid.Side)
	}

	var tradePrice = orderInBook.Price

	if orderInBook.Type == pb.OrderType_MARKET {
		tradePrice = newOrder.Price
	}

	if tradePrice.Sign() <= 0 {
		if ask.Type == pb.OrderType_MARKET && bid.Type == pb.OrderType_MARKET {
			tradePrice = getLastPrice()
		} else {
			return nil, fmt.Errorf("non-positive tradePrice %v", tradePrice)
		}
	}

	err := ask.fixPrice(tradePrice) // NOTICE: this mutates the order
	if err != nil {
		return nil, err
	}
	err = bid.fixPrice(tradePrice) // NOTICE: this mutates the order too
	if err != nil {
		return nil, err
	}

	log.Infof("Fixed price: ask %v, bid %v", ask, bid)

	tradeVol := min(ask.RemainingVolume, bid.RemainingVolume)
	tradeVal := min(ask.RemainingValue, bid.RemainingValue)

	log.Trace(newOrder.RemainingVolume, orderInBook.RemainingVolume, newOrder.RemainingValue, orderInBook.RemainingValue)
	if exutil.QuoFloat(fl(tradeVal), fl(tradeVol)).Cmp(tradePrice) != 0 {
		log.Warnf("Trading price mismatch due to rounding. Precalculated %v, actual %v", tradePrice, exutil.QuoFloat(fl(tradeVal), fl(tradeVol)))
	}
	if tradeVol.Sign() <= 0 {
		// no trade is executed
		log.Errorf("No trade. Vol: %v min(%v, %v), Price: %v\n", tradeVol, newOrder.RemainingVolume, orderInBook.RemainingVolume, tradePrice)
		return nil, nil
	}

	err = newOrder.fill(tradeVol, tradeVal)
	if err != nil {
		return nil, fmt.Errorf("cannot fill order: %v", err)
	}

	err = orderInBook.fill(tradeVol, tradeVal)
	if err != nil {
		return nil, fmt.Errorf("cannot fill order: %v", err)
	}

	log.Infof("Trade made at price: %v, value: %v, volume: %v", tradePrice, tradeVal, tradeVol)
	log.Infof("Post trade %v, %v", newOrder, orderInBook)
	// gen flag
	flag := pb.TradeFlag_SELL_INITIATED
	if newOrder.Side == pb.OrderSide_BID {
		flag = pb.TradeFlag_BUY_INITIATED
	}

	price, _ := tradePrice.Float64()
	trade := &Trade{
		Id:         exutil.NewUUID(),
		Instrument: newOrder.Instrument, // TODO Get the instrument in a better way.
		Price:      price,
		Volume:     tradeVol.Text(10),
		Value:      tradeVal.Text(10),
		Flag:       flag,
		Bid:        orderRef(bid),
		Ask:        orderRef(ask),
		Time:       time.Now().UnixNano(),
	}
	return trade, nil
}

func orderRef(o *Order) *pb.OrderRef {
	return &pb.OrderRef{
		Id:     o.Id,
		Type:   o.Type,
		Source: o.Source,
		Side:   o.Side,
		Owner:  o.Owner,
		Time:   o.Time,
	}
}

// TODO handle when there is no price information available
// Eg. Both sides are market orders.
func getLastPrice() *big.Float {
	return new(big.Float).SetFloat64(0.0)
}

// orderMatchCompare returns whether a trade can be made between the two orders
func orderMatchCompare(l, r *Order) (matched bool, msg string) {
	// Market order at any side is tradable
	if l.Type == pb.OrderType_MARKET || r.Type == pb.OrderType_MARKET {
		return true, "a market match"
	}
	var bid, ask *Order
	if l.Side == pb.OrderSide_BID {
		bid, ask = l, r
	} else {
		bid, ask = r, l
	}
	matched = (bid.Price.Cmp(ask.Price) >= 0)
	if !matched {
		msg = fmt.Sprintf("not match because bid price %v, ask price %v", bid, ask)
	}
	msg = fmt.Sprintf("matched because bid price %v, ask price %v", bid, ask)
	return
}

func (e *Engine) insert(order *Order) {
	heap.Push(e.Orderbook.ob(order.Side), order)
}

// processOrder starts an infinite loop to process input of newOrders channel
func (e *Engine) processEvent(in *InEvent) {
	if in.Id != nil {
		newId := exutil.UUIDtoA(in.Id)
		_, ok := e.eventMapper[newId]
		if !ok {
			que := e.eventQue
			if len(e.eventQue) > 300 {
				que = e.eventQue[1:]
				delete(e.eventMapper, e.eventQue[0])
			}
			que = append(que, newId)
			e.eventQue = que
			e.eventMapper[newId] = true
		} else {
			log.Errorf("in-event %v come again, ignored", newId)
			return
		}
	}
	log.Info(" get order event: ", formatInEventToString(in))

	var out *OutEvent
	switch in.GetEvent().(type) {
	case *pb.EngineEventIn_NewOrder:
		out = e.processNewOrderEvent(in.EventCoord, in.GetNewOrder())
	case *pb.EngineEventIn_UpdateOrder:
		out = e.processUpdateOrderEvent(in.EventCoord, in.GetUpdateOrder())
	case *pb.EngineEventIn_CancelOrder:
		out = e.processCancelOrderEvent(in.EventCoord, in.GetCancelOrder())
	default:
		var msg string
		msg = fmt.Sprintf("Engine event format is unknown, please check module version")
		out = newFailEvent(e.Orderbook.Code, in.EventCoord, pb.EngineEventResult_ENG_ARG_INVALID, msg)
		log.Errorln(msg)
	}
	e.outEvents <- out
}

func (e *Engine) processNewOrderEvent(coord *EventCoord, order *pb.OrderDefined) (evt *OutEvent) {

	log.Infoln("Processing new order event: ")
	evt = newOrderEvent(e.Orderbook.Code, coord, order)

	orderNew := OrderFromProto(order)
	if err := orderNew.validate(); err != nil {
		log.Errorf("Order is not valid: %v", err)
		evt.AddResult(pb.EngineEventResult_NEW_ORDER_FAILED, err.Error())
		return
	}
	e.tryTrade(evt, orderNew)
	log.Info("Post process:\n", e)
	return
}

func (e *Engine) processUpdateOrderEvent(coord *EventCoord, req *pb.UpdateOrderRequest) (evt *OutEvent) {
	log.Infof("Processing update order event: order %v", exutil.UUIDtoA(req.Id))

	order := e.fetchOrder(req.Id)
	if order == nil {
		err := fmt.Errorf("order %v not exist when try to update", exutil.UUIDtoA(req.Id))
		evt = newFailEvent(e.Orderbook.Code, coord, pb.EngineEventResult_UPDATE_ORDER_FAILED, err.Error())
		return
	}

	evt = newUpdateOrderEvent(e.Orderbook.Code, coord, req, order)
	// make a backup, if update failed, then recover cache to orderlist
	cacheOrder := &Order{}
	cacheOrder = cacheOrder.Copy(order)
	// check if it can be updated
	err := e.checkAndUpdateOrder(order, req)
	if err != nil {
		evt.AddResult(pb.EngineEventResult_UPDATE_ORDER_FAILED, err.Error())
		// recover order
		e.insert(cacheOrder)
		return
	}
	// then process like new order event
	e.tryTrade(evt, order)
	return
}

func (e *Engine) processCancelOrderEvent(coord *EventCoord, req *pb.CancelOrderRequest) (evt *OutEvent) {
	log.Infoln("Processing cancel order event: ")

	order := e.fetchOrder(req.Id)
	if order == nil {
		err := fmt.Errorf("order %v not exist when try to cancel", exutil.UUIDtoA(req.Id))
		evt = newFailEvent(e.Orderbook.Code, coord, pb.EngineEventResult_CANCEL_ORDER_FAILED, err.Error())
		return
	}
	evt = newCancelOrderEvent(e.Orderbook.Code, coord, req, order)

	return
}

func newFailEvent(code string, coord *EventCoord, result pb.EngineEventResult, reason string) *OutEvent {
	out := &OutEvent{
		Code: code,
		EngineEventOut: &pb.EngineEventOut{
			InPartition: coord.Partition,
			InOffset:    coord.Offset,
			Result:      result,
			Reason:      reason,
		},
	}
	return out
}

func newOrderEvent(code string, coord *EventCoord, order *pb.OrderDefined) *OutEvent {
	orderEvent := &pb.OrderEvent{
		Id:   exutil.NewUUID(), // new event id
		Type: pb.OrderEventType_CREATE_ORDER,
		// update volume
		UpdateFromVolume: "0",
		UpdateToVolume:   order.Volume,
		// update value
		UpdateFromValue: "0",
		UpdateToValue:   order.Value,
		// no use
		TradeFilledValue:  "0",
		TradeFilledVolume: "0",
		Price:             order.Price,
		Time:              time.Now().UnixNano(),
	}
	return &OutEvent{
		Code: code,
		EngineEventOut: &pb.EngineEventOut{
			// default result code if it is successful
			InPartition: coord.Partition,
			InOffset:    coord.Offset,
			Event: &pb.EngineEventOut_NewOrder{
				NewOrder: &pb.NewOrderComplete{
					OrderId: order.GetId(),
					Event:   orderEvent,
					Order:   order,
					// Trades need to be filled outside
				},
			},
		},
	}
}

func newUpdateOrderEvent(code string, coord *EventCoord, req *pb.UpdateOrderRequest, order *Order) *OutEvent {
	orderEvent := &pb.OrderEvent{
		Id:   exutil.NewUUID(), // new event id
		Type: pb.OrderEventType_UPDATE_ORDER,
		// update volume
		UpdateFromVolume: order.Volume.String(),
		UpdateToVolume:   req.Volume,
		// update value
		UpdateFromValue: order.Value.String(),
		UpdateToValue:   req.Value,
		// price is not important
		Price: req.Price,
		Time:  time.Now().UnixNano(),
	}
	return &OutEvent{
		Code: code,
		EngineEventOut: &pb.EngineEventOut{
			// default result code if it is successful
			InPartition: coord.Partition,
			InOffset:    coord.Offset,
			Event: &pb.EngineEventOut_UpdateOrder{
				UpdateOrder: &pb.UpdateOrderComplete{
					OrderId: req.GetId(),
					Event:   orderEvent,
					// Trades need to be filled outside
				},
			},
		},
	}
}

func newCancelOrderEvent(code string, coord *EventCoord, req *pb.CancelOrderRequest, order *Order) *OutEvent {
	orderEvent := &pb.OrderEvent{
		Id:   exutil.NewUUID(), // new event id
		Type: pb.OrderEventType_CANCEL_ORDER,
		// cancelled volume
		UpdateFromVolume: order.Volume.String(),
		UpdateToVolume:   order.FilledVolume.String(),
		// cancelled value
		UpdateFromValue: order.Value.String(),
		UpdateToValue:   order.FilledValue.String(),
		// no use
		TradeFilledValue:  "0",
		TradeFilledVolume: "0",

		Time: time.Now().UnixNano(),
	}
	orderEvent.Price, _ = order.Price.Float64()
	return &OutEvent{
		Code: code,
		EngineEventOut: &pb.EngineEventOut{
			// default result code if it is successful
			InPartition: coord.Partition,
			InOffset:    coord.Offset,
			Reason:      req.Reason,
			Event: &pb.EngineEventOut_CancelOrder{
				CancelOrder: &pb.CancelOrderComplete{
					OrderId: req.GetId(),
					Event:   orderEvent,
					// Trades need to be filled outside
				},
			},
		},
	}
}

func (e *Engine) fetchOrder(id *pb.UUID) *Order {
	orderList := &e.Orderbook.Asks
	index, order := orderList.Find(id)
	if order == nil {
		orderList = &e.Orderbook.Bids
		index, order = orderList.Find(id)
		if index < 0 || order == nil {
			return nil
		}
	}
	oldOrder := heap.Remove(orderList, index).(*Order)
	return oldOrder
}

func (e *Engine) checkAndUpdateOrder(old *Order, req *pb.UpdateOrderRequest) error {
	err := old.validate()
	if err != nil {
		return err
	}
	val, err := exutil.DecodeBigInt(req.Value)
	if err != nil {
		return err
	}
	vol, err := exutil.DecodeBigInt(req.Volume)
	if err != nil {
		return err
	}
	//check req, only allow adding fund to order
	switch old.Side {
	case pb.OrderSide_ASK:
		if old.Volume.Cmp(vol) >= 0 {
			return fmt.Errorf("cannot update order because volume change from %v to %v", old.Volume.String(), vol.String())
		}
		old.Volume = vol
		old.RemainingVolume = exutil.SubInt(vol, old.FilledVolume)
	case pb.OrderSide_BID:
		if old.Value.Cmp(val) >= 0 {
			return fmt.Errorf("cannot update order because value change from %v to %v", old.Value.String(), val.String())
		}
		old.Value = val
		old.RemainingValue = exutil.SubInt(val, old.FilledValue)
	}
	return nil
}

func (e *Engine) tryTrade(evt *OutEvent, order *Order) {

	orderList := e.Orderbook.cob(order.Side)

	for orderList.Len() != 0 {
		i, oib := 0, orderList.First()

		log.Infof("Matching with %vth order: %v", i, oib)
		if matched, msg := orderMatchCompare(order, oib); !matched {
			log.Infof(msg)
			// If cannot trade with the first order.
			break
		}
		// Else the two orders are tradable. Trade
		trade, err := e.matchOrder(order, oib)

		if err != nil || trade == nil {
			log.Errorln("Unable to trade tradable orders: ", err, *order)
			evt.AddResult(pb.EngineEventResult_NEW_ORDER_FAILED, err.Error())
			break
		}
		log.Infoln("Traded ", formatTradeToString((*pb.TradeDefined)(trade)))

		if trade != nil {
			evt.AddTrade(trade)
			if oib.isComplete() {
				// TODO change to "==" when using int
				// Oib is totally fullfilled
				heap.Pop(orderList)
			}
		}

		if order.isComplete() {
			// Incoming order is totally fullfilled
			// Jump out of the entire loop
			log.Infof("Filled %v/%v\n", order.FilledVolume, order.Volume)
			break
		}
	}

	// todo , review the logic, if bid side market order, it will check the filled value, why?
	if order.RemainingVolume.Sign() > 0 {
		log.Infof("Adding new order to order book. Leftover volume %v", order.RemainingVolume)
		e.insert(order)
	}

}

// empty for test purpose
func (e *Engine) empty() bool {
	return len(e.Orderbook.Asks) == 0 && len(e.Orderbook.Bids) == 0
}

// clear for test purpose
func (e *Engine) clear() {
	e.Orderbook.Asks = e.Orderbook.Asks[:0]
	e.Orderbook.Bids = e.Orderbook.Bids[:0]
}

func (e *Engine) String() string {
	var sb strings.Builder
	sb.WriteString("Asks: " + e.Orderbook.Asks.String())
	sb.WriteString("Bids: " + e.Orderbook.Bids.String())
	return sb.String()
}
