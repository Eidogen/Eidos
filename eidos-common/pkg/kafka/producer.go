package kafka

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IBM/sarama"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
)

// Producer Kafka 生产者
type Producer struct {
	config       *ProducerConfig
	syncProducer sarama.SyncProducer
	asyncProducer sarama.AsyncProducer
	client        sarama.Client

	// 批量发送相关
	batchMu       sync.Mutex
	batchMessages []*Message
	batchTimer    *time.Timer
	batchCh       chan *Message

	// 状态
	closed   int32
	closeCh  chan struct{}
	closeWg  sync.WaitGroup

	// 指标
	metrics *ProducerMetrics
}

// ProducerMetrics 生产者指标
type ProducerMetrics struct {
	MessagesSent      int64
	MessagesSucceeded int64
	MessagesFailed    int64
	BytesSent         int64
	BatchesSent       int64
	RetryCount        int64
	LastSendTime      time.Time
}

// SendResult 发送结果
type SendResult struct {
	Topic     string
	Partition int32
	Offset    int64
	Err       error
}

// NewProducer 创建生产者
func NewProducer(cfg *ProducerConfig) (*Producer, error) {
	if cfg == nil {
		cfg = DefaultProducerConfig()
	}

	if len(cfg.Brokers) == 0 {
		return nil, errors.New("kafka brokers is required")
	}

	saramaConfig, err := buildSaramaProducerConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build sarama config failed: %w", err)
	}

	client, err := sarama.NewClient(cfg.Brokers, saramaConfig)
	if err != nil {
		return nil, fmt.Errorf("create kafka client failed: %w", err)
	}

	syncProducer, err := sarama.NewSyncProducerFromClient(client)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("create sync producer failed: %w", err)
	}

	asyncProducer, err := sarama.NewAsyncProducerFromClient(client)
	if err != nil {
		syncProducer.Close()
		client.Close()
		return nil, fmt.Errorf("create async producer failed: %w", err)
	}

	p := &Producer{
		config:        cfg,
		client:        client,
		syncProducer:  syncProducer,
		asyncProducer: asyncProducer,
		batchMessages: make([]*Message, 0, cfg.Batch.Size),
		batchCh:       make(chan *Message, cfg.ChannelBufferSize),
		closeCh:       make(chan struct{}),
		metrics:       &ProducerMetrics{},
	}

	// 启动批量发送协程
	p.closeWg.Add(2)
	go p.batchLoop()
	go p.asyncResultLoop()

	logger.Info("kafka producer created",
		"brokers", cfg.Brokers,
		"client_id", cfg.ClientID,
		"idempotent", cfg.Idempotent,
	)

	return p, nil
}

