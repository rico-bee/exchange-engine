package api

import (
	"gitlab.com/sdce/exlib/service"
	pb "gitlab.com/sdce/protogo"
)

//Server serving for rpc api
type Server struct {
	Trading pb.TradingClient
	Loader  pb.FailLoaderClient
}

//New api server
func New(config *service.Config) (*Server, error) {
	trading, err := newTradingClient(config.Trading)
	if err != nil {
		return nil, err
	}
	loader, err := newFailLoaderClient(config.FailLoader)
	if err != nil {
		return nil, err
	}
	return &Server{
		Trading: trading,
		Loader:  loader,
	}, nil
}
