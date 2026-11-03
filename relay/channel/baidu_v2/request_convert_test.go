package baidu_v2

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildQianfanEmbeddingRequest locks the /v2/embeddings wire contract:
// only the documented qianfan parameters are forwarded, sampling options and
// dimensions from dto.EmbeddingRequest never reach upstream.
func TestBuildQianfanEmbeddingRequest(t *testing.T) {
	t.Run("list input keeps float encoding_format and drops unsupported fields", func(t *testing.T) {
		request := dto.EmbeddingRequest{
			Model:            "tao-8k",
			Input:            []any{"hello", "world"},
			EncodingFormat:   "float",
			Dimensions:       lo.ToPtr(1024),
			User:             "user-1",
			Seed:             lo.ToPtr(0.5),
			Temperature:      lo.ToPtr(0.7),
			TopP:             lo.ToPtr(0.9),
			FrequencyPenalty: lo.ToPtr(0.1),
			PresencePenalty:  lo.ToPtr(0.2),
		}
		converted, err := buildQianfanEmbeddingRequest(request)
		require.NoError(t, err)
		body, err := common.Marshal(converted)
		require.NoError(t, err)
		assert.JSONEq(t, `{"model":"tao-8k","input":["hello","world"],"user":"user-1","encoding_format":"float"}`, string(body))
	})

	t.Run("string input with base64 encoding_format falls back to floats", func(t *testing.T) {
		request := dto.EmbeddingRequest{
			Model:          "bge-large-zh",
			Input:          "hello",
			EncodingFormat: "base64",
		}
		converted, err := buildQianfanEmbeddingRequest(request)
		require.NoError(t, err)
		body, err := common.Marshal(converted)
		require.NoError(t, err)
		assert.JSONEq(t, `{"model":"bge-large-zh","input":"hello"}`, string(body))
	})

	t.Run("multimodal input passes through unchanged", func(t *testing.T) {
		request := dto.EmbeddingRequest{
			Model: "gme-Qwen2-VL-2B-Instruct",
			Input: []any{map[string]any{"text": "describe", "image": "https://example.com/a.jpg"}},
		}
		converted, err := buildQianfanEmbeddingRequest(request)
		require.NoError(t, err)
		body, err := common.Marshal(converted)
		require.NoError(t, err)
		assert.JSONEq(t, `{"model":"gme-Qwen2-VL-2B-Instruct","input":[{"text":"describe","image":"https://example.com/a.jpg"}]}`, string(body))
	})

	t.Run("empty input is rejected", func(t *testing.T) {
		_, err := buildQianfanEmbeddingRequest(dto.EmbeddingRequest{Model: "tao-8k", Input: []any{}})
		require.Error(t, err)
		_, err = buildQianfanEmbeddingRequest(dto.EmbeddingRequest{Model: "tao-8k"})
		require.Error(t, err)
	})
}

// TestBuildQianfanRerankRequest locks the /v2/rerank wire contract: documents
// must be non-empty strings, and the jina/cohere options without a qianfan
// equivalent are not forwarded.
func TestBuildQianfanRerankRequest(t *testing.T) {
	t.Run("converts documents and drops unsupported options", func(t *testing.T) {
		request := dto.RerankRequest{
			Model:           "bce-reranker-base",
			Query:           "query",
			Documents:       []any{"first", "second"},
			TopN:            lo.ToPtr(2),
			ReturnDocuments: lo.ToPtr(true),
			MaxChunkPerDoc:  lo.ToPtr(3),
			OverLapTokens:   lo.ToPtr(4),
		}
		converted, err := buildQianfanRerankRequest(request)
		require.NoError(t, err)
		body, err := common.Marshal(converted)
		require.NoError(t, err)
		assert.JSONEq(t, `{"model":"bce-reranker-base","query":"query","documents":["first","second"],"top_n":2}`, string(body))
	})

	t.Run("omits top_n when absent", func(t *testing.T) {
		request := dto.RerankRequest{
			Model:     "qwen3-reranker-4b",
			Query:     "query",
			Documents: []any{"first"},
		}
		converted, err := buildQianfanRerankRequest(request)
		require.NoError(t, err)
		body, err := common.Marshal(converted)
		require.NoError(t, err)
		assert.JSONEq(t, `{"model":"qwen3-reranker-4b","query":"query","documents":["first"]}`, string(body))
	})

	errorCases := []struct {
		name    string
		request dto.RerankRequest
	}{
		{"empty query", dto.RerankRequest{Model: "bce-reranker-base", Documents: []any{"first"}}},
		{"no documents", dto.RerankRequest{Model: "bce-reranker-base", Query: "query"}},
		{"object document", dto.RerankRequest{Model: "bce-reranker-base", Query: "query", Documents: []any{map[string]any{"text": "first"}}}},
		{"empty document", dto.RerankRequest{Model: "bce-reranker-base", Query: "query", Documents: []any{""}}},
		{"negative top_n", dto.RerankRequest{Model: "bce-reranker-base", Query: "query", Documents: []any{"first"}, TopN: lo.ToPtr(-1)}},
	}
	for _, testCase := range errorCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := buildQianfanRerankRequest(testCase.request)
			require.Error(t, err)
		})
	}
}

// TestGetRequestURLRoutesAnthropicNatively locks the native Anthropic path.
// An Anthropic Messages request carries no relay mode, so routing it by mode
// alone fails with "unsupported relay mode: 0" before any request is sent.
func TestGetRequestURLRoutesAnthropicNatively(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://qianfan.baidubce.com"},
	}

	requestURL, err := (&Adaptor{}).GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://qianfan.baidubce.com/anthropic/v1/messages", requestURL)
}

func TestGetRequestURLKeepsV2PathsForOpenAI(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://qianfan.baidubce.com"},
	}

	requestURL, err := (&Adaptor{}).GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://qianfan.baidubce.com/v2/chat/completions", requestURL)
}

func TestSetupRequestHeaderUsesApiKeyForAnthropic(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "bce-key|app-1"},
	}

	header := http.Header{}
	require.NoError(t, (&Adaptor{}).SetupRequestHeader(c, &header, info))
	assert.Equal(t, "bce-key", header.Get("x-api-key"))
	assert.Equal(t, "2023-06-01", header.Get("anthropic-version"))
	assert.Equal(t, "app-1", header.Get("appid"))
	assert.Empty(t, header.Get("Authorization"))
}

func TestSetupRequestHeaderKeepsBearerForOpenAI(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "bce-key"},
	}

	header := http.Header{}
	require.NoError(t, (&Adaptor{}).SetupRequestHeader(c, &header, info))
	assert.Equal(t, "Bearer bce-key", header.Get("Authorization"))
	assert.Empty(t, header.Get("x-api-key"))
}
