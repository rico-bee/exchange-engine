package engine

import (
	"fmt"
	"math/big"

	log "github.com/sirupsen/logrus"
	"gitlab.com/sdce/exlib/exutil"
	pb "gitlab.com/sdce/protogo"
)

// Order with big.Int helpers
type Order struct {
	Id *pb.UUID
	// Affiliate, Exchange
	Source string
	Side   pb.OrderSide
	Type   pb.OrderType
	Owner  *pb.Participant

	Price           *big.Float
	Value           *big.Int
	Volume          *big.Int
	FilledVolume    *big.Int
	FilledValue     *big.Int
	RemainingVolume *big.Int
	RemainingValue  *big.Int
	Status          pb.OrderStatus
	Time            int64
	Instrument      *pb.InstrumentRef
}

// OrderFromProto convert a protobuf struct into engine-using Order struct
func OrderFromProto(in *pb.OrderDefined) *Order {
	volf, ok := new(big.Float).SetString(in.Volume)
	if !ok {
		log.Errorf("failed to convert volume %v to big float", in.Volume)
		return nil
	}
	filledVolf, ok := new(big.Float).SetString(in.FilledVolume)
	if !ok {
		log.Errorf("failed to convert filledVolume %v to big float", in.FilledVolume)
		return nil
	}
	valf, ok := new(big.Float).SetString(in.Value)
	if !ok {
		log.Errorf("failed to convert value %v to big float", in.Value)
		return nil
	}
	filledValf, ok := new(big.Float).SetString(in.FilledValue)
	if !ok {
		log.Errorf("failed to convert filledvalue %v to big float", in.FilledValue)
		return nil
	}

	pricef := new(big.Float).SetFloat64(in.Price)
	if in.Type == pb.OrderType_LIMIT {
		if in.Side == pb.OrderSide_ASK {
			valf = valf.Mul(volf, pricef)
			filledValf = filledValf.Mul(filledVolf, pricef)
		} else {
			volf = volf.Quo(valf, pricef)
			filledVolf = filledVolf.Quo(filledValf, pricef)
		}
	}

	val, _ := valf.Int(nil)
	vol, _ := volf.Int(nil)
	filledVal, _ := filledValf.Int(nil)
	filledVol, _ := filledVolf.Int(nil)

	return &Order{
		Id: in.Id,
		//
		Source:          in.Source,
		Side:            in.Side,
		Type:            in.Type,
		Owner:           in.Owner,
		Price:           pricef,
		Value:           val,
		Volume:          vol,
		FilledVolume:    filledVol,
		FilledValue:     filledVal,
		RemainingVolume: exutil.SubInt(vol, filledVol),
		RemainingValue:  exutil.SubInt(val, filledVal),
		Status:          in.Status,
		Time:            in.Time,
		Instrument:      in.Instrument,
	}

}

// Fill specified amount
func (o *Order) fill(tradeVol *big.Int, tradeVal *big.Int) (err error) {
	newVolFilled, newValFilled := exutil.AddInt(o.FilledVolume, tradeVol), exutil.AddInt(o.FilledValue, tradeVal)
	// If filledVal > val || filledVol > vol
	// For ask orders, volume is fixed
	if o.Side == pb.OrderSide_ASK && newVolFilled.Cmp(o.Volume) > 0 {
		err = fmt.Errorf("over filled ask order: got %v when the order only have %v", newVolFilled, o.Volume)
		return
	}
	if o.Side == pb.OrderSide_BID && newValFilled.Cmp(o.Value) > 0 {
		err = fmt.Errorf("over filled bid order: got %v when the order only have %v", newValFilled, o.Value)
		return
	}
	o.FilledVolume = newVolFilled
	o.FilledValue = newValFilled
	o.RemainingVolume = exutil.SubInt(o.Volume, o.FilledVolume)
	o.RemainingValue = exutil.SubInt(o.Value, o.FilledValue)
	zero := new(big.Int)
	if o.RemainingValue.Cmp(zero) == 0 || o.RemainingVolume.Cmp(zero) == 0 {
		o.Status = pb.OrderStatus_FILLED
	} else {
		o.Status = pb.OrderStatus_PARTIAL_FILLED
	}
	return
}

