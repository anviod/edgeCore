package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anviod/edgex/internal/ai_agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func aiAuthRequest(method, path string, body []byte) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+GenerateTestToken())
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func TestCreateAiTask_ProtocolReverse(t *testing.T) {
	srv := newChannelTestServer(t)

	payload, err := json.Marshal(map[string]any{
		"skill":       "protocol-reverse",
		"protocol_id": "modbus-tcp",
		"filename":    "capture.pcap",
		"observations": []map[string]any{
			{"label": "Uab", "value": 220.5, "unit": "V"},
		},
	})
	require.NoError(t, err)

	resp, err := srv.app.Test(aiAuthRequest(http.MethodPost, "/api/ai/tasks", payload), -1)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "0", body["code"])
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	taskID, _ := data["id"].(string)
	assert.NotEmpty(t, taskID)
	assert.Equal(t, "queued", data["status"])

	// Poll until waiting_confirm or timeout
	deadline := time.Now().Add(5 * time.Second)
	var final map[string]any
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		getResp, err := srv.app.Test(aiAuthRequest(http.MethodGet, "/api/ai/tasks/"+taskID, nil), -1)
		require.NoError(t, err)
		var getBody map[string]any
		require.NoError(t, json.NewDecoder(getResp.Body).Decode(&getBody))
		getResp.Body.Close()
		final, _ = getBody["data"].(map[string]any)
		if status, _ := final["status"].(string); status == "waiting_confirm" {
			break
		}
	}
	require.NotNil(t, final)
	assert.Equal(t, "waiting_confirm", final["status"])

	deliverables, ok := final["deliverables"].(map[string]any)
	require.True(t, ok)
	assert.NotNil(t, deliverables["protocol_model"])
	assert.NotNil(t, deliverables["point_definition"])
	assert.NotNil(t, deliverables["driver_parameter"])
	assert.NotNil(t, deliverables["validation_case"])
}

func TestPostAiValidate(t *testing.T) {
	srv := newChannelTestServer(t)

	deliverables := map[string]any{
		"protocol_model": map[string]any{
			"protocol_id": "modbus-tcp",
			"confidence":  0.95,
		},
		"point_definition": map[string]any{
			"skill":       "protocol-reverse",
			"protocol_id": "modbus-tcp",
			"points": []map[string]any{
				{
					"id": "uab", "name": "Uab", "address": "40001",
					"datatype": "float32", "scale": 0.1, "confidence": 0.87,
				},
			},
		},
		"driver_parameter": map[string]any{
			"protocol_id": "modbus-tcp",
			"name":        "test-ch",
			"connection":  map[string]any{"ip": "192.168.1.1"},
		},
		"validation_case": map[string]any{
			"validation_cases": []map[string]any{
				{"point_id": "uab", "expected_value": 220.5, "tolerance_pct": 0.5,
					"frame_evidence": map[string]any{"raw_hex": "43DC6666"}},
			},
		},
	}
	payload, err := json.Marshal(map[string]any{"deliverables": deliverables})
	require.NoError(t, err)

	resp, err := srv.app.Test(aiAuthRequest(http.MethodPost, "/api/ai/validate", payload), -1)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	assert.Greater(t, data["pass_rate"], float64(80))
}

func TestConfirmAiTask(t *testing.T) {
	srv := newChannelTestServer(t)

	createPayload, _ := json.Marshal(map[string]any{"skill": string(ai_agent.SkillProtocolReverse)})
	createResp, err := srv.app.Test(aiAuthRequest(http.MethodPost, "/api/ai/tasks", createPayload), -1)
	require.NoError(t, err)
	var createBody map[string]any
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&createBody))
	createResp.Body.Close()
	taskID := createBody["data"].(map[string]any)["id"].(string)

	time.Sleep(3 * time.Second)

	confirmPayload, _ := json.Marshal(map[string]any{"apply_mode": "preview"})
	confirmResp, err := srv.app.Test(
		aiAuthRequest(http.MethodPost, "/api/ai/tasks/"+taskID+"/confirm", confirmPayload), -1)
	require.NoError(t, err)
	defer confirmResp.Body.Close()
	assert.Equal(t, http.StatusOK, confirmResp.StatusCode)

	var confirmBody map[string]any
	require.NoError(t, json.NewDecoder(confirmResp.Body).Decode(&confirmBody))
	data := confirmBody["data"].(map[string]any)
	assert.Equal(t, "applied", data["status"])
}

