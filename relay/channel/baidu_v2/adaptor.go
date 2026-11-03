package baidu_v2

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/claude"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertClaudeRequest forwards Anthropic Messages requests unchanged. Qianfan
// terminates the Anthropic protocol itself at /anthropic/v1/messages, so
// converting to OpenAI chat here would drop thinking and tool blocks that the
// native endpoint accepts.
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	if req == nil {
		return nil, errors.New("request is nil")
	}
	adaptor := claude.Adaptor{}
	return adaptor.ConvertClaudeRequest(c, info, req)
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	switch info.RelayMode {
	case constant.RelayModeImagesGenerations:
		switch qianfanImageGenerationAPI(info.UpstreamModelName) {
		case qianfanImageAPIAir:
			return buildQianfanAirImageRequest(request)
		case qianfanImageAPIErnie:
			return buildQianfanErnieImageRequest(request)
		default:
			return buildQianfanGeneralImageRequest(request)
		}
	case constant.RelayModeImagesEdits:
		if c != nil && c.Request != nil && strings.Contains(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
			return buildQianfanImageEditRequestFromForm(c, request)
		}
		return buildQianfanImageEditRequest(request)
	default:
		return nil, fmt.Errorf("unsupported relay mode: %d", info.RelayMode)
	}
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	// Anthropic Messages requests carry no relay mode, so this branch has to
	// come before the mode switch: a /v1/messages request would otherwise fall
	// through to "unsupported relay mode: 0".
	if info.RelayFormat == types.RelayFormatClaude {
		return fmt.Sprintf("%s/anthropic/v1/messages", info.ChannelBaseUrl), nil
	}
	switch info.RelayMode {
	case constant.RelayModeChatCompletions:
		return fmt.Sprintf("%s/v2/chat/completions", info.ChannelBaseUrl), nil
	case constant.RelayModeEmbeddings:
		return fmt.Sprintf("%s/v2/embeddings", info.ChannelBaseUrl), nil
	case constant.RelayModeImagesGenerations:
		switch qianfanImageGenerationAPI(info.UpstreamModelName) {
		case qianfanImageAPIAir:
			return fmt.Sprintf("%s/v2/musesteamer/images/generations", info.ChannelBaseUrl), nil
		case qianfanImageAPIErnie:
			return fmt.Sprintf("%s/v2/ernie-image/images/generations", info.ChannelBaseUrl), nil
		default:
			return fmt.Sprintf("%s/v2/images/generations", info.ChannelBaseUrl), nil
		}
	case constant.RelayModeImagesEdits:
		return fmt.Sprintf("%s/v2/images/edits", info.ChannelBaseUrl), nil
	case constant.RelayModeRerank:
		return fmt.Sprintf("%s/v2/rerank", info.ChannelBaseUrl), nil
	default:
	}
	return "", fmt.Errorf("unsupported relay mode: %d", info.RelayMode)
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	keyParts := strings.Split(info.ApiKey, "|")
	if len(keyParts) == 0 || keyParts[0] == "" {
		return errors.New("invalid API key: authorization token is required")
	}
	if len(keyParts) > 1 {
		if keyParts[1] != "" {
			req.Set("appid", keyParts[1])
		}
	}
	if info.RelayFormat == types.RelayFormatClaude {
		// The Anthropic-compatible endpoint authenticates with x-api-key
		// rather than a Bearer token.
		req.Set("x-api-key", keyParts[0])
		anthropicVersion := ""
		if c != nil && c.Request != nil {
			anthropicVersion = c.Request.Header.Get("anthropic-version")
		}
		if anthropicVersion == "" {
			anthropicVersion = "2023-06-01"
		}
		req.Set("anthropic-version", anthropicVersion)
		claude.CommonClaudeHeadersOperation(c, req, info)
		return nil
	}
	req.Set("Authorization", "Bearer "+keyParts[0])
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	if strings.HasSuffix(info.UpstreamModelName, "-search") {
		info.UpstreamModelName = strings.TrimSuffix(info.UpstreamModelName, "-search")
		request.Model = info.UpstreamModelName
		if len(request.WebSearch) == 0 {
			toMap := request.ToMap()
			toMap["web_search"] = map[string]any{
				"enable":          true,
				"enable_citation": true,
				"enable_trace":    true,
				"enable_status":   false,
			}
			return toMap, nil
		}
		return request, nil
	}
	return request, nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return buildQianfanRerankRequest(request)
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return buildQianfanEmbeddingRequest(request)
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if info.RelayFormat == types.RelayFormatClaude {
		adaptor := claude.Adaptor{}
		return adaptor.DoResponse(c, resp, info)
	}
	adaptor := openai.Adaptor{}
	return adaptor.DoResponse(c, resp, info)
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