// fixPrice for market order/stop order
// For market/stop order, price is not determined at the creation of the order.
func (o *Order) fixPrice(price *big.Float) error {

	val, _ := price.Float64()
	log.Warnf("trying to fix price to %v for order", val)
	fixf := new(big.Float).SetFloat64(0.5)
	switch o.Type {
	case pb.OrderType_ORDER_TYPE_INVALID:
		return fmt.Errorf("fixing price for invalid order")
	case pb.OrderType_ICEBERG:
		return fmt.Errorf("fixing price for iceberg order is not implemented")
	case pb.OrderType_STOP:
		return fmt.Errorf("fixing price for stop order is not implemented")
	case pb.OrderType_MARKET, pb.OrderType_LIMIT:
		switch o.Side {
		case pb.OrderSide_ORDER_SIDE_INVALID:
			return fmt.Errorf("fixing price for invalid side order")
		case pb.OrderSide_ASK:
			// For ask orders, volume is fixed
			// change value to match the price
			o.Value, _ = exutil.AddFloat(exutil.MulFloat(fl(o.Volume), price), fixf).Int(nil)
			o.FilledValue, _ = exutil.AddFloat(exutil.MulFloat(fl(o.FilledVolume), price), fixf).Int(nil)
			o.RemainingValue = exutil.SubInt(o.Value, o.FilledValue)
		case pb.OrderSide_BID:
			// For bid orders, value is fixed
			// change volume to match the price
			o.Volume, _ = exutil.AddFloat(exutil.QuoFloat(fl(o.Value), price), fixf).Int(nil)
			o.FilledVolume, _ = exutil.AddFloat(exutil.QuoFloat(fl(o.FilledValue), price), fixf).Int(nil)
			o.RemainingVolume = exutil.SubInt(o.Volume, o.FilledVolume)
		}
	}

	if o.Type == pb.OrderType_MARKET {
		// only market order change price
		o.Price = price
	}
	return nil
}

func (o Order) isComplete() bool {
	switch o.Side {
	case pb.OrderSide_ASK:
		return o.RemainingVolume.Sign() <= 0
	case pb.OrderSide_BID:
		return o.RemainingValue.Sign() <= 0
	default:
		log.Errorf("handling invalid order")
		return true
	}
}

func (o Order) validate() error {
	switch o.Side {
	case pb.OrderSide_ASK:
		if o.Volume.Sign() <= 0 {
			return fmt.Errorf("ask order cannot have volume of %v", o.Volume)
		}
	case pb.OrderSide_BID:
		if o.Value.Sign() <= 0 {
			return fmt.Errorf("bid order cannot have value of %v", o.Value)
		}
	default:
		return fmt.Errorf("invalid side: %v", o.Side)
	}
	switch o.Type {
	case pb.OrderType_MARKET:
	case pb.OrderType_LIMIT:
	case pb.OrderType_STOP, pb.OrderType_ICEBERG:
		return fmt.Errorf("%v order is not supported", o.Type)
	default:
		return fmt.Errorf("invalid type")
	}
	if o.isComplete() {
		return fmt.Errorf("already complete order")
	}
	return nil
}

func (o Order) String() string {
	return fmt.Sprintf("(Side:%v, RemVol:%v/%v, RemVal:%v/%v, Price:%v)", o.Side, o.RemainingVolume, o.Volume, o.RemainingValue, o.Value, o.Price)
}

// Copy set receiver o from src, and return o
func (o *Order) Copy(src *Order) *Order {
	*o = *src
	// cache all the items which may change
	o.Price = &big.Float{}
	o.Price.Copy(src.Price)
	o.Value = &big.Int{}
	o.Value.Add(o.Value, src.Value)
	o.Volume = &big.Int{}
	o.Volume.Add(o.Volume, src.Volume)
	o.FilledValue = &big.Int{}
	o.FilledValue.Add(o.FilledValue, src.FilledValue)
	o.FilledVolume = &big.Int{}
	o.FilledVolume.Add(o.FilledVolume, src.FilledVolume)
	o.RemainingValue = &big.Int{}
	o.RemainingValue.Add(o.RemainingValue, src.RemainingValue)
	o.RemainingVolume = &big.Int{}
	o.RemainingVolume.Add(o.RemainingVolume, src.RemainingVolume)
	return o
}
