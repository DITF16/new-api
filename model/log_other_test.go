package model

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestBodyLogOptInAndVisibility(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			payload := `{"messages":[{"role":"user","content":"排错 <script>"}],"stream":true}`
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(payload))
			storage, err := common.GetBodyStorage(c)
			require.NoError(t, err)
			t.Cleanup(func() { common.CleanupBodyStorage(c) })
			_, err = storage.Seek(5, io.SeekStart)
			require.NoError(t, err)
			common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{RecordRequestBody: enabled})
			other := NewLogOther().withRequestBody(c)
			position, err := storage.Seek(0, io.SeekCurrent)
			require.NoError(t, err)
			assert.EqualValues(t, 5, position)
			if !enabled {
				assert.NotContains(t, other.JSONString(), "request_body")
				return
			}
			var decoded map[string]any
			require.NoError(t, common.UnmarshalJsonStr(formatLogOtherJSON(other.JSONString(), logOtherVisibilityAdmin), &decoded))
			admin := decoded["admin_info"].(map[string]any)
			assert.Equal(t, payload, admin["request_body"])
			assert.Equal(t, false, admin["request_body_truncated"])
			assert.NotContains(t, formatLogOtherJSON(other.JSONString(), logOtherVisibilityUser), "request_body")
		})
	}
}

func TestRequestBodyLogTruncatesLargePayloadAndHandlesMissingBody(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{RecordRequestBody: true})
	var empty *LogOther
	assert.NotContains(t, empty.withRequestBody(c).JSONString(), "request_body")
	payload := strings.Repeat("x", (1<<20)-1) + "中文"
	storage, err := common.CreateBodyStorage([]byte(payload))
	require.NoError(t, err)
	c.Set(common.KeyBodyStorage, storage)
	t.Cleanup(func() { common.CleanupBodyStorage(c) })
	other := empty.withRequestBody(c)
	admin := other.Snapshot()["admin_info"].(map[string]any)
	assert.Equal(t, strings.Repeat("x", (1<<20)-1), admin["request_body"])
	assert.Equal(t, true, admin["request_body_truncated"])
}

func TestLogOtherScopesAndMerges(t *testing.T) {
	var other LogOther

	assert.True(t, other.SetPublic("request_path", "/v1/chat/completions"))
	other.MergePublic(map[string]any{
		"zero": 0,
	})
	assert.True(t, other.SetAdmin("use_channel", []string{"channel-a"}))
	other.MergeAdmin(map[string]any{
		"rejected": false,
	})
	assert.True(t, other.SetRoot("upstream_request_id", "upstream-private"))
	other.MergeRoot(map[string]any{
		"generation": 0,
	})
	assert.True(t, other.SetAudit("method", "POST"))
	other.MergeAudit(map[string]any{
		"success": false,
	})

	require.JSONEq(t, `{
		"request_path": "/v1/chat/completions",
		"zero": 0,
		"admin_info": {
			"use_channel": ["channel-a"],
			"rejected": false
		},
		"root_info": {
			"upstream_request_id": "upstream-private",
			"generation": 0
		},
		"audit_info": {
			"method": "POST",
			"success": false
		}
	}`, other.JSONString())
}

func TestLogOtherRejectsSensitivePublicFields(t *testing.T) {
	other := NewLogOther()

	for _, key := range []string{
		"admin_info",
		"root_info",
		"audit_info",
		"channel_id",
		"channel_name",
		"channel_type",
		"reject_reason",
	} {
		assert.False(t, other.SetPublic(key, "must-not-leak"), key)
	}
	other.MergePublic(map[string]any{
		"request_path": "/v1/responses",
		"channel_name": "still-must-not-leak",
		"admin_info":   map[string]any{"secret": true},
	})

	require.JSONEq(t, `{"request_path":"/v1/responses"}`, other.JSONString())
	require.JSONEq(t, `{}`, NewLogOther().JSONString())
}

func TestLogOtherJSONStringDoesNotMutateReceiver(t *testing.T) {
	other := NewLogOther()
	require.True(t, other.SetPublic("request_path", "/v1/chat/completions"))
	require.True(t, other.SetAdmin("rejected", false))

	before := other.Snapshot()
	first := other.JSONString()
	after := other.Snapshot()
	second := other.JSONString()

	require.Equal(t, before, after)
	require.Equal(t, first, second)
}