// buildSaramaProducerConfig 构建 Sarama 生产者配置
func buildSaramaProducerConfig(cfg *ProducerConfig) (*sarama.Config, error) {
	saramaConfig := sarama.NewConfig()

	// 版本
	if cfg.Version != "" {
		version, err := sarama.ParseKafkaVersion(cfg.Version)
		if err != nil {
			return nil, fmt.Errorf("parse kafka version failed: %w", err)
		}
		saramaConfig.Version = version
	}

	// 客户端 ID
	if cfg.ClientID != "" {
		saramaConfig.ClientID = cfg.ClientID
	}

	// 幂等配置 (保证精确一次语义)
	if cfg.Idempotent {
		saramaConfig.Producer.Idempotent = true
		saramaConfig.Producer.RequiredAcks = sarama.WaitForAll
		saramaConfig.Net.MaxOpenRequests = 1 // 幂等需要
	} else {
		switch cfg.RequiredAcks {
		case 0:
			saramaConfig.Producer.RequiredAcks = sarama.NoResponse
		case 1:
			saramaConfig.Producer.RequiredAcks = sarama.WaitForLocal
		default:
			saramaConfig.Producer.RequiredAcks = sarama.WaitForAll
		}
	}

	// 批量配置
	saramaConfig.Producer.Flush.Messages = cfg.Batch.Size
	saramaConfig.Producer.Flush.Bytes = cfg.Batch.Bytes
	saramaConfig.Producer.Flush.Frequency = cfg.Batch.Timeout

	// 重试配置
	saramaConfig.Producer.Retry.Max = cfg.Retry.Max
	saramaConfig.Producer.Retry.Backoff = cfg.Retry.Backoff

	// 压缩
	switch cfg.Compression {
	case "gzip":
		saramaConfig.Producer.Compression = sarama.CompressionGZIP
	case "snappy":
		saramaConfig.Producer.Compression = sarama.CompressionSnappy
	case "lz4":
		saramaConfig.Producer.Compression = sarama.CompressionLZ4
	case "zstd":
		saramaConfig.Producer.Compression = sarama.CompressionZSTD
	default:
		saramaConfig.Producer.Compression = sarama.CompressionNone
	}

	// 其他配置
	saramaConfig.Producer.MaxMessageBytes = cfg.MaxMessageBytes
	saramaConfig.Producer.Timeout = cfg.Timeout
	saramaConfig.ChannelBufferSize = cfg.ChannelBufferSize

	// 同步生产者需要返回结果
	saramaConfig.Producer.Return.Successes = true
	saramaConfig.Producer.Return.Errors = true

	// SASL 配置
	if cfg.SASL != nil && cfg.SASL.Enable {
		saramaConfig.Net.SASL.Enable = true
		saramaConfig.Net.SASL.User = cfg.SASL.Username
		saramaConfig.Net.SASL.Password = cfg.SASL.Password

		switch cfg.SASL.Mechanism {
		case "SCRAM-SHA-256":
			saramaConfig.Net.SASL.SCRAMClientGeneratorFunc = func() sarama.SCRAMClient {
				return &xdgScramClient{HashGeneratorFcn: SHA256}
			}
			saramaConfig.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA256
		case "SCRAM-SHA-512":
			saramaConfig.Net.SASL.SCRAMClientGeneratorFunc = func() sarama.SCRAMClient {
				return &xdgScramClient{HashGeneratorFcn: SHA512}
			}
			saramaConfig.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA512
		default:
			saramaConfig.Net.SASL.Mechanism = sarama.SASLTypePlaintext
		}
	}

	// TLS 配置
	if cfg.TLS != nil && cfg.TLS.Enable {
		tlsConfig, err := buildTLSConfig(cfg.TLS)
		if err != nil {
			return nil, fmt.Errorf("build tls config failed: %w", err)
		}
		saramaConfig.Net.TLS.Enable = true
		saramaConfig.Net.TLS.Config = tlsConfig
	}

	return saramaConfig, nil
}

// buildTLSConfig 构建 TLS 配置
func buildTLSConfig(cfg *TLSConfig) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: cfg.InsecureSkipVerify,
	}

	if cfg.CertFile != "" && cfg.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load cert pair failed: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	if cfg.CAFile != "" {
		caCert, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read ca file failed: %w", err)
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		tlsConfig.RootCAs = caCertPool
	}

	return tlsConfig, nil
}

// Send 同步发送单条消息
func (p *Producer) Send(ctx context.Context, msg *Message) (*SendResult, error) {
	if atomic.LoadInt32(&p.closed) == 1 {
		return nil, errors.New("producer is closed")
	}

	if msg == nil {
		return nil, errors.New("message is nil")
	}

	if msg.Topic == "" {
		return nil, errors.New("message topic is required")
	}

	producerMsg := p.buildProducerMessage(msg)

	partition, offset, err := p.syncProducer.SendMessage(producerMsg)

	atomic.AddInt64(&p.metrics.MessagesSent, 1)

	if err != nil {
		atomic.AddInt64(&p.metrics.MessagesFailed, 1)
		logger.Error("send message failed",
			"topic", msg.Topic,
			"error", err,
		)
		return &SendResult{
			Topic:     msg.Topic,
			Partition: partition,
			Offset:    offset,
			Err:       err,
		}, err
	}

	atomic.AddInt64(&p.metrics.MessagesSucceeded, 1)
	atomic.AddInt64(&p.metrics.BytesSent, int64(len(msg.Value)))
	p.metrics.LastSendTime = time.Now()

	logger.Debug("message sent",
		"topic", msg.Topic,
		"partition", partition,
		"offset", offset,
	)

	return &SendResult{
		Topic:     msg.Topic,
		Partition: partition,
		Offset:    offset,
	}, nil
}

