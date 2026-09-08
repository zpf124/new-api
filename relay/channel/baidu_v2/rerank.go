package baidu_v2

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/relaykit/dto"
)

// 千帆重排序 /v2/rerank. Qianfan has no return_documents / max_chunk_per_doc /
// overlap_tokens options and always returns the ranked document text, so those
// dto.RerankRequest fields are not forwarded.
type qianfanRerankRequest struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      *int     `json:"top_n,omitempty"`
}

// qianfanMaxRerankDocuments is the documented maximum number of documents on
// /v2/rerank, and therefore the largest meaningful top_n.
const qianfanMaxRerankDocuments = 64

// buildQianfanRerankRequest converts the shared rerank request. Qianfan
// requires documents to be non-empty strings; objects (some rerank clients
// send {"text": ...}) are rejected with a clear error instead of failing
// upstream.
func buildQianfanRerankRequest(request dto.RerankRequest) (*qianfanRerankRequest, error) {
	if request.Query == "" {
		return nil, errors.New("query is required")
	}
	if len(request.Documents) == 0 {
		return nil, errors.New("documents is required")
	}
	if len(request.Documents) > qianfanMaxRerankDocuments {
		return nil, fmt.Errorf("documents must not exceed %d entries", qianfanMaxRerankDocuments)
	}
	if request.TopN != nil && (*request.TopN < 0 || *request.TopN > qianfanMaxRerankDocuments) {
		return nil, fmt.Errorf("top_n must be between 0 and %d", qianfanMaxRerankDocuments)
	}
	documents := make([]string, 0, len(request.Documents))
	for i, item := range request.Documents {
		text, ok := item.(string)
		if !ok || text == "" {
			return nil, fmt.Errorf("documents[%d] must be a non-empty string", i)
		}
		documents = append(documents, text)
	}
	return &qianfanRerankRequest{
		Model:     request.Model,
		Query:     request.Query,
		Documents: documents,
		TopN:      request.TopN,
	}, nil
}
