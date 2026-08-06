package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/anviod/edgeCore/internal/model"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/time/rate"
)

// ==========================
// JWT Implementation
// ==========================
var DebugMode bool = true

type JWT struct {
	SigningKey []byte
}

type CustomClaims struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	jwt.RegisteredClaims
}

func NewJWT() *JWT {
	return &JWT{
		SigningKey: []byte("GATEWAY"), // TODO: Move to config
	}
}

func (j *JWT) CreateToken(claims CustomClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(j.SigningKey)
}

func (j *JWT) ParserToken(tokenString string) (*CustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		return j.SigningKey, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*CustomClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("token invalid")
}

// ==========================
// Middleware
// ==========================

func JWTAuth() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// WebSocket 实时值订阅（公开端点，浏览器无法稳定携带 Header token）
		switch c.Path() {
		case "/api/ws/values", "/ws", "/api/mcp", "/mcp":
			return c.Next()
		}

		token := c.Get("token")
		if token == "" {
			// Also check Authorization header Bearer token
			authHeader := c.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				token = strings.TrimPrefix(authHeader, "Bearer ")
			}
		}

		// Check Query Param (for WebSockets)
		if token == "" {
			token = c.Query("token")
		}

		if token == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code":    "1",
				"message": "请求未携带token，无权限访问",
				"data":    "",
			})
		}

		j := NewJWT()
		claims, err := j.ParserToken(token)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code":    "1",
				"message": "登录已经过期，请重新登录", // Simplify error message for user
				"data":    "",
			})
		}

		c.Locals("claims", claims)
		return c.Next()
	}
}

// ==========================
// Nonce & Rate Limiting
// ==========================

var (
	nonceStore sync.Map
	nonceMax   = 100000
)

var nonceLimiters sync.Map

func GetLimiter(ip string) *rate.Limiter {
	if l, ok := nonceLimiters.Load(ip); ok {
		return l.(*rate.Limiter)
	}
	limiter := rate.NewLimiter(2, 5) // 2 requests per second, burst 5
	nonceLimiters.Store(ip, limiter)
	return limiter
}

func GenerateNonce() (string, error) {
	// Simple size check to prevent memory exhaustion
	size := 0
	nonceStore.Range(func(_, _ any) bool {
		size++
		return true
	})
	if size > nonceMax {
		return "", fmt.Errorf("nonce store full")
	}

	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}

	nonce := hex.EncodeToString(b)
	nonceStore.Store(nonce, time.Now().Add(2*time.Minute))
	return nonce, nil
}

func ValidateAndConsumeNonce(nonce string) bool {
	v, ok := nonceStore.Load(nonce)
	if !ok {
		return false
	}

	expire := v.(time.Time)
	if time.Now().After(expire) {
		nonceStore.Delete(nonce)
		return false
	}

	nonceStore.Delete(nonce)
	return true
}

func init() {
	// Background cleanup for nonces
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		for range ticker.C {
			nonceStore.Range(func(key, value any) bool {
				expire := value.(time.Time)
				if time.Now().After(expire) {
					nonceStore.Delete(key)
				}
				return true
			})
		}
	}()
}

// ==========================
// Brute-force Protection
// ==========================

const (
	MaxLoginFailCount  = 10
	LoginBlockDuration = 3 * time.Minute
)

type LoginFailInfo struct {
	Count      int
	LastFailAt time.Time
	BlockUntil time.Time
}

var (
	loginFailMap   = make(map[string]*LoginFailInfo)
	loginFailMutex sync.Mutex
)

func IsIPBlocked(ip string) (bool, time.Duration) {
	loginFailMutex.Lock()
	defer loginFailMutex.Unlock()

	info, exists := loginFailMap[ip]
	if !exists {
		return false, 0
	}

	if time.Now().Before(info.BlockUntil) {
		return true, time.Until(info.BlockUntil)
	}

	return false, 0
}

