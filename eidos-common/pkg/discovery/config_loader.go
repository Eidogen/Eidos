package discovery

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/nacos"
	"gopkg.in/yaml.v3"
)

// ConfigLoader 配置加载器
// 支持从 Nacos 配置中心或环境变量加载配置
type ConfigLoader struct {
	configCenter *nacos.ConfigCenter
	serviceName  string
	group        string
	logger       *slog.Logger
	watchers     map[string]*nacos.ConfigWatcher
	mu           sync.RWMutex
}

// ConfigLoaderOption 配置加载器选项
type ConfigLoaderOption func(*ConfigLoader)

// WithGroup 设置配置分组
func WithGroup(group string) ConfigLoaderOption {
	return func(cl *ConfigLoader) {
		cl.group = group
	}
}

// WithLogger 设置日志器
func WithLogger(logger *slog.Logger) ConfigLoaderOption {
	return func(cl *ConfigLoader) {
		cl.logger = logger
	}
}

// NewConfigLoader 创建配置加载器
func NewConfigLoader(configCenter *nacos.ConfigCenter, serviceName string, opts ...ConfigLoaderOption) *ConfigLoader {
	cl := &ConfigLoader{
		configCenter: configCenter,
		serviceName:  serviceName,
		group:        "EIDOS_GROUP",
		logger:       slog.Default(),
		watchers:     make(map[string]*nacos.ConfigWatcher),
	}

	for _, opt := range opts {
		opt(cl)
	}

	return cl
}

// LoadConfig 从 Nacos 加载配置到结构体
// dataId 是配置的 ID，如 "eidos-trading" 或 "eidos-trading.yaml"
// target 是目标配置结构体指针
func (cl *ConfigLoader) LoadConfig(dataId string, target interface{}) error {
	if cl.configCenter == nil {
		cl.logger.Info("nacos config center not available, using env/defaults")
		return cl.loadFromEnv(target)
	}

	content, err := cl.configCenter.GetConfigWithGroup(dataId, cl.group)
	if err != nil {
		cl.logger.Warn("failed to load config from nacos, fallback to env",
			"dataId", dataId,
			"error", err,
		)
		return cl.loadFromEnv(target)
	}

	if content == "" {
		cl.logger.Info("empty config from nacos, using env/defaults", "dataId", dataId)
		return cl.loadFromEnv(target)
	}

	// 根据内容格式解析
	if err := cl.parseContent(content, target); err != nil {
		return fmt.Errorf("parse config content: %w", err)
	}

	cl.logger.Info("config loaded from nacos",
		"dataId", dataId,
		"group", cl.group,
	)

	return nil
}

// LoadConfigWithWatch 加载配置并监听变更
func (cl *ConfigLoader) LoadConfigWithWatch(dataId string, target interface{}, onChange func(interface{})) error {
	// 首先加载配置
	if err := cl.LoadConfig(dataId, target); err != nil {
		return err
	}

	if cl.configCenter == nil {
		return nil // 无 Nacos 时不监听
	}

	// 创建监视器
	watcher := nacos.NewConfigWatcher(
		cl.configCenter,
		dataId,
		cl.group,
		30*time.Second,
		func(namespace, group, dataID, data string) {
			cl.logger.Info("config changed, reloading",
				"dataId", dataID,
				"group", group,
			)

			// 创建新的目标实例
			targetType := reflect.TypeOf(target).Elem()
			newTarget := reflect.New(targetType).Interface()

			if err := cl.parseContent(data, newTarget); err != nil {
				cl.logger.Error("failed to parse updated config",
					"dataId", dataID,
					"error", err,
				)
				return
			}

			if onChange != nil {
				onChange(newTarget)
			}
		},
	)

	if err := watcher.Start(); err != nil {
		cl.logger.Warn("failed to start config watcher",
			"dataId", dataId,
			"error", err,
		)
		return nil // 不阻塞启动
	}

	cl.mu.Lock()
	cl.watchers[dataId] = watcher
	cl.mu.Unlock()

	cl.logger.Info("config watcher started", "dataId", dataId)
	return nil
}

