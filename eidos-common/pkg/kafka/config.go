// Package kafka 提供 Kafka 生产者和消费者的封装实现
// 支持幂等生产、批量发送、消费者组管理、手动 offset 提交等企业级特性
package kafka

import (
	"time"
)

// Config Kafka 基础配置
type Config struct {
	// Brokers Kafka broker 地址列表
	Brokers []string `yaml:"brokers" json:"brokers"`
	// ClientID 客户端标识
	ClientID string `yaml:"client_id" json:"client_id"`
	// Version Kafka 版本 (如 "2.8.0")
	Version string `yaml:"version" json:"version"`

	// SASL 认证配置
	SASL *SASLConfig `yaml:"sasl" json:"sasl"`
	// TLS 配置
	TLS *TLSConfig `yaml:"tls" json:"tls"`
}

// SASLConfig SASL 认证配置
type SASLConfig struct {
	// Enable 是否启用 SASL
	Enable bool `yaml:"enable" json:"enable"`
	// Mechanism 认证机制: PLAIN, SCRAM-SHA-256, SCRAM-SHA-512
	Mechanism string `yaml:"mechanism" json:"mechanism"`
	// Username 用户名
	Username string `yaml:"username" json:"username"`
	// Password 密码
	Password string `yaml:"password" json:"password"`
}

// TLSConfig TLS 配置
type TLSConfig struct {
	// Enable 是否启用 TLS
	Enable bool `yaml:"enable" json:"enable"`
	// CertFile 客户端证书文件
	CertFile string `yaml:"cert_file" json:"cert_file"`
	// KeyFile 客户端私钥文件
	KeyFile string `yaml:"key_file" json:"key_file"`
	// CAFile CA 证书文件
	CAFile string `yaml:"ca_file" json:"ca_file"`
	// InsecureSkipVerify 是否跳过证书验证
	InsecureSkipVerify bool `yaml:"insecure_skip_verify" json:"insecure_skip_verify"`
}

// ProducerConfig 生产者配置
type ProducerConfig struct {
	Config `yaml:",inline"`

	// Idempotent 是否启用幂等生产者 (保证精确一次语义)
	Idempotent bool `yaml:"idempotent" json:"idempotent"`

	// RequiredAcks 确认级别: 0=不等待, 1=Leader确认, -1=所有ISR确认
	RequiredAcks int `yaml:"required_acks" json:"required_acks"`

	// Batch 批量配置
	Batch BatchConfig `yaml:"batch" json:"batch"`

	// Retry 重试配置
	Retry RetryConfig `yaml:"retry" json:"retry"`

	// Compression 压缩算法: none, gzip, snappy, lz4, zstd
	Compression string `yaml:"compression" json:"compression"`

	// MaxMessageBytes 最大消息大小
	MaxMessageBytes int `yaml:"max_message_bytes" json:"max_message_bytes"`

	// Timeout 发送超时
	Timeout time.Duration `yaml:"timeout" json:"timeout"`

	// ChannelBufferSize 内部 channel 缓冲大小
	ChannelBufferSize int `yaml:"channel_buffer_size" json:"channel_buffer_size"`
}

// BatchConfig 批量发送配置
type BatchConfig struct {
	// Size 批量大小 (消息数)
	Size int `yaml:"size" json:"size"`
	// Bytes 批量大小 (字节数)
	Bytes int `yaml:"bytes" json:"bytes"`
	// Timeout 批量超时 (达到超时即使批量未满也会发送)
	Timeout time.Duration `yaml:"timeout" json:"timeout"`
}

// RetryConfig 重试配置
type RetryConfig struct {
	// Max 最大重试次数
	Max int `yaml:"max" json:"max"`
	// Backoff 重试间隔
	Backoff time.Duration `yaml:"backoff" json:"backoff"`
	// MaxBackoff 最大重试间隔
	MaxBackoff time.Duration `yaml:"max_backoff" json:"max_backoff"`
}

