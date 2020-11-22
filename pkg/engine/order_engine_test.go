package engine

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"math/big"
	"strconv"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/google/go-cmp/cmp"
	"gitlab.com/sdce/exlib/exutil"
	pb "gitlab.com/sdce/protogo"
)

func TestLimitOrder(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine("btcaud")
	eventsIn, eventsOut := engine.EventsInput(), engine.EventsOutput()
	go engine.Start(ctx)
	aid := exutil.NewUUID()
	bid := exutil.NewUUID()
	ao := sampleAskEvent(aid, pb.OrderType_LIMIT, "5", "0", "0", "0.384615") // val=5,pr=0.384615,vol=5/0.384615=13
	bo := sampleBidEvent(bid, pb.OrderType_LIMIT, "10", "0", "0", "0.5")     // vol=10,pr=0.5,val=10*0.5=5
	go func() {
		logrus.Debug("Before: ", engine)
		eventsIn <- ao
		eventsIn <- bo
		logrus.Debug("After: ", engine)
	}()
	err := checkOrder(eventsOut, "5") // no trade, output 1st order
	if err != nil {
		t.Fatal(err.Error())
	}
	err = checkTrade(eventsOut, "10", "5") // 2nd order trigger trade
	if err != nil {
		t.Fatal(err.Error())
	}
	err = checkOrderbook(*engine.Orderbook, OrderList{}, OrderList{})
	if err != nil {
		t.Fatal(err.Error())
	}
	err = noMoreEvent(eventsOut)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func TestLimitOrder2(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engine := NewEngine("btcaud")
	eventsIn, eventsOut := engine.EventsInput(), engine.EventsOutput()
	engine.Start(ctx)
	defer engine.Close()
	aid := exutil.NewUUID()
	bid := exutil.NewUUID()
	bo := sampleBidEvent(bid, pb.OrderType_LIMIT, "10", "0", "0", "0.5")     // 5 / 10 = 0.5
	ao := sampleAskEvent(aid, pb.OrderType_LIMIT, "5", "0", "0", "0.384615") // 5 / 13 = 0.384615
	go func() {
		logrus.Debug("Before: ", engine)
		eventsIn <- bo
		eventsIn <- ao
		logrus.Debug("After: ", engine)
	}()
	err := checkOrder(eventsOut, "10")
	if err != nil {
		t.Fatal(err.Error())
	}
	err = checkTrade(eventsOut, "10", "5")
	if err != nil {
		t.Fatal(err.Error())
	}

	asks := OrderList{
		OrderFromProto(sampleOrder(aid, pb.OrderSide_ASK, pb.OrderType_LIMIT, "13", "5", "10", "5"))}
	err = checkOrderbook(*engine.Orderbook, asks, OrderList{})
	if err != nil {
		t.Fatal(err.Error())
	}

	err = noMoreEvent(eventsOut)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func TestLimitOrder3(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine("btcaud")
	eventsIn, eventsOut := engine.EventsInput(), engine.EventsOutput()
	go engine.Start(ctx)
	t.Log("Before: ", engine)
	aid := exutil.NewUUID()
	bid := exutil.NewUUID()
	bo := sampleBidEvent(bid, pb.OrderType_LIMIT, "5", "0", "0", "2.6") // 13 / 5 = 2.6
	ao := sampleAskEvent(aid, pb.OrderType_LIMIT, "10", "0", "0", "2")  // 10 / 5 = 2
	go func() {
		eventsIn <- bo
		eventsIn <- ao
	}()
	t.Log("After: ", engine)

	err := checkOrder(eventsOut, "5")
	if err != nil {
		t.Fatal(err.Error())
	}
	err = checkTrade(eventsOut, "5", "10")
	if err != nil {
		t.Fatal(err.Error())
	}
	err = checkOrderbook(*engine.Orderbook, OrderList{}, OrderList{})
	if err != nil {
		t.Fatal(err.Error())
	}
	err = noMoreEvent(eventsOut)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func TestMarketOrder(t *testing.T) {
	ctx := context.Background()
	defer ctx.Done()
	engine := NewEngine("btcaud")
	eventsIn, eventsOut := engine.EventsInput(), engine.EventsOutput()
	go engine.Start(ctx)
	defer engine.Close()

	aid := exutil.NewUUID()
	bid := exutil.NewUUID()
	bo := sampleBidEvent(bid, pb.OrderType_MARKET, "10", "0", "0", "0")      // 0 / 10 = 0
	ao := sampleAskEvent(aid, pb.OrderType_LIMIT, "5", "0", "0", "0.384615") // 5 / 13 = 0.384615
	go func() {
		t.Log("Before: ", engine)
		eventsIn <- bo
		eventsIn <- ao
		t.Log("After: ", engine)
	}()
	err := checkOrder(eventsOut, "10")
	if err != nil {
		t.Fatal(err.Error())
	}
	err = checkTrade(eventsOut, "10", "3")
	if err != nil {
		t.Fatal(err.Error())
	}
	err = noMoreEvent(eventsOut)
	if err != nil {
		t.Fatal(err.Error())
	}
	asks := OrderList{
		OrderFromProto(sampleOrder(aid, pb.OrderSide_ASK, pb.OrderType_LIMIT, "13", "5", "10", "4"))}

	err = checkOrderbook(*engine.Orderbook, asks,
		OrderList{})
	if err != nil {
		t.Fatal(err.Error())
	}
}

func TestSpread(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine("btcaud")
	eventsIn, eventsOut := engine.EventsInput(), engine.EventsOutput()
	go engine.Start(ctx)

	aid := exutil.NewUUID()
	bid := exutil.NewUUID()
	ao := sampleAskEvent(aid, pb.OrderType_LIMIT, "5", "0", "0", "0.5")  // Using 5 aud to buy at 0.384615
	bo := sampleBidEvent(bid, pb.OrderType_LIMIT, "10", "0", "0", "0.4") // Selling btc at 4 / 10 = 0.4
	go func() {
		t.Log("Before: ", engine)
		eventsIn <- ao
		eventsIn <- bo
		t.Log("After: ", engine)
	}()
	err := checkOrder(eventsOut, "5")
	if err != nil {
		t.Fatal(err.Error())
	}
	err = checkOrder(eventsOut, "10")
	if err != nil {
		t.Fatal(err.Error())
	}
	err = noMoreEvent(eventsOut)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func TestCancelOrder(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine("btcaud")
	eventsIn, eventsOut := engine.EventsInput(), engine.EventsOutput()
	go engine.Start(ctx)

	aid := exutil.NewUUID()
	bid := exutil.NewUUID()
	ao := sampleAskEvent(aid, pb.OrderType_LIMIT, "5", "0", "0", "0.384615") // 13 / 5
	co := sampleCancelEvent(aid)
	bo := sampleBidEvent(bid, pb.OrderType_LIMIT, "10", "0", "0", "0.5") // 5 / 10
	go func() {
		t.Log("Before: ", engine)
		eventsIn <- ao
		eventsIn <- co
		eventsIn <- bo
		t.Log("After: ", engine)
	}()
	err := checkOrder(eventsOut, "5")
	if err != nil {
		t.Fatal(err.Error())
	}

	err = checkCancelOrder(eventsOut, co.GetCancelOrder().GetId())
	if err != nil {
		t.Fatal(err.Error())
	}

	err = checkOrder(eventsOut, "10")
	if err != nil {
		t.Fatal(err.Error())
	}
	err = noMoreEvent(eventsOut)
	if err != nil {
		t.Fatal(err.Error())
	}
}

type ExpectFunc func() error
type ExpectFuncs []ExpectFunc

// TestSuite
func TestSuite(t *testing.T) {

	ctx := context.Background()
	engine := NewEngine("btcaud")
	eventsIn, eventsOut := engine.EventsInput(), engine.EventsOutput()
	go engine.Start(ctx)

	expt := expect()
	expt.SetEngine(engine)

	type testElem struct {
		name      string
		inputs    [](*InEvent)
		wantResps []ExpectFuncs
	}
	// allocate 5 ids on each side for specified accessing
	askIds := []*pb.UUID{exutil.NewUUID(), exutil.NewUUID(), exutil.NewUUID(), exutil.NewUUID(), exutil.NewUUID()}
	bidIds := []*pb.UUID{exutil.NewUUID(), exutil.NewUUID(), exutil.NewUUID(), exutil.NewUUID(), exutil.NewUUID()}
	tests := []testElem{
		{
			name: "limit order",
			inputs: []*InEvent{
				// ask  5/0.384615=13
				sampleAskEvent(askIds[0], pb.OrderType_LIMIT, "13", "0", "0", "0.384615"),
				// bid  5/0.5=10
				sampleBidEvent(bidIds[0], pb.OrderType_LIMIT, "5", "0", "0", "0.5"),
				// deal 0.384615->5 13
			},
			wantResps: []ExpectFuncs{
				expt.OrderVolume("13").Exports(),
				expt.OrderValue("5").TradesValue([]string{"5"}).TradesVolume([]string{"13"}).
					OrderBookSize(pb.OrderSide_ASK, 0).OrderBookSize(pb.OrderSide_BID, 0).Exports(),
			},
		},
		{
			name: "limit order 1",
			inputs: []*InEvent{
				// bid  5/0.5=10
				sampleBidEvent(bidIds[0], pb.OrderType_LIMIT, "5", "0", "0", "0.5"),
				// ask  5/0.384615=13
				sampleAskEvent(askIds[0], pb.OrderType_LIMIT, "13", "0", "0", "0.384615"),
				// deal 0.5->5 10
			},
			wantResps: []ExpectFuncs{
				expt.OrderValue("5").Exports(),
				expt.OrderVolume("13").TradesValue([]string{"5"}).TradesVolume([]string{"10"}).
					OrderBookSize(pb.OrderSide_ASK, 1).OrderBookSize(pb.OrderSide_BID, 0).Exports(),
			},
		},
		{
			name: "exception 1",
			inputs: []*InEvent{
				sampleAskEvent(askIds[0], pb.OrderType_LIMIT, "1000000", "0", "0", "579874"),
				sampleBidEvent(bidIds[0], pb.OrderType_LIMIT, "579874000000", "0", "0", "579874"),
			},
			wantResps: []ExpectFuncs{
				expt.OrderVolume("1000000").Exports(),
				expt.OrderValue("579874000000").TradesValue([]string{"579874000000"}).TradesVolume([]string{"1000000"}).
					OrderBookSize(pb.OrderSide_ASK, 0).OrderBookSize(pb.OrderSide_BID, 0).Exports(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tctx, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			// give inputs
			go func() {
				for _, in := range tt.inputs {
					eventsIn <- in
				}
			}()
			// expect outputs
			for _, foos := range tt.wantResps {
				select {
				case out := <-eventsOut:
					expt.SetOutEvent(out)
					for _, foo := range foos {
						err := foo()
						if err != nil {
							t.Fatal(err.Error())
						}
					}
				case <-tctx.Done():
					t.Fatal("Timeout, output is blocked")
				}
			}
			// expect clean outputs
			err := noMoreEvent(eventsOut)
			if err != nil {
				t.Fatal(err.Error())
			}
			// clean engine for the next test
			if !engine.empty() {
				engine.clear()
			}
		})
	}
}

func BenchmarkLimitOrders(b *testing.B) {
	logrus.SetOutput(ioutil.Discard)
	nAsk := 100
	nBid := 7
	price := 405
	totalVol := nAsk * nBid * 30

	orderEvents := []*InEvent{}

	ctx := context.Background()
	engine := NewEngine("btcaud")
	eventsIn, eventsOut := engine.EventsInput(), engine.EventsOutput()
	go engine.Start(ctx)
	for i := 0; i < nAsk; i++ {
		orderEvents = append(orderEvents,
			sampleAskEvent(exutil.NewUUID(), pb.OrderType_LIMIT, strconv.Itoa(totalVol/nAsk), "0", "0", strconv.Itoa(price)))
	}

	for i := 0; i < nBid; i++ {
		orderEvents = append(orderEvents,
			sampleBidEvent(exutil.NewUUID(), pb.OrderType_LIMIT, strconv.Itoa(totalVol/nBid*price), "0", "0", strconv.Itoa(price)))
	}

	b.ResetTimer()
	for round := 0; round < b.N; round++ {
		for _, order := range orderEvents {
			eventsIn <- order
			<-eventsOut
		}

		if len(engine.Orderbook.Asks) != 0 {
			b.Errorf("Expecting no remaining ask order. Got %v", len(engine.Orderbook.Asks))
			b.Fail()
		}
		if len(engine.Orderbook.Bids) != 0 {
			b.Errorf("Expecting no remaining bid order. Got %v", len(engine.Orderbook.Bids))
			b.Fail()
		}
	}
}

///helper functions/////////////////////////

func init() {
	// Make it colorful! Effective in terminal (not effective in vscode test output.)
	logrus.SetFormatter(&logrus.TextFormatter{
		ForceColors: true,
	})
	logrus.SetLevel(logrus.TraceLevel)
}

func sampleOrder(id *pb.UUID, side pb.OrderSide, tp pb.OrderType, vol, val, filledVol, filledVal string) *pb.OrderDefined {
	valNum, _ := exutil.DecodeBigInt(val)
	volNum, _ := exutil.DecodeBigInt(vol)
	price, _ := exutil.QuoFloat(fl(valNum), fl(volNum)).Float64()
	return &pb.OrderDefined{
		Id:     id,
		Source: "client",
		Side:   side,
		Type:   tp,
		Price:  price,
		Value:  val,
		Volume: vol,
		Instrument: &pb.InstrumentRef{
			Code: "audbtc",
			// Description
			Quote: &pb.CurrencyRef{
				Id: exutil.NewUUID(),
			},
			Base: &pb.CurrencyRef{
				Id: exutil.NewUUID(),
			},
		},
		Owner: &pb.Participant{
			BrokerId: exutil.NewUUID(),
			ClientId: exutil.NewUUID(),
		},
		FilledVolume: filledVol,
		FilledValue:  filledVal,
		Status:       pb.OrderStatus_OPEN,
		Time:         time.Now().UnixNano(),
	}
}

// sampleAskEvent return new ask order event
func sampleAskEvent(id *pb.UUID, tp pb.OrderType, vol, filledVol, filledVal, price string) *InEvent {
	volNum, _ := exutil.DecodeBigInt(vol)
	priceNum, _ := new(big.Float).SetString(price)
	valNum, _ := addf(exutil.MulFloat(fl(volNum), priceNum), float64(0.00001)).Int(nil)

	o := sampleOrder(id, pb.OrderSide_ASK, tp, volNum.Text(10), valNum.Text(10), filledVal, filledVol)
	logrus.Info(o)

	return &InEvent{
		&pb.EngineEventIn{
			Event: &pb.EngineEventIn_NewOrder{NewOrder: o},
		},
		&EventCoord{
			Partition: 0,
			Offset:    1,
		},
	}
}

// sampleBidEvent return new bid order event
func sampleBidEvent(id *pb.UUID, tp pb.OrderType, val, filledVol, filledVal, price string) *InEvent {
	valNum, _ := exutil.DecodeBigInt(val)
	priceNum, _ := new(big.Float).SetString(price)
	volNum, _ := exutil.QuoFloat(fl(valNum), priceNum).Int(nil)
	o := sampleOrder(id, pb.OrderSide_BID, tp, volNum.Text(10), valNum.Text(10), filledVal, filledVol)
	logrus.Info(o)
	return &InEvent{
		&pb.EngineEventIn{
			Event: &pb.EngineEventIn_NewOrder{o},
		},
		&EventCoord{
			Partition: 0,
			Offset:    1,
		},
	}
}

// sampleCancelEvent return order cancel event
func sampleCancelEvent(id *pb.UUID) *InEvent {
	return &InEvent{
		EngineEventIn: &pb.EngineEventIn{
			Event: &pb.EngineEventIn_CancelOrder{
				CancelOrder: &pb.CancelOrderRequest{
					Id:     id,
					Reason: "want",
				},
			},
		},
	}
}

// sampleUpdateEvent return order update event
func sampleUpdateEvent(id *pb.UUID, ot pb.OrderType, val, vol string, price float64) *InEvent {
	return &InEvent{
		EngineEventIn: &pb.EngineEventIn{
			Event: &pb.EngineEventIn_UpdateOrder{
				UpdateOrder: &pb.UpdateOrderRequest{
					Id:     id,
					Type:   ot,
					Value:  val,
					Volume: vol,
					Price:  price,
				},
			},
		},
	}
}

////////////////////
type Expect struct {
	OrderId *pb.UUID
	Event   *pb.OrderEvent
	Order   *pb.OrderDefined
	Trades  []*pb.TradeDefined
	Book    *OrderBook
	Funcs   []ExpectFunc
}

func expect() *Expect {
	return &Expect{}
}

func (exp *Expect) SetOutEvent(out *OutEvent) {
	exp.OrderId = nil
	exp.Event = nil
	exp.Order = nil
	exp.Trades = nil

	switch out.GetEvent().(type) {
	case *pb.EngineEventOut_NewOrder:
		exp.Event = out.GetNewOrder().GetEvent()
		exp.OrderId = out.GetNewOrder().GetOrderId()
		exp.Trades = out.GetNewOrder().GetTrades()
		exp.Order = out.GetNewOrder().GetOrder()
	case *pb.EngineEventOut_UpdateOrder:
		exp.Event = out.GetUpdateOrder().GetEvent()
		exp.OrderId = out.GetUpdateOrder().GetOrderId()
		exp.Trades = out.GetUpdateOrder().GetTrades()
	case *pb.EngineEventOut_CancelOrder:
		exp.Event = out.GetCancelOrder().GetEvent()
		exp.OrderId = out.GetCancelOrder().GetOrderId()
	}
}

func (exp *Expect) SetEngine(ng *Engine) {
	exp.Book = ng.Orderbook
}

func (exp *Expect) Exports() []ExpectFunc {
	funcs := exp.Funcs
	exp.Funcs = []ExpectFunc{}
	return funcs
}

func (exp *Expect) OrderValue(value string) *Expect {
	f := func() error {
		evt := exp.Event
		if evt.GetUpdateToValue() != value {
			return fmt.Errorf("Expected order value %v, got %v", value, evt.GetUpdateToValue())
		}
		return nil
	}
	exp.Funcs = append(exp.Funcs, f)
	return exp
}

func (exp *Expect) OrderVolume(vol string) *Expect {
	f := func() error {
		evt := exp.Event
		if evt.GetUpdateToVolume() != vol {
			return fmt.Errorf("Expected order volume %v, got %v", vol, evt.GetUpdateToVolume())
		}
		return nil
	}
	exp.Funcs = append(exp.Funcs, f)
	return exp
}

func (exp *Expect) TradesVolume(vol []string) *Expect {
	f := func() error {
		trades := exp.Trades
		if len(vol) != len(trades) {
			return fmt.Errorf("TradesVolume length not consist, expect %v, got %v", len(vol), len(trades))
		}
		for i, trade := range trades {
			if trade.Volume != vol[i] {
				return fmt.Errorf("Expected trading[%v] volume %v, got %v", i, vol[i], trade.Volume)
			}
		}
		return nil
	}
	exp.Funcs = append(exp.Funcs, f)
	return exp
}

func (exp *Expect) TradesValue(val []string) *Expect {
	f := func() error {
		trades := exp.Trades
		if len(val) != len(trades) {
			return fmt.Errorf("TradesValue length not consist, expect %v, got %v", len(val), len(trades))
		}
		for i, trade := range trades {
			if trade.Value != val[i] {
				return fmt.Errorf("Expected trading[%v] value %v, got %v", i, val[i], trade.Value)
			}
		}
		return nil
	}
	exp.Funcs = append(exp.Funcs, f)
	return exp
}

func (exp *Expect) OrderID(id *pb.UUID) *Expect {
	f := func() error {
		oid := exp.OrderId
		if bytes.Compare(oid.Bytes, id.Bytes) != 0 {
			return fmt.Errorf("Expected order id %v, got %v", id.Bytes, oid.Bytes)
		}
		return nil
	}
	exp.Funcs = append(exp.Funcs, f)
	return exp
}

func (exp *Expect) OrderBookSize(side pb.OrderSide, sz int) *Expect {
	f := func() error {
		var olist OrderList
		switch side {
		case pb.OrderSide_ASK:
			olist = exp.Book.Asks
		case pb.OrderSide_BID:
			olist = exp.Book.Bids
		}

		if olist.Len() != sz {
			return fmt.Errorf("Expecting orderbook size %v, got %v", sz, olist.Len())
		}
		return nil
	}
	exp.Funcs = append(exp.Funcs, f)
	return exp
}

func (exp *Expect) TopOrder(side pb.OrderSide, vol, val, filVol, filVal string) *Expect {
	f := func() error {
		var olist OrderList
		switch side {
		case pb.OrderSide_ASK:
			olist = exp.Book.Asks
		case pb.OrderSide_BID:
			olist = exp.Book.Bids
		}
		top := olist.First()
		if top == nil {
			return fmt.Errorf("no order in book")
		}
		order := &Order{}
		order.Volume, _ = exutil.DecodeBigInt(vol)
		order.Value, _ = exutil.DecodeBigInt(val)
		order.FilledVolume, _ = exutil.DecodeBigInt(filVol)
		order.FilledValue, _ = exutil.DecodeBigInt(filVal)
		if top.Volume.Cmp(order.Volume) != 0 ||
			top.Value.Cmp(order.Value) != 0 ||
			top.FilledVolume.Cmp(order.FilledVolume) != 0 ||
			top.FilledValue.Cmp(order.FilledValue) != 0 {
			return fmt.Errorf("Expecting order %v, got %v", *order, *top)
		}
		return nil
	}
	exp.Funcs = append(exp.Funcs, f)
	return exp
}

////legacy//////////////////////////

func checkOrder(evts <-chan *OutEvent, amount string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()
	var out *OutEvent
	select {
	case out = <-evts:
	case <-ctx.Done():
		return fmt.Errorf("Expected order amount %v, timed out", amount)
	}

	order := out.GetNewOrder().GetOrder()
	switch order.GetSide() {
	case pb.OrderSide_ASK:
		if order.GetValue() != amount {
			return fmt.Errorf("Expected ask order value %v, got %v", amount, order.GetValue())
		}
	case pb.OrderSide_BID:
		if order.GetVolume() != amount {
			return fmt.Errorf("Expected bid order volume %v, got %v", amount, order.GetVolume())
		}
	}
	return nil
}

func checkTrade(newT <-chan *OutEvent, vol, val string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()
	var out *OutEvent
	select {
	case out = <-newT:
	case <-ctx.Done():
		return fmt.Errorf("Expected trading volume %v, timed out", vol)
	}

	trade := out.GetNewOrder().GetTrades()[0]
	if trade.Volume != vol {
		return fmt.Errorf("Expected trading volume %v, got %v", vol, trade.Volume)
	}
	if trade.Value != val {
		return fmt.Errorf("Expected trading value %v, got %v", val, trade.Value)
	}
	return nil
}

func checkCancelOrder(evts <-chan *OutEvent, id *pb.UUID) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()
	var out *OutEvent
	select {
	case out = <-evts:
	case <-ctx.Done():
		return fmt.Errorf("Expected cancelling order %v, timed out", id.Bytes)
	}

	order := out.GetCancelOrder()
	if bytes.Compare(order.GetOrderId().Bytes, id.Bytes) != 0 {
		return fmt.Errorf("Expected order id %v, got %v", id.Bytes, order.GetOrderId().Bytes)
	}
	return nil
}

func noMoreEvent(events <-chan *OutEvent) error {
	timer := time.NewTimer(time.Millisecond * 100)
	defer timer.Stop()
	select {
	case out, ok := <-events:
		return fmt.Errorf("Expected no trading. Got %v %v", out, ok)
	case <-timer.C:
	}
	return nil
}

func checkOrderbook(o OrderBook, asks OrderList, bids OrderList) error {
	comparer := cmp.Comparer(func(a Order, b Order) bool {
		return a.Volume.Cmp(b.Volume) == 0 &&
			a.Price.Cmp(b.Price) == 0 &&
			a.FilledVolume.Cmp(b.FilledVolume) == 0
	})

	if !cmp.Equal(o.Asks, asks, comparer) {
		return fmt.Errorf("Expecting asks %v, got %v", asks, o.Asks)
	}
	if !cmp.Equal(o.Bids, bids, comparer) {
		return fmt.Errorf("Expecting bids %v, got %v", bids, o.Bids)
	}
	return nil
}

func addf(x *big.Float, y float64) *big.Float {
	z := new(big.Float).SetFloat64(y)
	return z.Add(z, x)
}
