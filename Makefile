run:
	go run cmd/app/* btcaud ethaud btceth

build:
	mkdir -p out && cd out &&\
	go build -o exchange.engine ../cmd/app/*

tidy:
	go fmt ./...
	go mod tidy

build-test:
	mkdir -p out && cd out &&\
	GOARCH=amd64 go build -o engine.test ../cmd/test/*