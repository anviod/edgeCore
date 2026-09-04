package modbus

import (
	"net/url"
	"testing"
)

// TestRTUURLStripQuery 验证带查询参数的 RTU URL 能被正确解析、剥离并保留设备路径。
// 回归背景：connectOnce 曾把串口参数拼进 URL 查询串，而 modbus 库不解析查询参数，
// 导致设备路径变成 "/dev/ttyS3?baudrate=4800&..."，串口永远打不开（通道 offline）。
func TestRTUURLStripQuery(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		wantPath  string
		wantQuery map[string]string
	}{
		{
			name:     "legacy query params",
			raw:      "rtu:///dev/ttyS3?baudrate=4800&data_bits=8&parity=N&stop_bits=1",
			wantPath: "rtu:///dev/ttyS3",
			wantQuery: map[string]string{
				"baudrate":  "4800",
				"data_bits": "8",
				"parity":    "N",
				"stop_bits": "1",
			},
		},
		{
			name:     "bare rtu url",
			raw:      "rtu:///dev/ttyUSB0",
			wantPath: "rtu:///dev/ttyUSB0",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u, err := url.Parse(tc.raw)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			if u.RawQuery != "" {
				q := u.Query()
				for k, want := range tc.wantQuery {
					if got := q.Get(k); got != want {
						t.Errorf("query[%s] = %q, want %q", k, got, want)
					}
				}
				u.RawQuery = ""
			}
			if got := u.String(); got != tc.wantPath {
				t.Errorf("stripped URL = %q, want %q", got, tc.wantPath)
			}
		})
	}
}

func TestParseParity(t *testing.T) {
	cases := map[string]uint{
		"N": 0, "n": 0, "none": 0, "": 0, "x": 0,
		"E": 1, "e": 1, "even": 1, "EVEN": 1,
		"O": 2, "o": 2, "odd": 2, "ODD": 2,
	}
	for in, want := range cases {
		if got := parseParity(in); got != want {
			t.Errorf("parseParity(%q) = %d, want %d", in, got, want)
		}
	}
}
