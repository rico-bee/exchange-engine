package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"gitlab.com/sdce/exchange/engine/pkg"
	"gitlab.com/sdce/exchange/engine/pkg/api"
	"gitlab.com/sdce/exchange/engine/pkg/engine"
	"gitlab.com/sdce/exchange/engine/pkg/recorder"
	"gitlab.com/sdce/exlib/config"
	"gitlab.com/sdce/exlib/kafka"
	"gitlab.com/sdce/exlib/service"
	pb "gitlab.com/sdce/protogo"
)

const (
	configFileName = "exchange.engine"
	consumerGroup  = "match-engine"
)

func getConfigs() (kafkaConf kafka.Config, serviceConf service.Config, err error) {
	v, err := config.LoadConfig(configFileName)
	if err != nil {
		return
	}
	kafkaConf, err = kafka.GetConfig(v)
	if err != nil {
		return
	}
	serviceConf, err = service.GetConfig(v)
	if err != nil {
		return
	}
	return
}

func main() {
	log.SetReportCaller(true)
	log.SetFormatter(&log.JSONFormatter{})

	// init consumer

	codesStr := os.Args[1]
	codesStr = strings.Trim(codesStr, " ")
	codes := strings.Split(codesStr, " ")
	ctx := context.Background()

	// trap Ctrl+C and call cancel on the context
	ctx, cancel := context.WithCancel(ctx)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	defer func() {
		signal.Stop(c)
		log.Println("Stopping")
		cancel()
	}()

	go func() {
		select {
		case <-c:
			cancel()
		case <-ctx.Done():
		}
	}()

	kafkaConf, serviceConf, err := getConfigs()
	if err != nil {
		log.Errorln("Cannot load configs", err)
	}

	apis, err := api.New(&serviceConf)
	if err != nil {
		log.Fatalf("failed to connect to internal services: %v", err)
	}

_check_loader_loop:
	for {
		select {
		case <-time.After(10 * time.Second):
			err = checkLoaderState(ctx, apis)
			if err != nil {
				log.Error(err)
				log.Info("retry again ...")
			} else {
				break _check_loader_loop
			}
		case <-ctx.Done():
			return
		}
	}

	producer := kafka.NewProducer(kafkaConf)
	if producer == nil {
		log.Errorln("create kafka producer failed, pls check your config", kafkaConf)
		return
	}
	defer producer.Close()

	var rec = recorder.NewEventRecorder(ctx, producer.AsyncProducer)
	if rec == nil {
		log.Fatal("NewEngineServer failed as new recorder failed")
	}
	rstr := engine.NewEngineRestorer(apis)
	server := pkg.NewEngineServer(ctx, codes, rec, rstr)
	if server == nil {
		log.Errorln("create engine server failed")
		return
	}
	log.Infof("Talking to kafka cluster at %v", kafkaConf.Brokers)
	consumer := kafka.NewConsumer(kafkaConf, consumerGroup, server.GetTopics(), nil, nil)
	err = rstr.ResetOffset(consumer.Consumer)
	if err != nil {
		log.Fatalf("failed to reset offsets: %v", err)
	}

	defer consumer.Close()
	apis, rstr = nil, nil // release after using

	kafka.HandleMessages(ctx, consumer, server, kafka.NewDefaultSaver(configFileName, nil))
}

func checkLoaderState(ctx context.Context, apis *api.Server) error {
	resp, err := apis.Loader.GetLoaderState(ctx, &pb.GetLoaderStateRequest{})
	if err != nil {
		return fmt.Errorf("Error: loader cannot reach")
	}
	switch resp.State {
	case pb.FailLoaderState_FLS_IDLE:
		break
	case pb.FailLoaderState_FLS_NONE:
		return fmt.Errorf("Error: loader is in invalid state")
	case pb.FailLoaderState_FLS_BUSY:
		return fmt.Errorf("Error: loader is busy")
	default:
		return fmt.Errorf("Error: loader is in unknown state")
	}
	return nil
}
