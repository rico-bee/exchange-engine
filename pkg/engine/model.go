package engine

import (
	"fmt"

	"gitlab.com/sdce/exlib/exutil"

	pb "gitlab.com/sdce/protogo"
)

// EventCoord is the coordinate of an input event in message queue
// in kafka cluster, we need partition index and offset to locate a specific event
type EventCoord struct {
	Partition int32
	Offset    int64
}

// InEvent is used for packing input events from outside
type InEvent struct {
	*pb.EngineEventIn
	*EventCoord
}

// OutEvent is used for packing output events to outside
type OutEvent struct {
	*pb.EngineEventOut
	Code string
}

// AddTrade append a trade into itself
func (evt *OutEvent) AddTrade(trade *Trade) {
	switch evt.GetEvent().(type) {
	case *pb.EngineEventOut_NewOrder:
		evt.GetNewOrder().Trades = append(evt.GetNewOrder().Trades, (*pb.TradeDefined)(trade))
	case *pb.EngineEventOut_UpdateOrder:
		evt.GetUpdateOrder().Trades = append(evt.GetUpdateOrder().Trades, (*pb.TradeDefined)(trade))
	}
}

//AddResult set a result to itself
func (evt *OutEvent) AddResult(result pb.EngineEventResult, reason string) {
	evt.Result = result
	evt.Reason = reason
}
func (evt *OutEvent) String() string {
	var fullStr string
	fullStr += fmt.Sprintf(" Code: %v, Result: %v, Reason: %v ", evt.Code, evt.Result, evt.Reason)
	if cpl := evt.GetNewOrder(); cpl != nil {
		order := cpl.GetOrder()
		trades := cpl.GetTrades()
		event := cpl.GetEvent()
		fullStr += formatOrderToString(order)
		if trades != nil {
			for i, trade := range trades {
				fullStr += fmt.Sprintf(" %v|", i)
				fullStr += formatTradeToString(trade)
			}
		}
		fullStr += formatEventToString(event)
	} else if cpl := evt.GetUpdateOrder(); cpl != nil {
		oid := cpl.GetOrderId()
		trades := cpl.GetTrades()
		event := cpl.GetEvent()
		var fullStr string
		if oid != nil {
			fullStr += fmt.Sprintf("[oid:%v] ", exutil.UUIDtoA(oid))
		}
		if trades != nil {
			for i, trade := range trades {
				fullStr += fmt.Sprintf(" %v|", i)
				fullStr += formatTradeToString(trade)
			}
		}
		fullStr += formatEventToString(event)
	} else if cpl := evt.GetCancelOrder(); cpl != nil {
		oid := cpl.GetOrderId()
		event := cpl.GetEvent()
		var fullStr string
		if oid != nil {
			fullStr += fmt.Sprintf("[oid:%v] ", exutil.UUIDtoA(oid))
		}
		fullStr += formatEventToString(event)
	}

	return fullStr
}