// parseContent 解析配置内容
func (cl *ConfigLoader) parseContent(content string, target interface{}) error {
	content = strings.TrimSpace(content)

	// 尝试 JSON
	if strings.HasPrefix(content, "{") || strings.HasPrefix(content, "[") {
		if err := json.Unmarshal([]byte(content), target); err == nil {
			return nil
		}
	}

	// 尝试 YAML
	if err := yaml.Unmarshal([]byte(content), target); err == nil {
		return nil
	}

	return fmt.Errorf("unable to parse config content")
}

// loadFromEnv 从环境变量加载配置
func (cl *ConfigLoader) loadFromEnv(target interface{}) error {
	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return fmt.Errorf("target must be a non-nil pointer")
	}

	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return nil // 非结构体不处理
	}

	cl.loadStructFromEnv(v, "")
	return nil
}

// loadStructFromEnv 递归从环境变量加载结构体字段
func (cl *ConfigLoader) loadStructFromEnv(v reflect.Value, prefix string) {
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)

		if !field.CanSet() {
			continue
		}

		// 获取 env tag
		envTag := fieldType.Tag.Get("env")
		if envTag == "" {
			// 使用字段名生成环境变量名
			envTag = strings.ToUpper(toSnakeCase(fieldType.Name))
		}

		if prefix != "" {
			envTag = prefix + "_" + envTag
		}

		// 递归处理嵌套结构体
		if field.Kind() == reflect.Struct {
			cl.loadStructFromEnv(field, envTag)
			continue
		}

		// 从环境变量获取值
		envVal := os.Getenv(envTag)
		if envVal == "" {
			continue
		}

		// 设置字段值
		if err := cl.setFieldValue(field, envVal); err != nil {
			cl.logger.Debug("failed to set field from env",
				"field", fieldType.Name,
				"env", envTag,
				"error", err,
			)
		}
	}
}

// setFieldValue 设置字段值
func (cl *ConfigLoader) setFieldValue(field reflect.Value, value string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if field.Type() == reflect.TypeOf(time.Duration(0)) {
			d, err := time.ParseDuration(value)
			if err != nil {
				return err
			}
			field.SetInt(int64(d))
		} else {
			v, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return err
			}
			field.SetInt(v)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return err
		}
		field.SetUint(v)
	case reflect.Float32, reflect.Float64:
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return err
		}
		field.SetFloat(v)
	case reflect.Bool:
		v, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		field.SetBool(v)
	case reflect.Slice:
		// 支持逗号分隔的字符串切片
		if field.Type().Elem().Kind() == reflect.String {
			parts := strings.Split(value, ",")
			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}
			field.Set(reflect.ValueOf(parts))
		}
	}
	return nil
}

// toSnakeCase 转换为蛇形命名
func toSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune('_')
		}
		result.WriteRune(r)
	}
	return result.String()
}

// PublishConfig 发布配置到 Nacos
func (cl *ConfigLoader) PublishConfig(dataId string, config interface{}) error {
	if cl.configCenter == nil {
		return fmt.Errorf("nacos config center not available")
	}

	content, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	return cl.configCenter.PublishConfigWithGroup(dataId, cl.group, string(content))
}

// Close 关闭配置加载器
func (cl *ConfigLoader) Close() {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	for dataId, watcher := range cl.watchers {
		if err := watcher.Stop(); err != nil {
			cl.logger.Error("failed to stop config watcher",
				"dataId", dataId,
				"error", err,
			)
		}
	}
	cl.watchers = make(map[string]*nacos.ConfigWatcher)
}

// GetConfigCenter 获取配置中心实例
func (cl *ConfigLoader) GetConfigCenter() *nacos.ConfigCenter {
	return cl.configCenter
}

// CreateConfigCenter 创建 Nacos 配置中心（辅助函数）
func CreateConfigCenter(serverAddr, namespace, group, username, password string) (*nacos.ConfigCenter, error) {
	if serverAddr == "" {
		return nil, nil // 未配置则返回 nil
	}

	cfg := &nacos.ConfigCenterConfig{
		ServerAddr: serverAddr,
		Namespace:  namespace,
		Group:      group,
		Username:   username,
		Password:   password,
		LogDir:     "/tmp/nacos/log",
		CacheDir:   "/tmp/nacos/cache",
		LogLevel:   "warn",
		TimeoutMs:  5000,
	}

	return nacos.NewConfigCenter(cfg)
}