func AddLoginFail(ip string) {
	loginFailMutex.Lock()
	defer loginFailMutex.Unlock()

	info, exists := loginFailMap[ip]
	if !exists {
		loginFailMap[ip] = &LoginFailInfo{
			Count:      1,
			LastFailAt: time.Now(),
		}
		return
	}

	info.Count++
	info.LastFailAt = time.Now()

	if info.Count >= MaxLoginFailCount {
		info.BlockUntil = time.Now().Add(LoginBlockDuration)
	}
}

func ClearLoginFail(ip string) {
	loginFailMutex.Lock()
	defer loginFailMutex.Unlock()
	delete(loginFailMap, ip)
}

// ==========================
// Handlers
// ==========================

// LoginRequest defines the login payload
type LoginRequest struct {
	LoginFlag bool   `json:"loginFlag"`
	LoginType string `json:"loginType"` // "local" or "ldap"
	Data      struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Nonce    string `json:"nonce"`
	} `json:"data"`
}

// LoginResponse defines the successful login response
type LoginResponse struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Username    string   `json:"username"`
		Token       string   `json:"token"`
		Permissions []string `json:"permissions"`
	} `json:"data"`
}

func (s *Server) handleGetSystemInfo(c *fiber.Ctx) error {
	cfg := s.sm.GetConfig()
	return c.JSON(fiber.Map{
		"code": "0",
		"data": fiber.Map{
			"name":      cfg.Hostname.Name,
			"softVer":   model.Version,
			"buildTime": model.FormatBuildTime(),
			"commitID":  model.CommitID,
		},
	})
}

func (s *Server) handleGetNonce(c *fiber.Ctx) error {
	ip := c.IP()
	limiter := GetLimiter(ip)
	if !limiter.Allow() {
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"code": "1", "msg": "请求过于频繁"})
	}

	nonce, err := GenerateNonce()
	if err != nil {
		log.Printf("Failed to generate nonce: %v", err)
		return c.JSON(fiber.Map{"code": "1", "msg": "Generate nonce failed"})
	}

	return c.JSON(fiber.Map{
		"code": "0",
		"data": fiber.Map{
			"nonce": nonce,
		},
	})
}

func (s *Server) handleLogin(c *fiber.Ctx) error {
	var req LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"code": "1", "msg": "Invalid request"})
	}

	ip := c.IP()
	if blocked, wait := IsIPBlocked(ip); blocked {
		return c.JSON(fiber.Map{
			"code": "1",
			"msg":  fmt.Sprintf("登录失败次数过多，请 %v 后重试", wait.Round(time.Second)),
		})
	}

	// 1. Verify Nonce (skip in debug mode if nonce is empty)
	if !DebugMode || req.Data.Nonce != "" {
		if !ValidateAndConsumeNonce(req.Data.Nonce) {
			return c.JSON(fiber.Map{"code": "1", "msg": "加密已过期或无效，请刷新页面"})
		}
	}

	var user *model.UserConfig
	var found bool

	if req.LoginType == "ldap" {
		// LDAP Authentication
		var err error
		var success bool
		success, user, err = s.AuthenticateLDAP(req.Data.Username, req.Data.Password)
		if !success {
			AddLoginFail(ip)
			msg := "LDAP认证失败"
			if err != nil {
				msg = fmt.Sprintf("LDAP认证失败: %v", err)
			}
			return c.JSON(fiber.Map{"code": "1", "msg": msg})
		}
		found = true
	} else {
		// Local Authentication (Default)
		// 2. Get User
		user, found = s.sm.GetUser(req.Data.Username)
		if !found {
			AddLoginFail(ip)
			return c.JSON(fiber.Map{"code": "1", "msg": "用户不存在"})
		}

		// 3. Verify Password
		// In debug mode with empty nonce, compare password directly (plain text)
		if DebugMode && req.Data.Nonce == "" {
			if req.Data.Password != user.Password {
				AddLoginFail(ip)
				return c.JSON(fiber.Map{"code": "1", "msg": "密码错误"})
			}
		} else {
			expected := sha256.Sum256([]byte(user.Password + req.Data.Nonce))
			expectedHex := hex.EncodeToString(expected[:])
			if req.Data.Password != expectedHex {
				AddLoginFail(ip)
				return c.JSON(fiber.Map{"code": "1", "msg": "密码错误"})
			}
		}
	}

	// 4. Login Success
	ClearLoginFail(ip)

	// 5. Generate JWT
	j := NewJWT()
	claims := CustomClaims{
		Name:  user.Username,
		Email: "",
		RegisteredClaims: jwt.RegisteredClaims{
			NotBefore: jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * 7 * time.Hour)), // 24*7 hours
			Issuer:    "IndustrialEdgeGateway",
		},
	}
	token, err := j.CreateToken(claims)
	if err != nil {
		log.Printf("Failed to generate token: %v", err)
		return c.JSON(fiber.Map{"code": "1", "msg": "Generate token failed"})
	}

	permissions := []string{}
	if user.Role != "" {
		permissions = append(permissions, user.Role)
	} else {
		permissions = append(permissions, "admin")
	}

	return c.JSON(LoginResponse{
		Code: "0",
		Msg:  "Success",
		Data: struct {
			Username    string   `json:"username"`
			Token       string   `json:"token"`
			Permissions []string `json:"permissions"`
		}{
			Username:    user.Username,
			Token:       token,
			Permissions: permissions,
		},
	})
}