// ConsumerConfig 消费者配置
type ConsumerConfig struct {
	Config `yaml:",inline"`

	// GroupID 消费者组 ID
	GroupID string `yaml:"group_id" json:"group_id"`

	// Topics 订阅的主题列表
	Topics []string `yaml:"topics" json:"topics"`

	// InitialOffset 初始 offset: newest (最新), oldest (最旧)
	InitialOffset string `yaml:"initial_offset" json:"initial_offset"`

	// AutoCommit 自动提交配置 (建议禁用，使用手动提交)
	AutoCommit AutoCommitConfig `yaml:"auto_commit" json:"auto_commit"`

	// Session 会话配置
	Session SessionConfig `yaml:"session" json:"session"`

	// FetchConfig 拉取配置
	Fetch FetchConfig `yaml:"fetch" json:"fetch"`

	// Concurrency 并发消费者数量
	Concurrency int `yaml:"concurrency" json:"concurrency"`

	// MaxProcessingTime 单条消息最大处理时间
	MaxProcessingTime time.Duration `yaml:"max_processing_time" json:"max_processing_time"`

	// RetryTopic 重试主题 (处理失败的消息会发送到此主题)
	RetryTopic string `yaml:"retry_topic" json:"retry_topic"`

	// DeadLetterTopic 死信主题 (重试多次仍失败的消息)
	DeadLetterTopic string `yaml:"dead_letter_topic" json:"dead_letter_topic"`

	// MaxRetries 最大重试次数
	MaxRetries int `yaml:"max_retries" json:"max_retries"`
}

// AutoCommitConfig 自动提交配置
type AutoCommitConfig struct {
	// Enable 是否启用自动提交 (建议禁用)
	Enable bool `yaml:"enable" json:"enable"`
	// Interval 自动提交间隔
	Interval time.Duration `yaml:"interval" json:"interval"`
}

// SessionConfig 会话配置
type SessionConfig struct {
	// Timeout 会话超时 (超时未心跳会触发重平衡)
	Timeout time.Duration `yaml:"timeout" json:"timeout"`
	// HeartbeatInterval 心跳间隔
	HeartbeatInterval time.Duration `yaml:"heartbeat_interval" json:"heartbeat_interval"`
}

// FetchConfig 拉取配置
type FetchConfig struct {
	// MinBytes 最小拉取字节数
	MinBytes int `yaml:"min_bytes" json:"min_bytes"`
	// MaxBytes 最大拉取字节数
	MaxBytes int `yaml:"max_bytes" json:"max_bytes"`
	// MaxWaitTime 最大等待时间
	MaxWaitTime time.Duration `yaml:"max_wait_time" json:"max_wait_time"`
}

// DefaultProducerConfig 默认生产者配置
func DefaultProducerConfig() *ProducerConfig {
	return &ProducerConfig{
		Config: Config{
			Version: "2.8.0",
		},
		Idempotent:   true,
		RequiredAcks: -1, // 所有 ISR 确认
		Batch: BatchConfig{
			Size:    100,
			Bytes:   1048576, // 1MB
			Timeout: 10 * time.Millisecond,
		},
		Retry: RetryConfig{
			Max:        3,
			Backoff:    100 * time.Millisecond,
			MaxBackoff: 1 * time.Second,
		},
		Compression:       "snappy",
		MaxMessageBytes:   1048576, // 1MB
		Timeout:           10 * time.Second,
		ChannelBufferSize: 256,
	}
}

// DefaultConsumerConfig 默认消费者配置
func DefaultConsumerConfig() *ConsumerConfig {
	return &ConsumerConfig{
		Config: Config{
			Version: "2.8.0",
		},
		InitialOffset: "newest",
		AutoCommit: AutoCommitConfig{
			Enable:   false, // 默认禁用自动提交
			Interval: 1 * time.Second,
		},
		Session: SessionConfig{
			Timeout:           30 * time.Second,
			HeartbeatInterval: 3 * time.Second,
		},
		Fetch: FetchConfig{
			MinBytes:    1,
			MaxBytes:    10485760, // 10MB
			MaxWaitTime: 500 * time.Millisecond,
		},
		Concurrency:       1,
		MaxProcessingTime: 30 * time.Second,
		MaxRetries:        3,
	}
}
