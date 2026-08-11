// ============================================================
//  device_control.go - ESP32 设备双向控制模块 (v3.1)
//
//  PC → ESP32: 发送控制命令 (TCP port 8081)
//  ESP32 → PC: 响应 + 照片上传 (HTTP POST /api/v1/upload)
//
//  功能:
//    - 设备发现 (扫描局域网)
//    - 命令下发 (拍照/状态/配置/重启)
//    - 设备状态轮询
//    - HMAC-SHA256 命令签名
// ============================================================

package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ======================= 设备控制客户端 =======================

// DeviceController ESP32 设备控制器
type DeviceController struct {
	mu        sync.Mutex
	devices   map[string]*ESP32Device // deviceID -> device
	tcpTimeout time.Duration
}

// ESP32Device 已发现的 ESP32 设备
type ESP32Device struct {
	DeviceID        string `json:"device_id"`
	IP              string `json:"ip"`
	Port            int    `json:"port"`
	FirmwareVersion string `json:"fw_version"`
	LastSeen        time.Time `json:"last_seen"`
	Online          bool   `json:"online"`
}

// DeviceStatus ESP32 设备状态 (从命令响应获取)
type DeviceStatus struct {
	IP        string `json:"ip"`
	UptimeSec int64  `json:"uptime_sec"`
	FreeHeap  uint32 `json:"free_heap"`
	WiFiRSSI  int    `json:"wifi_rssi"`
	FWVersion string `json:"fw_version"`
}

// CommandRequest 发送给 ESP32 的命令
type CommandRequest struct {
	Cmd    string      `json:"cmd"`
	Params interface{} `json:"params,omitempty"`
	TS     int64       `json:"ts"`  // 时间戳
	Sig    string      `json:"sig"` // HMAC 签名
}

// NewDeviceController 创建设备控制器
func NewDeviceController() *DeviceController {
	return &DeviceController{
		devices:    make(map[string]*ESP32Device),
		tcpTimeout: 5 * time.Second,
	}
}

// ======================= 设备发现 =======================

// DiscoverDevices 扫描局域网发现 ESP32 设备
// 扫描 8081 端口 (ESP32 命令服务器端口)
func (dc *DeviceController) DiscoverDevices() []ESP32Device {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	var discovered []ESP32Device

	// 获取本机所有局域网 IP
	localIPs := dc.getLocalIPs()
	if len(localIPs) == 0 {
		slog.Warn("设备发现: 未检测到局域网连接")
		return discovered
	}

	// 对每个网段 /24 扫描 8081 端口
	type result struct {
		ip   string
		dev  *ESP32Device
	}
	results := make(chan result, 256)

	var wg sync.WaitGroup
	for _, ipNet := range localIPs {
		// 扫描 /24 子网
		baseIP := ipNet.Mask(net.CIDRMask(24, 32))
		for i := 1; i < 255; i++ {
			ip := make(net.IP, 4)
			copy(ip, baseIP)
			ip[3] = byte(i)

			// 跳过本机 IP
			if dc.isLocalIP(ip) {
				continue
			}

			wg.Add(1)
			go func(targetIP string) {
				defer wg.Done()
				dev := dc.probeDevice(targetIP, 8081)
				results <- result{ip: targetIP, dev: dev}
			}(ip.String())
		}
	}

	// 等待所有扫描完成 (最多 3 秒)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}

	// 收集结果
	close(results)
	for r := range results {
		if r.dev != nil {
			dc.devices[r.dev.DeviceID] = r.dev
			discovered = append(discovered, *r.dev)
		}
	}

	slog.Info("设备发现完成", "found", len(discovered))
	return discovered
}

// probeDevice 探测单个设备
func (dc *DeviceController) probeDevice(ip string, port int) *ESP32Device {
	addr := fmt.Sprintf("%s:%d", ip, port)
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return nil
	}
	defer conn.Close()

	// 发送状态查询命令
	req := CommandRequest{Cmd: "status", TS: time.Now().Unix()}
	reqJSON, _ := json.Marshal(req)

	conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Write(append(reqJSON, '\n')); err != nil {
		return nil
	}

	var respBuf [1024]byte
	n, err := conn.Read(respBuf[:])
	if err != nil || n == 0 {
		return nil
	}

	// 解析响应
	var resp struct {
		Status string       `json:"status"`
		Data   DeviceStatus `json:"data"`
	}
	if err := json.Unmarshal(respBuf[:n], &resp); err != nil {
		return nil
	}

	if resp.Status != "ok" {
		return nil
	}

	return &ESP32Device{
		DeviceID:        fmt.Sprintf("esp32-%s", strings.ReplaceAll(ip, ".", "-")),
		IP:              ip,
		Port:            port,
		FirmwareVersion: resp.Data.FWVersion,
		LastSeen:        time.Now(),
		Online:          true,
	}
}

