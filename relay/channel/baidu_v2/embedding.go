package baidu_v2

import (
	"errors"

	"github.com/QuantumNous/new-api/relaykit/dto"
)

// 千帆向量 /v2/embeddings. Qianfan only accepts the documented subset of the
// OpenAI embedding parameters, so the unsupported dto fields (dimensions,
// sampling options) are dropped instead of being forwarded upstream. Input
// may be a string, a string array, or — for gme-Qwen2-VL-2B-Instruct — a
// single-element object array with text/image fields, which passes through
// unchanged.
type qianfanEmbeddingRequest struct {
	Model          string `json:"model"`
	Input          any    `json:"input"`
	User           string `json:"user,omitempty"`
	EncodingFormat string `json:"encoding_format,omitempty"`
}

// buildQianfanEmbeddingRequest converts the shared embedding request. Qianfan
// documents encoding_format as float-only; a base64 request would change the
// response shape upstream, so it is dropped and the float arrays are returned.
func buildQianfanEmbeddingRequest(request dto.EmbeddingRequest) (*qianfanEmbeddingRequest, error) {
	if request.Input == nil {
		return nil, errors.New("input is required")
	}
	if items, ok := request.Input.([]any); ok && len(items) == 0 {
		return nil, errors.New("input must not be an empty list")
	}
	req := &qianfanEmbeddingRequest{
		Model: request.Model,
		Input: request.Input,
		User:  request.User,
	}
	if request.EncodingFormat == "float" {
		req.EncodingFormat = request.EncodingFormat
	}
	return req, nil
}
