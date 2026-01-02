module github.com/eidos-exchange/eidos/eidos-api

go 1.22.0

require (
	github.com/eidos-exchange/eidos/eidos-common v0.0.0
	github.com/gin-gonic/gin v1.10.0
	github.com/gorilla/websocket v1.5.3
	google.golang.org/grpc v1.68.0
	google.golang.org/protobuf v1.35.2
)

replace github.com/eidos-exchange/eidos/eidos-common => ../eidos-common
