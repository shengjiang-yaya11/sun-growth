// ============================================================
//  【电脑端软件】运行目标: Windows / macOS / Linux / NAS 服务器
//  编译工具链: Go 1.21+ (跨平台单二进制, 不依赖 ESP32)
//  运行方式: 直接运行二进制 或 Docker 容器 / systemd 服务
// ============================================================
//
// Package auth - 设备认证与安全验证
package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// Security 负责设备认证、签名验证、解密
type Security struct {
	devices           map[string][]byte // device_id -> secret (32 bytes)
	timestampTolerance int64             // 时间戳容差(秒)
	usedNonces        map[string]time.Time // 防重放: 已使用的 nonce
	mu                sync.RWMutex
}

// New 创建安全模块
func New(devices map[string]string, tolerance int64) *Security {
	s := &Security{
		devices:           make(map[string][]byte),
		timestampTolerance: tolerance,
		usedNonces:        make(map[string]time.Time),
	}

	// 将十六进制密钥转为字节
	for id, hexKey := range devices {
		key, err := hex.DecodeString(hexKey)
		if err != nil || len(key) != 32 {
			continue
		}
		s.devices[id] = key
	}

	// 启动 nonce 清理协程
	go s.cleanupNonces()

	return s
}

// VerifyRequest 验证设备请求签名
// 返回: 设备密钥(用于后续解密), 错误
func (s *Security) VerifyRequest(deviceID, timestamp, nonce, signatureB64, payloadHashB64 string) ([]byte, error) {
	s.mu.RLock()
	secret, exists := s.devices[deviceID]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("unknown device: %s", deviceID)
	}

	// 解码签名
	signature, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return nil, fmt.Errorf("invalid signature encoding: %w", err)
	}
	if len(signature) != 32 {
		return nil, fmt.Errorf("invalid signature length: %d", len(signature))
	}

	// 解码载荷哈希
	payloadHash, err := base64.StdEncoding.DecodeString(payloadHashB64)
	if err != nil {
		return nil, fmt.Errorf("invalid hash encoding: %w", err)
	}
	if len(payloadHash) != 32 {
		return nil, fmt.Errorf("invalid hash length: %d", len(payloadHash))
	}

	// 验证时间戳 (防重放)
	var ts int64
	fmt.Sscanf(timestamp, "%d", &ts)
	now := time.Now().Unix()
	if now-ts > s.timestampTolerance || ts-now > s.timestampTolerance {
		return nil, fmt.Errorf("timestamp out of tolerance: ts=%d now=%d", ts, now)
	}

	// 检查 nonce 是否已使用 (防重放)
	s.mu.Lock()
	if _, used := s.usedNonces[nonce]; used {
		s.mu.Unlock()
		return nil, fmt.Errorf("nonce already used (replay attack)")
	}
	s.usedNonces[nonce] = time.Now()
	s.mu.Unlock()

	// 计算期望签名
	// sig = HMAC(secret, device_id || \0 || timestamp || \0 || nonce || \0 || payload_hash)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(deviceID))
	mac.Write([]byte{0})
	mac.Write([]byte(timestamp))
	mac.Write([]byte{0})
	mac.Write([]byte(nonce))
	mac.Write([]byte{0})
	mac.Write(payloadHash)
	expectedSig := mac.Sum(nil)

	// 常量时间比较
	if !hmac.Equal(signature, expectedSig) {
		return nil, fmt.Errorf("signature mismatch")
	}

	return secret, nil
}

