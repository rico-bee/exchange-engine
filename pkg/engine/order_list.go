package engine

import (
	"bytes"
	"fmt"
	"strings"

	pb "gitlab.com/sdce/protogo"
)

// OrderList is an ordered list to store orders
// implements container/heap
type OrderList []*Order

func (o OrderList) Len() int { return len(o) }

// Less reports whether the element with
// index i should sort before the element with index j.
func (o OrderList) Less(i, j int) bool {
	var io, jo = o[i], o[j]
	it := io.Time
	jt := jo.Time

	switch {
	case io.Type == pb.OrderType_MARKET && jo.Type == pb.OrderType_MARKET:
		{
			return it < jt
		}
	case io.Type != pb.OrderType_MARKET && jo.Type != pb.OrderType_MARKET:
		{
			if io.Price == jo.Price {
				// For (limit) orders with same price
				// Earlier one goes first
				return it < jt
			}
			if io.Side == pb.OrderSide_ASK {
				// For ask/offer/sell order
				// Lower price goes first
				return io.Price.Cmp(jo.Price) < 0
			} else {
				// For bid/request/buy order
				// Higher price goes first
				return io.Price.Cmp(jo.Price) > 0
			}
		}
	default:
		return io.Type == pb.OrderType_MARKET
	}
}

// Swap swaps the elements with indexes i and j.
func (o OrderList) Swap(i, j int) {
	o[j], o[i] = o[i], o[j]
}

// Push appends the item to the end. Takes (x *Order) as parameter
func (o *OrderList) Push(x interface{}) {
	item := x.(*Order)
	*o = append(*o, item)
}

// Pop is used by heap package.
// removes the last item and return it
func (o *OrderList) Pop() interface{} {
	old := *o
	n := len(old)
	item := old[n-1]
	*o = old[0 : n-1]
	return item
}

func (o OrderList) String() string {
	var sb strings.Builder
	for i, order := range o {
		sb.WriteString(fmt.Sprintf("%v|%v,", i, order.String()))
	}
	return fmt.Sprintf("OrderList[%v] %v\n", o.Len(), sb.String())
}

// First return the first item of the sorted heap
// It does not belong to the Interface of heap
func (o OrderList) First() *Order {
	if len(o) == 0 {
		return nil
	}
	return o[0]
}

// Find return the index and order object
// return -1 and nil if not found
func (o OrderList) Find(id *pb.UUID) (int, *Order) {
	for i, order := range o {
		if bytes.Compare(id.Bytes, order.Id.Bytes) == 0 {
			return i, order
		}
	}
	return -1, nil
}