func TestGetAiQuota(t *testing.T) {
	srv := newChannelTestServer(t)
	resp, err := srv.app.Test(aiAuthRequest(http.MethodGet, "/api/ai/quota", nil), -1)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data := body["data"].(map[string]any)
	assert.Equal(t, "remote", data["mode"])
	assert.NotZero(t, data["tokens_limit"])
}

func TestGetAiDiagnosticsSummary(t *testing.T) {
	srv := newChannelTestServer(t)
	resp, err := srv.app.Test(aiAuthRequest(http.MethodGet, "/api/ai/diagnostics/summary", nil), -1)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data := body["data"].(map[string]any)
	steps, ok := data["steps"].([]any)
	require.True(t, ok)
	assert.NotEmpty(t, steps)
}

func TestPostAiEdgeRuleDraft(t *testing.T) {
	srv := newChannelTestServer(t)
	payload, _ := json.Marshal(map[string]any{"description": "冷机出水温度超过12度报警"})
	resp, err := srv.app.Test(aiAuthRequest(http.MethodPost, "/api/ai/edge-rule/draft", payload), -1)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data := body["data"].(map[string]any)
	assert.NotNil(t, data["draft"])
}

func TestPostAiTaskFromUpload(t *testing.T) {
	srv := newChannelTestServer(t)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "capture.pcap")
	require.NoError(t, err)
	_, err = part.Write([]byte("fake pcap content"))
	require.NoError(t, err)
	require.NoError(t, writer.WriteField("protocol_id", "modbus-tcp"))
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/api/ai/tasks/upload", &body)
	req.Header.Set("Authorization", "Bearer "+GenerateTestToken())
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := srv.app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var respBody map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&respBody))
	assert.Equal(t, "0", respBody["code"])
	data, ok := respBody["data"].(map[string]any)
	require.True(t, ok)
	taskID, _ := data["id"].(string)
	assert.NotEmpty(t, taskID)
	assert.Equal(t, "queued", data["status"])

	inputFiles, ok := data["input_files"].([]any)
	require.True(t, ok)
	require.Len(t, inputFiles, 1)
	assert.Equal(t, "capture.pcap", inputFiles[0])

	uploadDir := filepath.Join(os.TempDir(), "edgex-ai-uploads", taskID)
	_, err = os.Stat(filepath.Join(uploadDir, "capture.pcap"))
	assert.NoError(t, err, "uploaded file should be stored under task ID directory")
}

func TestPostAiTaskFromUpload_InvalidType(t *testing.T) {
	srv := newChannelTestServer(t)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "notes.txt")
	require.NoError(t, err)
	_, err = part.Write([]byte("not allowed"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/api/ai/tasks/upload", &body)
	req.Header.Set("Authorization", "Bearer "+GenerateTestToken())
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := srv.app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestListAiTasks(t *testing.T) {
	srv := newChannelTestServer(t)

	createPayload, _ := json.Marshal(map[string]any{"skill": string(ai_agent.SkillProtocolReverse)})
	createResp, err := srv.app.Test(aiAuthRequest(http.MethodPost, "/api/ai/tasks", createPayload), -1)
	require.NoError(t, err)
	createResp.Body.Close()

	resp, err := srv.app.Test(aiAuthRequest(http.MethodGet, "/api/ai/tasks", nil), -1)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data, ok := body["data"].([]any)
	require.True(t, ok)
	assert.NotEmpty(t, data)
}
