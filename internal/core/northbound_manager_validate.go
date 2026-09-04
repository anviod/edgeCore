package core

import (
	"fmt"
	"regexp"
	"strings"
)

// validChannelID 通道 ID 只允许英文字母、数字、下划线、横线，禁止空格等其它字符，
// 保证 ID 可安全用作 URL 路径段、MQTT Topic 段等场景。
var validChannelID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// validateNorthboundChannelID 校验通道 ID 格式（空串视为未设置，由调用方决定是否生成）。
func validateNorthboundChannelID(id string) error {
	if id == "" {
		return nil
	}
	if !validChannelID.MatchString(id) {
		return fmt.Errorf("通道 ID「%s」格式非法：只能包含英文字母、数字、下划线或横线，且不能包含空格", id)
	}
	return nil
}

// validateNorthboundChannelName checks that name is non-empty and unique across all
// northbound protocols. excludeID is the channel being updated (empty when creating).
// Caller must hold nm.mu.
func (nm *NorthboundManager) validateNorthboundChannelName(excludeID, name string) error {
	if err := validateNorthboundChannelID(excludeID); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("通道名称不能为空")
	}
	for _, c := range nm.config.MQTT {
		if c.ID != excludeID && strings.EqualFold(strings.TrimSpace(c.Name), name) {
			return fmt.Errorf("通道名称「%s」已存在", name)
		}
	}
	for _, c := range nm.config.HTTP {
		if c.ID != excludeID && strings.EqualFold(strings.TrimSpace(c.Name), name) {
			return fmt.Errorf("通道名称「%s」已存在", name)
		}
	}
	for _, c := range nm.config.OPCUA {
		if c.ID != excludeID && strings.EqualFold(strings.TrimSpace(c.Name), name) {
			return fmt.Errorf("通道名称「%s」已存在", name)
		}
	}
	for _, c := range nm.config.SparkplugB {
		if c.ID != excludeID && strings.EqualFold(strings.TrimSpace(c.Name), name) {
			return fmt.Errorf("通道名称「%s」已存在", name)
		}
	}
	for _, c := range nm.config.EdgeOSMQTT {
		if c.ID != excludeID && strings.EqualFold(strings.TrimSpace(c.Name), name) {
			return fmt.Errorf("通道名称「%s」已存在", name)
		}
	}
	for _, c := range nm.config.EdgeOSNATS {
		if c.ID != excludeID && strings.EqualFold(strings.TrimSpace(c.Name), name) {
			return fmt.Errorf("通道名称「%s」已存在", name)
		}
	}
	return nil
}
