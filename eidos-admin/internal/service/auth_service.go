package service

import (
	"context"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/model"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/repository"
)

// AuthService 认证服务
type AuthService struct {
	adminRepo      *repository.AdminRepository
	auditRepo      *repository.AuditLogRepository
	jwtSecret      []byte
	jwtExpireHours int
	maxAttempts    int
	lockDuration   time.Duration
}

// AuthServiceConfig 认证服务配置
type AuthServiceConfig struct {
	JWTSecret      string
	JWTExpireHours int
	MaxAttempts    int
	LockDuration   time.Duration
}

// NewAuthService 创建认证服务
func NewAuthService(adminRepo *repository.AdminRepository, auditRepo *repository.AuditLogRepository, cfg *AuthServiceConfig) *AuthService {
	return &AuthService{
		adminRepo:      adminRepo,
		auditRepo:      auditRepo,
		jwtSecret:      []byte(cfg.JWTSecret),
		jwtExpireHours: cfg.JWTExpireHours,
		maxAttempts:    cfg.MaxAttempts,
		lockDuration:   cfg.LockDuration,
	}
}

// Claims JWT Claims
type Claims struct {
	AdminID     int64      `json:"admin_id"`
	Username    string     `json:"username"`
	Role        model.Role `json:"role"`
	Permissions []string   `json:"permissions"`
	jwt.RegisteredClaims
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	Token     string       `json:"token"`
	ExpiresAt int64        `json:"expires_at"`
	Admin     *model.Admin `json:"admin"`
}

// Login 登录
func (s *AuthService) Login(ctx context.Context, req *LoginRequest, ip, userAgent string) (*LoginResponse, error) {
	// 查找管理员
	admin, err := s.adminRepo.GetByUsername(ctx, req.Username)
	if err != nil {
		return nil, err
	}
	if admin == nil {
		s.recordLoginAudit(ctx, 0, req.Username, ip, userAgent, false, "用户不存在")
		return nil, errors.New("用户名或密码错误")
	}

	// 检查账户状态
	if admin.Status != model.AdminStatusActive {
		s.recordLoginAudit(ctx, admin.ID, admin.Username, ip, userAgent, false, "账户已禁用")
		return nil, errors.New("账户已禁用")
	}

	// 检查是否被锁定
	locked, err := s.adminRepo.IsLocked(ctx, admin.ID)
	if err != nil {
		return nil, err
	}
	if locked {
		s.recordLoginAudit(ctx, admin.ID, admin.Username, ip, userAgent, false, "账户已锁定")
		return nil, errors.New("账户已锁定，请稍后再试")
	}

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.Password)); err != nil {
		// 记录登录失败
		s.adminRepo.UpdateLoginFailed(ctx, admin.ID, s.maxAttempts, s.lockDuration)
		s.recordLoginAudit(ctx, admin.ID, admin.Username, ip, userAgent, false, "密码错误")
		return nil, errors.New("用户名或密码错误")
	}

	// 生成 JWT Token
	expiresAt := time.Now().Add(time.Duration(s.jwtExpireHours) * time.Hour)
	token, err := s.generateToken(admin, expiresAt)
	if err != nil {
		return nil, err
	}

	// 更新登录成功信息
	if err := s.adminRepo.UpdateLoginSuccess(ctx, admin.ID, ip); err != nil {
		return nil, err
	}

	// 记录登录审计
	s.recordLoginAudit(ctx, admin.ID, admin.Username, ip, userAgent, true, "")

	// 清除密码
	admin.PasswordHash = ""

	return &LoginResponse{
		Token:     token,
		ExpiresAt: expiresAt.UnixMilli(),
		Admin:     admin,
	}, nil
}

// generateToken 生成 JWT Token
func (s *AuthService) generateToken(admin *model.Admin, expiresAt time.Time) (string, error) {
	permissions := model.RolePermissions[admin.Role]

	claims := &Claims{
		AdminID:     admin.ID,
		Username:    admin.Username,
		Role:        admin.Role,
		Permissions: permissions,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "eidos-admin",
			Subject:   admin.Username,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

// ValidateToken 验证 JWT Token
func (s *AuthService) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return s.jwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}

// Logout 登出
func (s *AuthService) Logout(ctx context.Context, adminID int64, username, ip, userAgent string) error {
	// 记录登出审计
	auditLog := &model.AuditLog{
		AdminID:       adminID,
		AdminUsername: username,
		IPAddress:     ip,
		UserAgent:     userAgent,
		Action:        model.AuditActionLogout,
		ResourceType:  model.ResourceTypeAdmin,
		ResourceID:    username,
		Description:   "管理员登出",
		Status:        model.AuditStatusSuccess,
	}
	return s.auditRepo.Create(ctx, auditLog)
}

// ChangePassword 修改密码
func (s *AuthService) ChangePassword(ctx context.Context, adminID int64, oldPassword, newPassword string) error {
	admin, err := s.adminRepo.GetByID(ctx, adminID)
	if err != nil {
		return err
	}
	if admin == nil {
		return errors.New("管理员不存在")
	}

	// 验证旧密码
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(oldPassword)); err != nil {
		return errors.New("原密码错误")
	}

	// 生成新密码哈希
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	admin.PasswordHash = string(hash)
	admin.UpdatedBy = adminID
	return s.adminRepo.Update(ctx, admin)
}

// HashPassword 密码哈希
func (s *AuthService) HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// recordLoginAudit 记录登录审计
func (s *AuthService) recordLoginAudit(ctx context.Context, adminID int64, username, ip, userAgent string, success bool, errorMsg string) {
	status := model.AuditStatusSuccess
	if !success {
		status = model.AuditStatusFailed
	}

	description := "管理员登录成功"
	if !success {
		description = "管理员登录失败"
	}

	auditLog := &model.AuditLog{
		AdminID:       adminID,
		AdminUsername: username,
		IPAddress:     ip,
		UserAgent:     userAgent,
		Action:        model.AuditActionLogin,
		ResourceType:  model.ResourceTypeAdmin,
		ResourceID:    username,
		Description:   description,
		Status:        status,
		ErrorMessage:  errorMsg,
	}
	s.auditRepo.Create(ctx, auditLog)
}