func (s *Server) handleLogout(c *fiber.Ctx) error {
	// JWT is stateless, so we rely on client deleting the token.
	// Optionally we could blacklist the token here if we implemented a blacklist.
	return c.JSON(fiber.Map{
		"code": "0",
		"msg":  "Logged out",
	})
}

type ChangePasswordRequest struct {
	OldPassword string `json:"oldPassword"` // Hashed with nonce
	NewPassword string `json:"newPassword"` // Raw password (will be stored as is, assuming config stores plain text or we hash it here?)
	Nonce       string `json:"nonce"`
}

func (s *Server) handleChangePassword(c *fiber.Ctx) error {
	claims := c.Locals("claims").(*CustomClaims)
	username := claims.Name

	var req ChangePasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"code": "1", "msg": "Invalid request"})
	}

	// 1. Verify Nonce
	if !ValidateAndConsumeNonce(req.Nonce) {
		return c.JSON(fiber.Map{"code": "1", "msg": "加密已过期或无效，请刷新页面"})
	}

	// 2. Get User
	user, found := s.sm.GetUser(username)
	if !found {
		return c.JSON(fiber.Map{"code": "1", "msg": "用户不存在"})
	}

	// 3. Verify Old Password
	// The client should send SHA256(old_raw_password + nonce)
	// We verify by calculating SHA256(stored_password + nonce) and comparing
	expected := sha256.Sum256([]byte(user.Password + req.Nonce))
	expectedHex := hex.EncodeToString(expected[:])

	if req.OldPassword != expectedHex {
		return c.JSON(fiber.Map{"code": "1", "msg": "旧密码错误"})
	}

	// 4. Update Password
	// Assuming we store plain text password in config for now based on current logic
	if err := s.sm.UpdateUserPassword(username, req.NewPassword); err != nil {
		log.Printf("Failed to update password: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"code": "1", "msg": "修改密码失败"})
	}

	return c.JSON(fiber.Map{
		"code": "0",
		"msg":  "密码修改成功",
	})
}

// Helper to check login from other parts of the system if needed
func (s *Server) ValidateUser(username, password, nonce string) (*model.UserConfig, error) {
	user, found := s.sm.GetUser(username)
	if !found {
		return nil, fmt.Errorf("user not found")
	}
	expected := sha256.Sum256([]byte(user.Password + nonce))
	expectedHex := hex.EncodeToString(expected[:])
	if password != expectedHex {
		return nil, fmt.Errorf("invalid password")
	}
	return user, nil
}
