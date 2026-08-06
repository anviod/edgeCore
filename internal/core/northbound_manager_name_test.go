package core

import (
	"strings"
	"testing"

	"github.com/anviod/edgeCore/internal/model"
)

func TestNorthboundManager_ValidateChannelNameUnique(t *testing.T) {
	nm := NewNorthboundManager(model.NorthboundConfig{
		MQTT: []model.MQTTConfig{{ID: "mqtt-1", Name: "生产 MQTT"}},
		HTTP: []model.HTTPConfig{{ID: "http-1", Name: "云端 HTTP"}},
	}, nil, nil, nil, func(cfg model.NorthboundConfig) error { return nil })

	tests := []struct {
		name      string
		excludeID string
		channel   string
		wantErr   bool
		wantMsg   string
	}{
		{name: "empty name", excludeID: "", channel: "  ", wantErr: true, wantMsg: "不能为空"},
		{name: "duplicate mqtt", excludeID: "", channel: "生产 MQTT", wantErr: true, wantMsg: "已存在"},
		{name: "duplicate http cross protocol", excludeID: "", channel: "云端 HTTP", wantErr: true, wantMsg: "已存在"},
		{name: "case insensitive", excludeID: "", channel: "生产 mqtt", wantErr: true, wantMsg: "已存在"},
		{name: "edit keep same name", excludeID: "mqtt-1", channel: "生产 MQTT", wantErr: false},
		{name: "edit rename to duplicate", excludeID: "mqtt-1", channel: "云端 HTTP", wantErr: true, wantMsg: "已存在"},
		{name: "new unique name", excludeID: "", channel: "测试通道", wantErr: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nm.mu.Lock()
			err := nm.validateNorthboundChannelName(tc.excludeID, tc.channel)
			nm.mu.Unlock()
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if tc.wantMsg != "" && !strings.Contains(err.Error(), tc.wantMsg) {
					t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestNorthboundManager_UpsertMQTT_RejectsDuplicateName(t *testing.T) {
	nm := NewNorthboundManager(model.NorthboundConfig{
		MQTT: []model.MQTTConfig{{ID: "existing", Name: "已有通道", Enable: false}},
	}, nil, nil, nil, func(cfg model.NorthboundConfig) error {
		t.Fatal("saveFunc should not be called on validation failure")
		return nil
	})

	_, err := nm.UpsertMQTTConfig(model.MQTTConfig{ID: "new-id", Name: "已有通道"})
	if err == nil || !strings.Contains(err.Error(), "已存在") {
		t.Fatalf("expected duplicate name error, got %v", err)
	}
	if len(nm.config.MQTT) != 1 {
		t.Fatalf("config should not be modified on validation failure, got %d mqtt entries", len(nm.config.MQTT))
	}
}

func TestNorthboundManager_UpsertHTTP_RejectsDuplicateNameCrossProtocol(t *testing.T) {
	nm := NewNorthboundManager(model.NorthboundConfig{
		MQTT: []model.MQTTConfig{{ID: "mqtt-1", Name: "共享名称", Enable: false}},
	}, nil, nil, nil, func(cfg model.NorthboundConfig) error {
		t.Fatal("saveFunc should not be called on validation failure")
		return nil
	})

	err := nm.UpsertHTTPConfig(model.HTTPConfig{ID: "http-new", Name: "共享名称"})
	if err == nil || !strings.Contains(err.Error(), "已存在") {
		t.Fatalf("expected duplicate name error, got %v", err)
	}
	if len(nm.config.HTTP) != 0 {
		t.Fatalf("config should not be modified on validation failure")
	}
}

func TestValidateNorthboundChannelID(t *testing.T) {
	tests := []struct {
		id      string
		wantErr bool
	}{
		{id: "", wantErr: false},
		{id: "bacnet_1785909123456", wantErr: false},
		{id: "edgeos-mqtt_1785", wantErr: false},
		{id: "ABC123_-xyz", wantErr: false},
		{id: "New BACnet Server", wantErr: true},
		{id: "带中文", wantErr: true},
		{id: "id/with/slash", wantErr: true},
		{id: "a.b", wantErr: true},
		{id: "a@b", wantErr: true},
	}
	for _, tc := range tests {
		err := validateNorthboundChannelID(tc.id)
		if tc.wantErr && err == nil {
			t.Fatalf("id %q: expected error", tc.id)
		}
		if !tc.wantErr && err != nil {
			t.Fatalf("id %q: unexpected error: %v", tc.id, err)
		}
	}
}

func TestNorthboundManager_UpsertBACnetServer_RejectsInvalidID(t *testing.T) {
	nm := NewNorthboundManager(model.NorthboundConfig{}, nil, nil, nil, func(cfg model.NorthboundConfig) error {
		t.Fatal("saveFunc should not be called on validation failure")
		return nil
	})

	_, _, err := nm.UpsertBACnetServerConfig(model.BACnetServerConfig{
		ID: "New BACnet Server", Name: "合法名称", Enable: false,
	})
	if err == nil || !strings.Contains(err.Error(), "格式非法") {
		t.Fatalf("expected invalid ID error, got %v", err)
	}
	if len(nm.config.BACnetServer) != 0 {
		t.Fatalf("config should not be modified on validation failure")
	}
}
