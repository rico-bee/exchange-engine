package main

// High level (manual) testing that (hopefully) acts like postman

import (
	"math/big"
	"time"

	"github.com/Shopify/sarama"
	uuid "github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"gitlab.com/sdce/exlib/exutil"
	"gitlab.com/sdce/exlib/kafka"
	"gitlab.com/sdce/exlib/kafka/topic"
	pb "gitlab.com/sdce/protogo"
)

const (
	port = ":50051"
)

var (
	btcAudInstrument = &pb.Instrument{
		Id:    randUUID(),
		Code:  "btcaud",
		Name:  "BTC/AUD",
		Base:  randUUID(),
		Quote: randUUID(),
	}
)

func randUUID() *pb.UUID {
	return &pb.UUID{Bytes: uuid.NewV4().Bytes()}
}

func sampleOrder(side pb.OrderSide, tp pb.OrderType, vol, val, filledVol, filledVal string) *pb.OrderDefined {
	valNum, _ := exutil.DecodeBigInt(val)
	volNum, _ := exutil.DecodeBigInt(vol)
	price, _ := new(big.Float).Quo(new(big.Float).SetInt(valNum), new(big.Float).SetInt(volNum)).Float64()
	return &pb.OrderDefined{
		Id:         randUUID(),
		Source:     "client",
		Side:       side,
		Type:       tp,
		Price:      price,
		Value:      val,
		Volume:     vol,
		Instrument: btcAudInstrument,
		Owner: &pb.Participant{
			BrokerId: randUUID(),
			ClientId: randUUID(),
		},
		FilledVolume: filledVol,
		FilledValue:  filledVal,
		Status:       pb.OrderStatus_OPEN,
		Time:         time.Now().UnixNano(),
	}
}

var sampleAsk = sampleOrder(pb.OrderSide_ASK, pb.OrderType_LIMIT, "13", "5", "0", "0") // 5 / 13 = 0.384615
var sampleBid = sampleOrder(pb.OrderSide_BID, pb.OrderType_LIMIT, "10", "5", "0", "0") // 5 / 10 = 0.5

func getConfigs() (kafkaConf kafka.Config, err error) {
	var v = viper.New()
	v.SetConfigFile("config/local.yaml")
	err = v.ReadInConfig()
	if err != nil {
		return
	}
	kafkaConf, err = kafka.GetConfig(v)
	if err != nil {
		return
	}
	return
}

func main() {
	log.SetReportCaller(true)
	log.SetFormatter(&log.JSONFormatter{})

	// init consumer

	kafkaConf, err := getConfigs()
	if err != nil {
		log.Errorln("Cannot load configs", err)
	}
	producer := kafka.NewProducer(kafkaConf)
	defer producer.Close()

	askByte, err := sampleAsk.Marshal()
	if err != nil {
		log.Println("Unable to marshal message: ", err)
	}
	bidByte, err := sampleBid.Marshal()
	if err != nil {
		log.Println("Unable to marshal message: ", err)
	}

	log.Infof("Sending message to kafka...")
	// for i := 0; i < 100; i++ {
	producer.SyncProducer.SendMessage(
		&sarama.ProducerMessage{
			Topic: topic.EngineInPrefix + "audbtc",
			Value: sarama.ByteEncoder(askByte),
		},
	)
	log.Infof("Ask sent, sending bid...")

	producer.SyncProducer.SendMessage(
		&sarama.ProducerMessage{
			Topic: topic.EngineInPrefix + "audbtc",
			Value: sarama.ByteEncoder(bidByte),
		},
	)

}