// SendAsync 异步发送消息
func (p *Producer) SendAsync(msg *Message) error {
	if atomic.LoadInt32(&p.closed) == 1 {
		return errors.New("producer is closed")
	}

	if msg == nil {
		return errors.New("message is nil")
	}

	if msg.Topic == "" {
		return errors.New("message topic is required")
	}

	producerMsg := p.buildProducerMessage(msg)
	p.asyncProducer.Input() <- producerMsg

	atomic.AddInt64(&p.metrics.MessagesSent, 1)

	return nil
}

// SendBatch 同步批量发送
func (p *Producer) SendBatch(ctx context.Context, messages []*Message) ([]*SendResult, error) {
	if atomic.LoadInt32(&p.closed) == 1 {
		return nil, errors.New("producer is closed")
	}

	if len(messages) == 0 {
		return nil, nil
	}

	results := make([]*SendResult, len(messages))
	var wg sync.WaitGroup
	var mu sync.Mutex

	// 并发发送每条消息
	for i, msg := range messages {
		wg.Add(1)
		go func(idx int, m *Message) {
			defer wg.Done()
			result, err := p.Send(ctx, m)
			if err != nil {
				result = &SendResult{
					Topic: m.Topic,
					Err:   err,
				}
			}
			mu.Lock()
			results[idx] = result
			mu.Unlock()
		}(i, msg)
	}

	wg.Wait()
	atomic.AddInt64(&p.metrics.BatchesSent, 1)

	return results, nil
}

// SendBatchAsync 异步批量发送 (通过内部批量缓冲)
func (p *Producer) SendBatchAsync(msg *Message) error {
	if atomic.LoadInt32(&p.closed) == 1 {
		return errors.New("producer is closed")
	}

	select {
	case p.batchCh <- msg:
		return nil
	default:
		return errors.New("batch channel is full")
	}
}

// batchLoop 批量发送循环
func (p *Producer) batchLoop() {
	defer p.closeWg.Done()

	ticker := time.NewTicker(p.config.Batch.Timeout)
	defer ticker.Stop()

	for {
		select {
		case <-p.closeCh:
			// 关闭前发送剩余消息
			p.flushBatch()
			return
		case msg := <-p.batchCh:
			p.batchMu.Lock()
			p.batchMessages = append(p.batchMessages, msg)
			if len(p.batchMessages) >= p.config.Batch.Size {
				p.flushBatchLocked()
			}
			p.batchMu.Unlock()
		case <-ticker.C:
			p.flushBatch()
		}
	}
}

// flushBatch 刷新批量缓冲
func (p *Producer) flushBatch() {
	p.batchMu.Lock()
	defer p.batchMu.Unlock()
	p.flushBatchLocked()
}

// flushBatchLocked 刷新批量缓冲 (需要持有锁)
func (p *Producer) flushBatchLocked() {
	if len(p.batchMessages) == 0 {
		return
	}

	messages := p.batchMessages
	p.batchMessages = make([]*Message, 0, p.config.Batch.Size)

	// 异步发送
	go func(msgs []*Message) {
		for _, msg := range msgs {
			producerMsg := p.buildProducerMessage(msg)
			p.asyncProducer.Input() <- producerMsg
		}
		atomic.AddInt64(&p.metrics.BatchesSent, 1)
	}(messages)
}

// asyncResultLoop 异步结果处理循环
func (p *Producer) asyncResultLoop() {
	defer p.closeWg.Done()

	for {
		select {
		case <-p.closeCh:
			return
		case success := <-p.asyncProducer.Successes():
			if success != nil {
				atomic.AddInt64(&p.metrics.MessagesSucceeded, 1)
				atomic.AddInt64(&p.metrics.BytesSent, int64(success.Value.Length()))
				p.metrics.LastSendTime = time.Now()

				logger.Debug("async message sent",
					"topic", success.Topic,
					"partition", success.Partition,
					"offset", success.Offset,
				)
			}
		case err := <-p.asyncProducer.Errors():
			if err != nil {
				atomic.AddInt64(&p.metrics.MessagesFailed, 1)
				logger.Error("async message failed",
					"topic", err.Msg.Topic,
					"error", err.Err,
				)
			}
		}
	}
}

