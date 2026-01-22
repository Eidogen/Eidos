module github.com/eidos-exchange/eidos/tests/integration

go 1.25.6

require (
	github.com/IBM/sarama v1.46.3
	github.com/eidos-exchange/eidos/proto v0.0.0
	github.com/google/uuid v1.6.0
	github.com/redis/go-redis/v9 v9.17.2
	github.com/shopspring/decimal v1.4.0
	github.com/stretchr/testify v1.11.1
	google.golang.org/grpc v1.68.0
)

replace github.com/eidos-exchange/eidos/proto => ../../proto
