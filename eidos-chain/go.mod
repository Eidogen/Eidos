module github.com/eidos-exchange/eidos/eidos-chain

go 1.22.0

require (
	github.com/eidos-exchange/eidos/eidos-common v0.0.0
	github.com/ethereum/go-ethereum v1.14.12
	github.com/go-redis/redis/v8 v8.11.5
	github.com/google/uuid v1.6.0
	github.com/segmentio/kafka-go v0.4.47
	github.com/shopspring/decimal v1.4.0
	github.com/stretchr/testify v1.9.0
	go.uber.org/zap v1.27.0
	google.golang.org/grpc v1.68.0
	google.golang.org/protobuf v1.35.2
	gopkg.in/yaml.v3 v3.0.1
	gorm.io/driver/postgres v1.5.9
	gorm.io/gorm v1.25.12
)

require (
	github.com/DATA-DOG/go-sqlmock v1.5.2
	github.com/alicebob/miniredis/v2 v2.33.0
)

replace github.com/eidos-exchange/eidos/eidos-common => ../eidos-common