// Decrypt 解密照片数据
// 密文格式: [nonce(12)] [hmac_tag(32)] [ciphertext(N)]
func (s *Security) Decrypt(encrypted []byte, secret []byte) ([]byte, error) {
	const overhead = 44 // nonce(12) + tag(32)
	if len(encrypted) < overhead {
		return nil, fmt.Errorf("encrypted data too short: %d", len(encrypted))
	}

	nonce := encrypted[:12]
	tag := encrypted[12:44]
	ciphertext := encrypted[44:]

	// 1. 验证 HMAC (Encrypt-then-MAC)
	// MAC key = HMAC(secret, "bio-mac-key-v1")
	macKeyMac := hmac.New(sha256.New, secret)
	macKeyMac.Write([]byte("bio-mac-key-v1"))
	macKey := macKeyMac.Sum(nil)[:32]

	mac := hmac.New(sha256.New, macKey)
	mac.Write(nonce)
	mac.Write(ciphertext)
	expectedTag := mac.Sum(nil)

	if !hmac.Equal(tag, expectedTag) {
		return nil, fmt.Errorf("MAC verification failed (data tampered)")
	}

	// 2. 解密
	// Enc key = HMAC(secret, "bio-enc-key-v1")
	encKeyMac := hmac.New(sha256.New, secret)
	encKeyMac.Write([]byte("bio-enc-key-v1"))
	encKey := encKeyMac.Sum(nil)[:32]

	block, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, fmt.Errorf("AES init failed: %w", err)
	}

	plaintext := make([]byte, len(ciphertext))
	// ESP32 端 mbedTLS 使用 16 字节 nonce_counter (12 字节 nonce + 4 字节计数器初值 0)
	// Go cipher.NewCTR 要求 IV 长度等于 AES 块大小 (16 字节)
	// 将 12 字节 nonce 扩展为 16 字节 IV: [nonce(12)] [counter(4)=0]
	iv := make([]byte, 16)
	copy(iv[:12], nonce)
	stream := cipher.NewCTR(block, iv)
	stream.XORKeyStream(plaintext, ciphertext)

	return plaintext, nil
}

// VerifyPayloadHash 验证载荷 SHA-256 哈希
func (s *Security) VerifyPayloadHash(data []byte, expectedHashB64 string) bool {
	hash := sha256.Sum256(data)
	expected, err := base64.StdEncoding.DecodeString(expectedHashB64)
	if err != nil {
		return false
	}
	return hmac.Equal(hash[:], expected)
}

// cleanupNonces 定期清理过期的 nonce
func (s *Security) cleanupNonces() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		cutoff := time.Now().Add(-time.Duration(s.timestampTolerance) * 2 * time.Second)
		for nonce, t := range s.usedNonces {
			if t.Before(cutoff) {
				delete(s.usedNonces, nonce)
			}
		}
		s.mu.Unlock()
	}
}

// RegisterDevice 注册新设备
func (s *Security) RegisterDevice(deviceID string, secretHex string) error {
	key, err := hex.DecodeString(secretHex)
	if err != nil || len(key) != 32 {
		return fmt.Errorf("invalid secret: must be 32 bytes hex")
	}
	s.mu.Lock()
	s.devices[deviceID] = key
	s.mu.Unlock()
	return nil
}

// ListDevices 列出已注册设备
func (s *Security) ListDevices() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var ids []string
	for id := range s.devices {
		ids = append(ids, id)
	}
	return ids
}

// RemoveDevice 移除已注册设备
func (s *Security) RemoveDevice(deviceID string) {
	s.mu.Lock()
	delete(s.devices, deviceID)
	s.mu.Unlock()
}

// GetDeviceSecret 返回设备密钥 (用于数据库迁移等场景)
func (s *Security) GetDeviceSecret(deviceID string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key, ok := s.devices[deviceID]
	if !ok {
		return "", false
	}
	return hex.EncodeToString(key), true
}

// ======================= CSRF 保护 (Double-Submit Cookie 模式) =======================

// GenerateCSRFToken 生成随机 CSRF token (32 字节十六进制)
func GenerateCSRFToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("生成 CSRF token 失败: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// ValidateCSRF 校验 double-submit cookie 模式的 CSRF token
// 要求 cookie 中的 token 与表单/请求头中的 token 一致且非空
func ValidateCSRF(cookieToken, formToken string) bool {
	if cookieToken == "" || formToken == "" {
		return false
	}
	return hmac.Equal([]byte(cookieToken), []byte(formToken))
}
