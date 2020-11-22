# build stage
FROM golang:1.12-alpine AS build_base

RUN apk add bash ca-certificates git gcc g++ libc-dev
WORKDIR /go/src/gitlab.com/sdce/exchange/engine

# Force the go compiler to use modules
ENV GO111MODULE=on
 
# We want to populate the module cache based on the go.{mod,sum} files.
COPY go.mod .
COPY go.sum .
#This is the ‘magic’ step that will download all the dependencies that are specified in 
# the go.mod and go.sum file.
# Because of how the layer caching system works in Docker, the  go mod download 
# command will _ only_ be re-run when the go.mod or go.sum file change 
# (or when we add another docker instruction this line)
# RUN git config --global user.name "builder_sdce" 
RUN git config --global credential.helper store
RUN echo "https://builder_sdce:getSome6@gitlab.com" > ~/.git-credentials
RUN go mod download

# This image builds the weavaite server
FROM build_base AS server_builder
# Here we copy the rest of the source code
COPY . .
# And compile the project
ADD . /src
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o exchange.engine /src/cmd/app/*.go
# final stage
FROM alpine
WORKDIR /app
COPY --from=server_builder /go/src/gitlab.com/sdce/exchange/engine/exchange.engine /app/
ENTRYPOINT ./exchange.engine
