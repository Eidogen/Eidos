package kafka_test

import (
	"context"
	"fmt"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/kafka"
)

// ExampleProducer 生产者使用示例
func ExampleProducer() {
	// 创建生产者配置
	cfg := kafka.DefaultProducerConfig()
	cfg.Brokers = []string{"localhost:9092"}
	cfg.ClientID = "example-producer"
	cfg.Idempotent = true // 启用幂等生产者

	// 创建生产者
	producer, err := kafka.NewProducer(cfg)
	if err != nil {
		panic(err)
	}
	defer producer.Close()

	// 同步发送单条消息
	msg := kafka.NewMessageBuilder().
		WithTopic("example-topic").
		WithKey("user-123").
		WithValue(map[string]interface{}{
			"action": "login",
			"time":   time.Now().Unix(),
		}).
		WithTraceID("trace-123").
		Build()

	result, err := producer.Send(context.Background(), msg)
	if err != nil {
		fmt.Printf("send failed: %v\n", err)
		return
	}
	fmt.Printf("sent to partition %d offset %d\n", result.Partition, result.Offset)

	// 批量发送
	messages := make([]*kafka.Message, 0, 10)
	for i := 0; i < 10; i++ {
		messages = append(messages, kafka.NewMessageBuilder().
			WithTopic("example-topic").
			WithKey(fmt.Sprintf("key-%d", i)).
			WithValue(map[string]int{"id": i}).
			Build())
	}

	results, err := producer.SendBatch(context.Background(), messages)
	if err != nil {
		fmt.Printf("batch send failed: %v\n", err)
		return
	}

	for _, r := range results {
		if r.Err != nil {
			fmt.Printf("message failed: %v\n", r.Err)
		}
	}

	// 带重试的发送
	_, err = producer.SendWithRetry(context.Background(), msg, 3)
	if err != nil {
		fmt.Printf("send with retry failed: %v\n", err)
	}
}

// ExampleConsumer 消费者使用示例
func ExampleConsumer() {
	// 创建消费者配置
	cfg := kafka.DefaultConsumerConfig()
	cfg.Brokers = []string{"localhost:9092"}
	cfg.GroupID = "example-group"
	cfg.Topics = []string{"example-topic"}
	cfg.Concurrency = 3           // 3 个并发消费者
	cfg.AutoCommit.Enable = false // 禁用自动提交，使用手动提交
	cfg.RetryTopic = "example-topic-retry"
	cfg.DeadLetterTopic = "example-topic-dlq"
	cfg.MaxRetries = 3

	// 创建消费者
	consumer, err := kafka.NewConsumer(cfg)
	if err != nil {
		panic(err)
	}

	// 设置消息处理函数
	consumer.SetHandler(func(ctx context.Context, msg *kafka.Message) error {
		fmt.Printf("received message: topic=%s partition=%d offset=%d key=%s\n",
			msg.Topic, msg.Partition, msg.Offset, string(msg.Key))

		// 处理消息逻辑
		// ...

		// 返回 nil 表示处理成功，offset 会被标记为已消费
		// 返回 error 会触发重试逻辑
		return nil
	})

	// 如果需要重试功能，设置重试生产者
	producerCfg := kafka.DefaultProducerConfig()
	producerCfg.Brokers = cfg.Brokers
	retryProducer, _ := kafka.NewProducer(producerCfg)
	consumer.SetRetryProducer(retryProducer)

	// 启动消费者
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := consumer.Start(ctx); err != nil {
		panic(err)
	}

	// 等待信号优雅关闭
	// <-sigChan

	// 关闭消费者
	consumer.Close()
	retryProducer.Close()
}

// ExampleManager 管理器使用示例
func ExampleManager() {
	// 创建管理器
	manager := kafka.NewManager()
	defer manager.Close()

	// 注册生产者
	producerCfg := kafka.DefaultProducerConfig()
	producerCfg.Brokers = []string{"localhost:9092"}
	producerCfg.ClientID = "order-service"

	producer, err := manager.RegisterProducer("order-producer", producerCfg)
	if err != nil {
		panic(err)
	}

	// 注册消费者
	consumerCfg := kafka.DefaultConsumerConfig()
	consumerCfg.Brokers = []string{"localhost:9092"}
	consumerCfg.GroupID = "order-service-group"
	consumerCfg.Topics = []string{"orders"}

	consumer, err := manager.RegisterConsumer("order-consumer", consumerCfg)
	if err != nil {
		panic(err)
	}

	// 设置处理函数
	handler := func(ctx context.Context, msg *kafka.Message) error {
		fmt.Printf("processing order: %s\n", string(msg.Value))
		return nil
	}
	consumer.SetHandler(handler)

	// 启动消费者
	ctx := context.Background()
	if err := manager.StartConsumer(ctx, "order-consumer", handler); err != nil {
		panic(err)
	}

	// 使用生产者发送消息
	msg := kafka.NewMessageBuilder().
		WithTopic("orders").
		WithKey("order-123").
		WithValue(map[string]interface{}{"id": "order-123", "amount": 100}).
		Build()

	_, err = producer.Send(ctx, msg)
	if err != nil {
		fmt.Printf("send failed: %v\n", err)
	}

	// 健康检查
	health := manager.HealthCheck()
	fmt.Printf("healthy: %v\n", health.Healthy)

	// 获取指标
	producerMetrics := manager.ProducerMetrics()
	consumerMetrics := manager.ConsumerMetrics()
	fmt.Printf("producer metrics: %+v\n", producerMetrics)
	fmt.Printf("consumer metrics: %+v\n", consumerMetrics)
}

// ExampleSerializer 序列化器使用示例
func ExampleSerializer() {
	// JSON 序列化器 (默认)
	jsonSerializer := kafka.NewJSONSerializer()

	type Order struct {
		ID     string  `json:"id"`
		Amount float64 `json:"amount"`
	}

	order := &Order{ID: "123", Amount: 99.99}

	// 序列化
	data, err := jsonSerializer.Serialize(order)
	if err != nil {
		panic(err)
	}

	// 使用序列化后的数据构建消息
	msg := kafka.NewMessageBuilder().
		WithTopic("orders").
		WithKey(order.ID).
		WithValueBytes(data).
		WithSerializer(jsonSerializer).
		Build()

	fmt.Printf("message: %+v\n", msg)

	// Protobuf 序列化器
	pbSerializer := kafka.NewProtobufSerializer()

	// 对于 protobuf 消息，使用 pb 序列化器
	// pbData, _ := pbSerializer.Serialize(protoMessage)
	_ = pbSerializer
}

// ExampleMetricsCollector 指标收集示例
func ExampleMetricsCollector() {
	// 创建指标收集器
	collector := kafka.NewMetricsCollector("my-client", "my-group")

	// 记录生产者指标
	collector.RecordProducerSend("orders", 1024, true, nil, 0.005)
	collector.RecordProducerBatch("orders")
	collector.RecordProducerRetry("orders")

	// 记录消费者指标
	collector.RecordConsumerReceive("orders", 0, 512)
	collector.RecordConsumerProcess("orders", 0, true, nil, 0.01)
	collector.RecordConsumerCommit("orders")
	collector.RecordConsumerRebalance()
	collector.SetConsumerLag("orders", 0, 100)
	collector.SetConsumerOffset("orders", 0, 12345)
}
