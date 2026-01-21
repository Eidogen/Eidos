package redis

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"sync"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
)

var (
	// ErrScriptNotFound 脚本未找到
	ErrScriptNotFound = errors.New("script not found")
	// ErrScriptLoadFailed 脚本加载失败
	ErrScriptLoadFailed = errors.New("script load failed")
)

// Script Lua 脚本封装
type Script struct {
	src  string
	hash string
}

// NewScript 创建脚本
func NewScript(src string) *Script {
	h := sha1.New()
	h.Write([]byte(src))
	return &Script{
		src:  src,
		hash: hex.EncodeToString(h.Sum(nil)),
	}
}

// Hash 获取脚本 SHA1 哈希
func (s *Script) Hash() string {
	return s.hash
}

// Source 获取脚本源码
func (s *Script) Source() string {
	return s.src
}

// Run 执行脚本 (优先使用 EVALSHA，失败则使用 EVAL)
func (s *Script) Run(ctx context.Context, client redis.Scripter, keys []string, args ...interface{}) *redis.Cmd {
	return redis.NewScript(s.src).Run(ctx, client, keys, args...)
}

// ScriptManager 脚本管理器
type ScriptManager struct {
	client  redis.UniversalClient
	scripts map[string]*Script
	loaded  map[string]bool
	mu      sync.RWMutex
}

// NewScriptManager 创建脚本管理器
func NewScriptManager(client redis.UniversalClient) *ScriptManager {
	return &ScriptManager{
		client:  client,
		scripts: make(map[string]*Script),
		loaded:  make(map[string]bool),
	}
}

// Register 注册脚本
func (m *ScriptManager) Register(name string, src string) *Script {
	m.mu.Lock()
	defer m.mu.Unlock()

	script := NewScript(src)
	m.scripts[name] = script
	m.loaded[name] = false

	return script
}

// Get 获取脚本
func (m *ScriptManager) Get(name string) (*Script, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	script, exists := m.scripts[name]
	return script, exists
}

// Load 加载脚本到 Redis
func (m *ScriptManager) Load(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	script, exists := m.scripts[name]
	if !exists {
		return ErrScriptNotFound
	}

	// 已加载则跳过
	if m.loaded[name] {
		return nil
	}

	// 加载脚本
	hash, err := m.client.ScriptLoad(ctx, script.src).Result()
	if err != nil {
		return err
	}

	// 验证哈希
	if hash != script.hash {
		logger.Warn("script hash mismatch",
			zap.String("name", name),
			zap.String("expected", script.hash),
			zap.String("actual", hash),
		)
	}

	m.loaded[name] = true
	logger.Debug("script loaded", zap.String("name", name), zap.String("hash", hash))

	return nil
}

// LoadAll 加载所有脚本
func (m *ScriptManager) LoadAll(ctx context.Context) error {
	m.mu.RLock()
	names := make([]string, 0, len(m.scripts))
	for name := range m.scripts {
		names = append(names, name)
	}
	m.mu.RUnlock()

	for _, name := range names {
		if err := m.Load(ctx, name); err != nil {
			logger.Error("failed to load script",
				zap.String("name", name),
				zap.Error(err),
			)
			return err
		}
	}

	return nil
}

// Run 执行脚本
func (m *ScriptManager) Run(ctx context.Context, name string, keys []string, args ...interface{}) (interface{}, error) {
	m.mu.RLock()
	script, exists := m.scripts[name]
	m.mu.RUnlock()

	if !exists {
		return nil, ErrScriptNotFound
	}

	result, err := script.Run(ctx, m.client, keys, args...).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		logger.Debug("script execution failed",
			zap.String("name", name),
			zap.Error(err),
		)
	}

	return result, err
}

// RunInt64 执行脚本并返回 int64
func (m *ScriptManager) RunInt64(ctx context.Context, name string, keys []string, args ...interface{}) (int64, error) {
	result, err := m.Run(ctx, name, keys, args...)
	if err != nil {
		return 0, err
	}

	switch v := result.(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case float64:
		return int64(v), nil
	default:
		return 0, errors.New("unexpected result type")
	}
}

// RunString 执行脚本并返回 string
func (m *ScriptManager) RunString(ctx context.Context, name string, keys []string, args ...interface{}) (string, error) {
	result, err := m.Run(ctx, name, keys, args...)
	if err != nil {
		return "", err
	}

	switch v := result.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	default:
		return "", errors.New("unexpected result type")
	}
}

// RunBool 执行脚本并返回 bool
func (m *ScriptManager) RunBool(ctx context.Context, name string, keys []string, args ...interface{}) (bool, error) {
	result, err := m.Run(ctx, name, keys, args...)
	if err != nil {
		return false, err
	}

	switch v := result.(type) {
	case int64:
		return v == 1, nil
	case int:
		return v == 1, nil
	case bool:
		return v, nil
	default:
		return false, errors.New("unexpected result type")
	}
}

// Exists 检查脚本是否已注册
func (m *ScriptManager) Exists(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.scripts[name]
	return exists
}

// IsLoaded 检查脚本是否已加载到 Redis
func (m *ScriptManager) IsLoaded(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.loaded[name]
}

// Flush 清除所有已加载脚本
func (m *ScriptManager) Flush(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.client.ScriptFlush(ctx).Err(); err != nil {
		return err
	}

	for name := range m.loaded {
		m.loaded[name] = false
	}

	logger.Info("all scripts flushed")
	return nil
}

// List 列出所有脚本
func (m *ScriptManager) List() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]string, len(m.scripts))
	for name, script := range m.scripts {
		result[name] = script.hash
	}
	return result
}

// EvalScript 直接执行 Lua 脚本 (不注册)
func EvalScript(ctx context.Context, client redis.UniversalClient, script string, keys []string, args ...interface{}) (interface{}, error) {
	return client.Eval(ctx, script, keys, args...).Result()
}

// EvalScriptInt64 直接执行脚本并返回 int64
func EvalScriptInt64(ctx context.Context, client redis.UniversalClient, script string, keys []string, args ...interface{}) (int64, error) {
	return client.Eval(ctx, script, keys, args...).Int64()
}

// EvalScriptString 直接执行脚本并返回 string
func EvalScriptString(ctx context.Context, client redis.UniversalClient, script string, keys []string, args ...interface{}) (string, error) {
	return client.Eval(ctx, script, keys, args...).Text()
}

// EvalScriptBool 直接执行脚本并返回 bool
func EvalScriptBool(ctx context.Context, client redis.UniversalClient, script string, keys []string, args ...interface{}) (bool, error) {
	return client.Eval(ctx, script, keys, args...).Bool()
}
