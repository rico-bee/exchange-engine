package api

import (
	log "github.com/sirupsen/logrus"
	pb "gitlab.com/sdce/protogo"
	"google.golang.org/grpc"
)

func newTradingClient(tradingURL string) (pb.TradingClient, error) {
	// Set up a connection to the server.
	log.Infoln("Trading grpc host:" + tradingURL)
	conn, err := grpc.Dial(tradingURL, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	return pb.NewTradingClient(conn), nil
}

func newFailLoaderClient(loaderURL string) (pb.FailLoaderClient, error) {
	// Set up a connection to the server.
	log.Infoln("Fail loader grpc host:" + loaderURL)
	conn, err := grpc.Dial(loaderURL, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	return pb.NewFailLoaderClient(conn), nil
}