// ======================= 命令发送 =======================

// SendCommand 向 ESP32 发送命令 (带签名)
// secretHex: 设备密钥 (32 字节十六进制)
func (dc *DeviceController) SendCommand(deviceIP string, port int,
	cmd string, params interface{}, secretHex string) (string, error) {

	// 构建请求
	ts := time.Now().Unix()
	paramsJSON := "{}"
	if params != nil {
		b, _ := json.Marshal(params)
		paramsJSON = string(b)
	}

	// 计算签名: HMAC(secret, cmd || paramsJSON || ts)
	signPayload := cmd + paramsJSON + strconv.FormatInt(ts, 10)
	sig := dc.computeSignature(secretHex, signPayload)

	req := CommandRequest{
		Cmd:    cmd,
		Params: params,
		TS:     ts,
		Sig:    sig,
	}
	reqJSON, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	// TCP 连接
	addr := fmt.Sprintf("%s:%d", deviceIP, port)
	conn, err := net.DialTimeout("tcp", addr, dc.tcpTimeout)
	if err != nil {
		return "", fmt.Errorf("connect to %s: %w", addr, err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(dc.tcpTimeout))
	if _, err := conn.Write(append(reqJSON, '\n')); err != nil {
		return "", fmt.Errorf("write: %w", err)
	}

	var respBuf [2048]byte
	n, err := conn.Read(respBuf[:])
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read: %w", err)
	}

	return strings.TrimSpace(string(respBuf[:n])), nil
}

// CaptureNow 立即拍照
func (dc *DeviceController) CaptureNow(deviceIP string, port int, secretHex string) error {
	resp, err := dc.SendCommand(deviceIP, port, "capture", nil, secretHex)
	if err != nil {
		return err
	}
	var result struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	if result.Status != "ok" {
		return fmt.Errorf("capture failed: %s", resp)
	}
	slog.Info("远程拍照命令已发送", "device", deviceIP)
	return nil
}

// GetDeviceStatus 查询设备状态
func (dc *DeviceController) GetDeviceStatus(deviceIP string, port int) (*DeviceStatus, error) {
	resp, err := dc.SendCommand(deviceIP, port, "status", nil, "")
	if err != nil {
		return nil, err
	}
	var result struct {
		Status string       `json:"status"`
		Data   DeviceStatus `json:"data"`
	}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if result.Status != "ok" {
		return nil, fmt.Errorf("status query failed")
	}
	return &result.Data, nil
}

// SetInterval 设置拍照间隔 (秒)
func (dc *DeviceController) SetInterval(deviceIP string, port int,
	intervalSec int, secretHex string) error {
	params := map[string]int{"interval": intervalSec}
	resp, err := dc.SendCommand(deviceIP, port, "set_config", params, secretHex)
	if err != nil {
		return err
	}
	var result struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	if result.Status != "ok" {
		return fmt.Errorf("set_config failed: %s", resp)
	}
	slog.Info("拍照间隔已更新", "device", deviceIP, "interval", intervalSec)
	return nil
}

// ======================= 安全: HMAC 签名 =======================

func (dc *DeviceController) computeSignature(secretHex, payload string) string {
	if secretHex == "" {
		return ""
	}
	secret, err := hex.DecodeString(secretHex)
	if err != nil || len(secret) != 32 {
		return ""
	}

	// 派生签名密钥: HMAC(secret, "bio-cmd-sig-v1")
	sigKeyMac := hmac.New(sha256.New, secret)
	sigKeyMac.Write([]byte("bio-cmd-sig-v1"))
	sigKey := sigKeyMac.Sum(nil)[:32]

	// HMAC-SHA256(payload)
	mac := hmac.New(sha256.New, sigKey)
	mac.Write([]byte(payload))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// ======================= 辅助 =======================

func (dc *DeviceController) getLocalIPs() []net.IP {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var ips []net.IP
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok {
			ip4 := ipNet.IP.To4()
			if ip4 != nil && !ip4.IsLoopback() &&
				ip4[0] != 169 && ip4[0] != 254 { // 排除 APIPA
				ips = append(ips, ip4)
			}
		}
	}
	return ips
}

func (dc *DeviceController) isLocalIP(ip net.IP) bool {
	for _, local := range dc.getLocalIPs() {
		if ip.Equal(local) {
			return true
		}
	}
	return false
}
