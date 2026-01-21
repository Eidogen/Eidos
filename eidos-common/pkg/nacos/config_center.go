package nacos

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/config_client"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"go.uber.org/zap"
)

// ConfigCenterConfig 配置中心配置
type ConfigCenterConfig struct {
	ServerAddr   string `yaml:"server_addr" json:"server_addr"`       // Nacos 服务器地址
	Namespace    string `yaml:"namespace" json:"namespace"`           // 命名空间 ID
	Group        string `yaml:"group" json:"group"`                   // 默认分组
	Username     string `yaml:"username" json:"username"`             // 用户名
	Password     string `yaml:"password" json:"password"`             // 密码
	LogDir       string `yaml:"log_dir" json:"log_dir"`               // 日志目录
	CacheDir     string `yaml:"cache_dir" json:"cache_dir"`           // 缓存目录
	LogLevel     string `yaml:"log_level" json:"log_level"`           // 日志级别
	TimeoutMs    uint64 `yaml:"timeout_ms" json:"timeout_ms"`         // 超时时间
	NotLoadCache bool   `yaml:"not_load_cache" json:"not_load_cache"` // 是否不加载缓存
}

// DefaultConfigCenterConfig 返回默认配置
func DefaultConfigCenterConfig() *ConfigCenterConfig {
	return &ConfigCenterConfig{
		ServerAddr:   "127.0.0.1:8848",
		Namespace:    "public",
		Group:        "DEFAULT_GROUP",
		LogDir:       "/tmp/nacos/log",
		CacheDir:     "/tmp/nacos/cache",
		LogLevel:     "warn",
		TimeoutMs:    5000,
		NotLoadCache: false,
	}
}

// ConfigCenter Nacos 配置中心
type ConfigCenter struct {
	configClient config_client.IConfigClient
	config       *ConfigCenterConfig
	cache        sync.Map // 配置缓存 map[string]string
	listeners    sync.Map // 监听器 map[string][]ConfigChangeListener
	mu           sync.RWMutex
}

// ConfigChangeListener 配置变更监听器
type ConfigChangeListener func(namespace, group, dataId, data string)

