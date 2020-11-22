package engine

import (
	"fmt"
	"math/big"

	"gitlab.com/sdce/exlib/exutil"
	pb "gitlab.com/sdce/protogo"
)

func fl(x *big.Int) *big.Float {
	return new(big.Float).SetInt(x)
}

func min(a, b *big.Int) *big.Int {
	if a.Cmp(b) < 0 {
		return a
	}
	return b
}

func formatTradeToString(trade *pb.TradeDefined) string {
	if trade == nil {
		return ""
	}
	return fmt.Sprintf(" <trade: %v>, price:%v, val:%v, vol:%v, time:%v, bid_order:%v ask_order:%v ", exutil.UUIDtoA(trade.Id),
		trade.Price, trade.Value, trade.Volume, trade.Time,
		exutil.UUIDtoA(trade.Bid.Id), exutil.UUIDtoA(trade.Ask.Id))
}

func formatOrderToString(order *pb.OrderDefined) string {
	if order == nil {
		return ""
	}
	return fmt.Sprintf(" [oid:%v] side:%v, type:%v, vol:%v, val:%v, price:%v, fillVol:%v, fillVal:%v ",
		exutil.UUIDtoA(order.Id), order.Side, order.Type,
		order.Volume, order.Value, order.Price, order.FilledVolume, order.FilledValue)
}

func formatEventToString(event *pb.OrderEvent) string {
	if event == nil {
		return ""
	}
	return fmt.Sprintf(" <event:%v> type:%v, fromVal:%v, toVal:%v, fromVol:%v, toVol:%v, price:%v ",
		exutil.UUIDtoA(event.Id), event.Type,
		event.UpdateFromValue, event.UpdateToValue, event.UpdateFromVolume, event.UpdateToVolume, event.Price)
}

func formatOrderbookToString(ob *pb.OrderBook) string {
	if ob == nil {
		return ""
	}
	return fmt.Sprintf(" [orderBook] instId: %v, bids:%v, asks:%v ",
		exutil.UUIDtoA(ob.InstrumentId), len(ob.Bids), len(ob.Asks))
}

func formatInEventToString(in *InEvent) string {
	if in == nil {
		return ""
	}
	eventStr := fmt.Sprintf("inevent id %v: ", in.Id)
	if order := in.GetNewOrder(); order != nil {
		return eventStr + formatOrderToString(order)
	} else if req := in.GetUpdateOrder(); req != nil {
		return eventStr + fmt.Sprintf(" [update order] id: %v, type:%v, val:%v, vol:%v, price:%v ",
			exutil.UUIDtoA(req.Id), req.Type, req.Value, req.Volume, req.Price)
	} else if req := in.GetCancelOrder(); req != nil {
		return eventStr + fmt.Sprintf(" [cancel order] id: %v, reason: %v ",
			exutil.UUIDtoA(req.Id), req.Reason)
	}
	return ""
}
