// ============================================================
//  【电脑端软件】运行目标: Windows / macOS / Linux / NAS 服务器
//  编译工具链: Go 1.21+ (跨平台单二进制, 不依赖 ESP32)
//  运行方式: 直接运行二进制 或 Docker 容器 / systemd 服务
// ============================================================
//
// Package util - JPEG 元数据提取
//
// 从 JPEG 字节流中解析图片宽高, 用于照片入库时记录维度信息
package util

import (
	"encoding/binary"
	"fmt"
)

// JPEGDimensions 从 JPEG 字节数据中提取宽度和高度
//
// 解析原理: 扫描 JPEG 标记段, 找到 SOF (Start Of Frame) 标记
// (0xFFC0 - 0xFFCF, 排除 0xFFC4/0xFFC8), 从中读取 2 字节高 + 2 字节宽
//
// 返回 (width, height, error)
func JPEGDimensions(data []byte) (int, int, error) {
	if len(data) < 4 {
		return 0, 0, fmt.Errorf("数据过短, 不是有效 JPEG")
	}

	// 校验 JPEG 头 (SOI: 0xFF 0xD8)
	if data[0] != 0xFF || data[1] != 0xD8 {
		return 0, 0, fmt.Errorf("非 JPEG 数据 (缺少 SOI 标记)")
	}

	// 从偏移 2 开始扫描标记段
	offset := 2
	for offset < len(data)-1 {
		// 查找标记前缀 0xFF
		if data[offset] != 0xFF {
			offset++
			continue
		}

		marker := data[offset+1]

		// SOI / EOI / RSTn: 无长度字段, 跳过
		if marker == 0xD8 || marker == 0xD9 ||
			(marker >= 0xD0 && marker <= 0xD7) {
			offset += 2
			continue
		}

		// SOF 标记: 0xC0 - 0xCF (排除 0xC4 = DHT, 0xC8 = JPG, 0xCC = DAC)
		if marker >= 0xC0 && marker <= 0xCF &&
			marker != 0xC4 && marker != 0xC8 && marker != 0xCC {
			// 需要: 2字节长度 + 1字节精度 + 2字节高 + 2字节宽 = 至少 7 字节
			if offset+9 >= len(data) {
				return 0, 0, fmt.Errorf("SOF 段数据不完整")
			}
			// precision (1 byte, offset+4)
			// height (2 bytes, big-endian, offset+5..6)
			height := int(binary.BigEndian.Uint16(data[offset+5 : offset+7]))
			// width (2 bytes, big-endian, offset+7..8)
			width := int(binary.BigEndian.Uint16(data[offset+7 : offset+9]))
			return width, height, nil
		}

		// 其他标记段: 读取 2 字节长度, 跳过
		if offset+3 >= len(data) {
			return 0, 0, fmt.Errorf("标记段长度字段越界")
		}
		segLen := int(binary.BigEndian.Uint16(data[offset+2 : offset+4]))
		if segLen < 2 {
			return 0, 0, fmt.Errorf("无效标记段长度: %d", segLen)
		}
		offset += 2 + segLen
	}

	return 0, 0, fmt.Errorf("未找到 SOF 标记")
}
