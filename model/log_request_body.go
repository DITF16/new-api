package model

import (
	"io"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

// withRequestBody snapshots the cached client payload before request cleanup.
// An independent reader preserves in-flight relay/retry cursors. Keep payloads
// admin-only and bounded, including for disk-backed uploads.
func (o *LogOther) withRequestBody(c *gin.Context) *LogOther {
	if c == nil {
		return o
	}
	settings, _ := common.GetContextKeyType[dto.ChannelSettings](c, constant.ContextKeyChannelSetting)
	if !settings.RecordRequestBody {
		return o
	}
	if o == nil {
		o = NewLogOther()
	}
	// Never consume an uncached body: some relay paths may still be reading it.
	value, exists := c.Get(common.KeyBodyStorage)
	storage, ok := value.(common.BodyStorage)
	if !exists || !ok || storage == nil {
		return o
	}
	reader, err := storage.NewReader()
	if err != nil {
		return o
	}
	defer reader.Close()
	const maxLoggedRequestBody = 1 << 20
	body, err := io.ReadAll(io.LimitReader(reader, maxLoggedRequestBody+1))
	if err != nil {
		return o
	}
	truncated := len(body) > maxLoggedRequestBody
	if truncated {
		body = body[:maxLoggedRequestBody]
	}
	o.SetAdmin("request_body", strings.ToValidUTF8(string(body), ""))
	o.SetAdmin("request_body_truncated", truncated)
	return o
}
