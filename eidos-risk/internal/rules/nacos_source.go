// Package rules provides Nacos-based rule source
package rules

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/config_client"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"go.uber.org/zap"
)

// NacosRuleSource loads rules from Nacos configuration center
type NacosRuleSource struct {
	mu sync.RWMutex

	client    config_client.IConfigClient
	dataID    string
	group     string
	namespace string

	lastConfig string
	enabled    bool
}

// NacosSourceConfig represents Nacos source configuration
type NacosSourceConfig struct {
	ServerAddr string
	Namespace  string
	Group      string
	DataID     string
	Username   string
	Password   string
	LogDir     string
	CacheDir   string
}

// NewNacosRuleSource creates a new Nacos rule source
func NewNacosRuleSource(cfg *NacosSourceConfig) (*NacosRuleSource, error) {
	// Parse server address
	serverConfigs := []constant.ServerConfig{
		{
			IpAddr: cfg.ServerAddr,
			Port:   8848,
		},
	}

	clientConfig := constant.ClientConfig{
		NamespaceId:         cfg.Namespace,
		TimeoutMs:           5000,
		NotLoadCacheAtStart: true,
		LogDir:              cfg.LogDir,
		CacheDir:            cfg.CacheDir,
		Username:            cfg.Username,
		Password:            cfg.Password,
	}

	client, err := clients.NewConfigClient(
		vo.NacosClientParam{
			ClientConfig:  &clientConfig,
			ServerConfigs: serverConfigs,
		},
	)
	if err != nil {
		return nil, err
	}

	dataID := cfg.DataID
	if dataID == "" {
		dataID = "risk-rules.json"
	}

	group := cfg.Group
	if group == "" {
		group = "EIDOS_GROUP"
	}

	return &NacosRuleSource{
		client:    client,
		dataID:    dataID,
		group:     group,
		namespace: cfg.Namespace,
		enabled:   true,
	}, nil
}

// Name returns the source name
func (s *NacosRuleSource) Name() string {
	return "nacos"
}

// Load loads rules from Nacos
func (s *NacosRuleSource) Load(ctx context.Context) ([]*RuleConfig, error) {
	if !s.enabled {
		return nil, nil
	}

	content, err := s.client.GetConfig(vo.ConfigParam{
		DataId: s.dataID,
		Group:  s.group,
	})
	if err != nil {
		logger.Error("failed to get config from nacos",
			zap.String("data_id", s.dataID),
			zap.String("group", s.group),
			zap.Error(err))
		return nil, err
	}

	if content == "" {
		return nil, nil
	}

	s.mu.Lock()
	s.lastConfig = content
	s.mu.Unlock()

	return s.parseConfig(content)
}

// Watch watches for rule changes in Nacos
func (s *NacosRuleSource) Watch(ctx context.Context, updates chan<- []*RuleConfig) error {
	if !s.enabled {
		return nil
	}

	err := s.client.ListenConfig(vo.ConfigParam{
		DataId: s.dataID,
		Group:  s.group,
		OnChange: func(namespace, group, dataId, data string) {
			logger.Info("nacos config changed",
				zap.String("data_id", dataId),
				zap.String("group", group))

			s.mu.Lock()
			s.lastConfig = data
			s.mu.Unlock()

			rules, err := s.parseConfig(data)
			if err != nil {
				logger.Error("failed to parse nacos config", zap.Error(err))
				return
			}

			select {
			case updates <- rules:
			default:
				logger.Warn("rule update channel full, skipping nacos update")
			}
		},
	})

	if err != nil {
		return err
	}

	// Wait for context cancellation
	<-ctx.Done()

	// Cancel the listener
	s.client.CancelListenConfig(vo.ConfigParam{
		DataId: s.dataID,
		Group:  s.group,
	})

	return ctx.Err()
}

// parseConfig parses the Nacos configuration content
func (s *NacosRuleSource) parseConfig(content string) ([]*RuleConfig, error) {
	if content == "" {
		return nil, nil
	}

	var nacosConfig NacosRulesConfig
	if err := json.Unmarshal([]byte(content), &nacosConfig); err != nil {
		return nil, err
	}

	return nacosConfig.Rules, nil
}

// GetLastConfig returns the last loaded config
func (s *NacosRuleSource) GetLastConfig() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastConfig
}

// NacosRulesConfig represents the Nacos configuration format
type NacosRulesConfig struct {
	Version   string        `json:"version"`
	UpdatedAt int64         `json:"updated_at"`
	Rules     []*RuleConfig `json:"rules"`
}

// NacosMockSource provides a mock Nacos source for testing
type NacosMockSource struct {
	rules []*RuleConfig
}

// NewNacosMockSource creates a mock Nacos source
func NewNacosMockSource(rules []*RuleConfig) *NacosMockSource {
	return &NacosMockSource{rules: rules}
}

// Name returns the source name
func (s *NacosMockSource) Name() string {
	return "nacos-mock"
}

// Load loads rules from the mock
func (s *NacosMockSource) Load(ctx context.Context) ([]*RuleConfig, error) {
	return s.rules, nil
}

// Watch does nothing for the mock
func (s *NacosMockSource) Watch(ctx context.Context, updates chan<- []*RuleConfig) error {
	<-ctx.Done()
	return ctx.Err()
}

// UpdateRules updates the mock rules and sends notification
func (s *NacosMockSource) UpdateRules(rules []*RuleConfig, updates chan<- []*RuleConfig) {
	s.rules = rules
	select {
	case updates <- rules:
	default:
	}
}