// buildProducerMessage 构建 Sarama 消息
func (p *Producer) buildProducerMessage(msg *Message) *sarama.ProducerMessage {
	producerMsg := &sarama.ProducerMessage{
		Topic:     msg.Topic,
		Timestamp: msg.Timestamp,
	}

	if msg.Key != nil {
		producerMsg.Key = sarama.ByteEncoder(msg.Key)
	}

	if msg.Value != nil {
		producerMsg.Value = sarama.ByteEncoder(msg.Value)
	}

	if msg.Partition >= 0 {
		producerMsg.Partition = msg.Partition
	}

	if len(msg.Headers) > 0 {
		headers := make([]sarama.RecordHeader, len(msg.Headers))
		for i, h := range msg.Headers {
			headers[i] = sarama.RecordHeader{
				Key:   []byte(h.Key),
				Value: h.Value,
			}
		}
		producerMsg.Headers = headers
	}

	return producerMsg
}

// SendWithRetry 带重试的发送
func (p *Producer) SendWithRetry(ctx context.Context, msg *Message, maxRetries int) (*SendResult, error) {
	var lastErr error
	backoff := p.config.Retry.Backoff

	for i := 0; i <= maxRetries; i++ {
		result, err := p.Send(ctx, msg)
		if err == nil {
			return result, nil
		}

		lastErr = err
		atomic.AddInt64(&p.metrics.RetryCount, 1)

		logger.Warn("send message failed, retrying",
			"topic", msg.Topic,
			"retry", i+1,
			"max_retries", maxRetries,
			"error", err,
		)

		// 指数退避
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
			backoff *= 2
			if backoff > p.config.Retry.MaxBackoff {
				backoff = p.config.Retry.MaxBackoff
			}
		}
	}

	return nil, fmt.Errorf("send message failed after %d retries: %w", maxRetries, lastErr)
}

// Metrics 获取指标
func (p *Producer) Metrics() *ProducerMetrics {
	return &ProducerMetrics{
		MessagesSent:      atomic.LoadInt64(&p.metrics.MessagesSent),
		MessagesSucceeded: atomic.LoadInt64(&p.metrics.MessagesSucceeded),
		MessagesFailed:    atomic.LoadInt64(&p.metrics.MessagesFailed),
		BytesSent:         atomic.LoadInt64(&p.metrics.BytesSent),
		BatchesSent:       atomic.LoadInt64(&p.metrics.BatchesSent),
		RetryCount:        atomic.LoadInt64(&p.metrics.RetryCount),
		LastSendTime:      p.metrics.LastSendTime,
	}
}

// Partitions 获取主题分区列表
func (p *Producer) Partitions(topic string) ([]int32, error) {
	return p.client.Partitions(topic)
}

// Close 关闭生产者
func (p *Producer) Close() error {
	if !atomic.CompareAndSwapInt32(&p.closed, 0, 1) {
		return nil
	}

	logger.Info("closing kafka producer")

	// 关闭信号
	close(p.closeCh)

	// 等待协程退出
	p.closeWg.Wait()

	// 关闭生产者
	var errs []error

	if err := p.asyncProducer.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close async producer: %w", err))
	}

	if err := p.syncProducer.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close sync producer: %w", err))
	}

	if err := p.client.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close client: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("close producer errors: %v", errs)
	}

	logger.Info("kafka producer closed",
		"messages_sent", p.metrics.MessagesSent,
		"messages_succeeded", p.metrics.MessagesSucceeded,
		"messages_failed", p.metrics.MessagesFailed,
	)

	return nil
}

// IsClosed 检查是否已关闭
func (p *Producer) IsClosed() bool {
	return atomic.LoadInt32(&p.closed) == 1
}