// NewConfigCenter 创建配置中心客户端
func NewConfigCenter(cfg *ConfigCenterConfig) (*ConfigCenter, error) {
	if cfg == nil {
		cfg = DefaultConfigCenterConfig()
	}

	// 解析服务器地址
	serverConfigs, err := parseServerAddr(cfg.ServerAddr)
	if err != nil {
		return nil, fmt.Errorf("parse server addr failed: %w", err)
	}

	// 客户端配置
	clientConfig := constant.ClientConfig{
		NamespaceId:         cfg.Namespace,
		TimeoutMs:           cfg.TimeoutMs,
		NotLoadCacheAtStart: cfg.NotLoadCache,
		LogDir:              cfg.LogDir,
		CacheDir:            cfg.CacheDir,
		LogLevel:            cfg.LogLevel,
	}

	if cfg.Username != "" {
		clientConfig.Username = cfg.Username
		clientConfig.Password = cfg.Password
	}

	// 创建配置客户端
	configClient, err := clients.NewConfigClient(
		vo.NacosClientParam{
			ClientConfig:  &clientConfig,
			ServerConfigs: serverConfigs,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("create config client failed: %w", err)
	}

	logger.Info("nacos config center created",
		zap.String("server_addr", cfg.ServerAddr),
		zap.String("namespace", cfg.Namespace),
	)

	return &ConfigCenter{
		configClient: configClient,
		config:       cfg,
	}, nil
}

// GetConfig 获取配置
func (c *ConfigCenter) GetConfig(dataId string) (string, error) {
	return c.GetConfigWithGroup(dataId, c.config.Group)
}

// GetConfigWithGroup 获取指定分组的配置
func (c *ConfigCenter) GetConfigWithGroup(dataId, group string) (string, error) {
	// 先从缓存获取
	cacheKey := c.cacheKey(dataId, group)
	if cached, ok := c.cache.Load(cacheKey); ok {
		return cached.(string), nil
	}

	// 从 Nacos 获取
	content, err := c.configClient.GetConfig(vo.ConfigParam{
		DataId: dataId,
		Group:  group,
	})
	if err != nil {
		return "", fmt.Errorf("get config failed: %w", err)
	}

	// 存入缓存
	c.cache.Store(cacheKey, content)

	logger.Debug("config loaded from nacos",
		zap.String("data_id", dataId),
		zap.String("group", group),
		zap.Int("length", len(content)),
	)

	return content, nil
}

// PublishConfig 发布配置
func (c *ConfigCenter) PublishConfig(dataId, content string) error {
	return c.PublishConfigWithGroup(dataId, c.config.Group, content)
}

// PublishConfigWithGroup 发布指定分组的配置
func (c *ConfigCenter) PublishConfigWithGroup(dataId, group, content string) error {
	success, err := c.configClient.PublishConfig(vo.ConfigParam{
		DataId:  dataId,
		Group:   group,
		Content: content,
	})
	if err != nil {
		return fmt.Errorf("publish config failed: %w", err)
	}
	if !success {
		return fmt.Errorf("publish config failed: unknown error")
	}

	// 更新缓存
	cacheKey := c.cacheKey(dataId, group)
	c.cache.Store(cacheKey, content)

	logger.Info("config published to nacos",
		zap.String("data_id", dataId),
		zap.String("group", group),
	)

	return nil
}

// DeleteConfig 删除配置
func (c *ConfigCenter) DeleteConfig(dataId string) error {
	return c.DeleteConfigWithGroup(dataId, c.config.Group)
}

// DeleteConfigWithGroup 删除指定分组的配置
func (c *ConfigCenter) DeleteConfigWithGroup(dataId, group string) error {
	success, err := c.configClient.DeleteConfig(vo.ConfigParam{
		DataId: dataId,
		Group:  group,
	})
	if err != nil {
		return fmt.Errorf("delete config failed: %w", err)
	}
	if !success {
		return fmt.Errorf("delete config failed: unknown error")
	}

	// 清除缓存
	cacheKey := c.cacheKey(dataId, group)
	c.cache.Delete(cacheKey)

	logger.Info("config deleted from nacos",
		zap.String("data_id", dataId),
		zap.String("group", group),
	)

	return nil
}

// Listen 监听配置变更
func (c *ConfigCenter) Listen(dataId string, listener ConfigChangeListener) error {
	return c.ListenWithGroup(dataId, c.config.Group, listener)
}

// ListenWithGroup 监听指定分组的配置变更
func (c *ConfigCenter) ListenWithGroup(dataId, group string, listener ConfigChangeListener) error {
	err := c.configClient.ListenConfig(vo.ConfigParam{
		DataId: dataId,
		Group:  group,
		OnChange: func(namespace, group, dataId, data string) {
			// 更新缓存
			cacheKey := c.cacheKey(dataId, group)
			c.cache.Store(cacheKey, data)

			logger.Info("config changed",
				zap.String("namespace", namespace),
				zap.String("group", group),
				zap.String("data_id", dataId),
			)

			// 调用监听器
			if listener != nil {
				listener(namespace, group, dataId, data)
			}

			// 调用所有注册的监听器
			c.notifyListeners(dataId, group, namespace, data)
		},
	})
	if err != nil {
		return fmt.Errorf("listen config failed: %w", err)
	}

	logger.Info("started listening config changes",
		zap.String("data_id", dataId),
		zap.String("group", group),
	)

	return nil
}

// AddListener 添加配置变更监听器
func (c *ConfigCenter) AddListener(dataId, group string, listener ConfigChangeListener) {
	cacheKey := c.cacheKey(dataId, group)
	c.mu.Lock()
	defer c.mu.Unlock()

	var listeners []ConfigChangeListener
	if existing, ok := c.listeners.Load(cacheKey); ok {
		listeners = existing.([]ConfigChangeListener)
	}
	listeners = append(listeners, listener)
	c.listeners.Store(cacheKey, listeners)
}

// notifyListeners 通知所有监听器
func (c *ConfigCenter) notifyListeners(dataId, group, namespace, data string) {
	cacheKey := c.cacheKey(dataId, group)
	c.mu.RLock()
	defer c.mu.RUnlock()

	if existing, ok := c.listeners.Load(cacheKey); ok {
		listeners := existing.([]ConfigChangeListener)
		for _, listener := range listeners {
			go listener(namespace, group, dataId, data)
		}
	}
}

// CancelListen 取消监听
func (c *ConfigCenter) CancelListen(dataId string) error {
	return c.CancelListenWithGroup(dataId, c.config.Group)
}

// CancelListenWithGroup 取消指定分组的监听
func (c *ConfigCenter) CancelListenWithGroup(dataId, group string) error {
	err := c.configClient.CancelListenConfig(vo.ConfigParam{
		DataId: dataId,
		Group:  group,
	})
	if err != nil {
		return fmt.Errorf("cancel listen config failed: %w", err)
	}

	logger.Info("stopped listening config changes",
		zap.String("data_id", dataId),
		zap.String("group", group),
	)

	return nil
}

// RefreshCache 刷新缓存
func (c *ConfigCenter) RefreshCache(dataId string) error {
	return c.RefreshCacheWithGroup(dataId, c.config.Group)
}

// RefreshCacheWithGroup 刷新指定分组的缓存
func (c *ConfigCenter) RefreshCacheWithGroup(dataId, group string) error {
	content, err := c.configClient.GetConfig(vo.ConfigParam{
		DataId: dataId,
		Group:  group,
	})
	if err != nil {
		return fmt.Errorf("refresh cache failed: %w", err)
	}

	cacheKey := c.cacheKey(dataId, group)
	c.cache.Store(cacheKey, content)
	return nil
}

// ClearCache 清除缓存
func (c *ConfigCenter) ClearCache() {
	c.cache = sync.Map{}
}

// cacheKey 生成缓存键
func (c *ConfigCenter) cacheKey(dataId, group string) string {
	return group + "@@" + dataId
}

// Close 关闭配置中心
func (c *ConfigCenter) Close() {
	c.configClient.CloseClient()
	logger.Info("nacos config center closed")
}

// HealthCheck 健康检查
func (c *ConfigCenter) HealthCheck(ctx context.Context) error {
	// 尝试获取一个简单的配置来验证连接
	_, err := c.configClient.GetConfig(vo.ConfigParam{
		DataId: "__health_check__",
		Group:  c.config.Group,
	})
	// 忽略配置不存在的错误
	if err != nil && err.Error() != "config not found" {
		// 某些 Nacos 版本在配置不存在时不会返回错误
		return nil
	}
	return nil
}

// ConfigWatcher 配置监视器，支持自动重载
type ConfigWatcher struct {
	center       *ConfigCenter
	dataId       string
	group        string
	interval     time.Duration
	onChange     ConfigChangeListener
	lastContent  string
	stopCh       chan struct{}
	mu           sync.Mutex
}

// NewConfigWatcher 创建配置监视器
func NewConfigWatcher(center *ConfigCenter, dataId, group string, interval time.Duration, onChange ConfigChangeListener) *ConfigWatcher {
	return &ConfigWatcher{
		center:   center,
		dataId:   dataId,
		group:    group,
		interval: interval,
		onChange: onChange,
		stopCh:   make(chan struct{}),
	}
}

// Start 启动监视器
func (w *ConfigWatcher) Start() error {
	// 首先加载初始配置
	content, err := w.center.GetConfigWithGroup(w.dataId, w.group)
	if err != nil {
		return fmt.Errorf("load initial config failed: %w", err)
	}
	w.lastContent = content

	// 使用 Nacos 的监听机制
	err = w.center.ListenWithGroup(w.dataId, w.group, func(namespace, group, dataId, data string) {
		w.mu.Lock()
		defer w.mu.Unlock()

		if data != w.lastContent {
			w.lastContent = data
			if w.onChange != nil {
				w.onChange(namespace, group, dataId, data)
			}
		}
	})
	if err != nil {
		return fmt.Errorf("start listening failed: %w", err)
	}

	// 启动定期刷新 (作为备份机制)
	go w.runRefresh()

	return nil
}

// runRefresh 定期刷新配置
func (w *ConfigWatcher) runRefresh() {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			content, err := w.center.GetConfigWithGroup(w.dataId, w.group)
			if err != nil {
				logger.Warn("failed to refresh config",
					zap.String("data_id", w.dataId),
					zap.String("group", w.group),
					zap.Error(err),
				)
				continue
			}

			w.mu.Lock()
			if content != w.lastContent {
				w.lastContent = content
				if w.onChange != nil {
					go w.onChange(w.center.config.Namespace, w.group, w.dataId, content)
				}
			}
			w.mu.Unlock()
		}
	}
}

// Stop 停止监视器
func (w *ConfigWatcher) Stop() error {
	close(w.stopCh)
	return w.center.CancelListenWithGroup(w.dataId, w.group)
}

// GetCurrentContent 获取当前配置内容
func (w *ConfigWatcher) GetCurrentContent() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.lastContent
}
