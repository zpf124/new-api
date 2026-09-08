package baidu_v2

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/gin-gonic/gin"
)

// qianfanMaxSeed is the documented inclusive upper bound of the seed parameter
// on every qianfan image API ([0, 4294967295]).
const qianfanMaxSeed = uint64(4294967295)

// Documented qianfan parameter bounds, enforced locally so invalid values fail
// with a clear gateway error instead of reaching the upstream error path.
const (
	// qianfanMaxImageN is the inclusive upper bound of n on the image generation
	// and edit APIs ([1, 4]); qwen-image and qwen-image-edit only accept 1. This
	// is the upstream API limit, distinct from dto.MaxImageN (128) which is only
	// a billing safety ceiling.
	qianfanMaxImageN = 4
	// qianfanMaxImageSteps is the inclusive upper bound of steps ([1, 50]).
	qianfanMaxImageSteps = 50
	// qianfanMaxGeneralGuidance caps guidance on the general generation API
	// ([0, 30] for flux.1-schnell, [0, 20] for qwen-image).
	qianfanMaxGeneralGuidance = 30.0
	// qianfanMaxEditGuidance caps guidance on the edit API ([0, 20]).
	qianfanMaxEditGuidance = 20.0
	// qianfanMaxEditImageBytes is the documented 10MB file limit of the edit API,
	// enforced before the whole upload is read into memory.
	qianfanMaxEditImageBytes = 10 * 1024 * 1024
)

// Qianfan exposes three image generation APIs with different endpoint paths
// and parameter sets, and distinguishes them by model name. The adaptor routes
// with the same mapping.
type qianfanImageAPIKind int

const (
	qianfanImageAPIGeneral qianfanImageAPIKind = iota
	qianfanImageAPIErnie
	qianfanImageAPIAir
)

// qianfanImageGenerationAPI classifies a generation model into one of the
// documented qianfan image generation APIs:
//   - 蒸汽机 Air-Image /v2/musesteamer/images/generations (musesteamer-air-image)
//   - Ernie-Image-Turbo /v2/ernie-image/images/generations (ernie-image-turbo)
//   - 通用图像生成 /v2/images/generations (qwen-image and platform models)
func qianfanImageGenerationAPI(model string) qianfanImageAPIKind {
	switch model {
	case "musesteamer-air-image":
		return qianfanImageAPIAir
	case "ernie-image-turbo":
		return qianfanImageAPIErnie
	default:
		return qianfanImageAPIGeneral
	}
}

// 通用图像生成 /v2/images/generations
type qianfanImageGenerationRequest struct {
	Model          string   `json:"model"`
	Prompt         string   `json:"prompt"`
	NegativePrompt string   `json:"negative_prompt,omitempty"`
	User           string   `json:"user,omitempty"`
	N              *int     `json:"n,omitempty"`
	Size           string   `json:"size,omitempty"`
	Steps          *int     `json:"steps,omitempty"`
	Seed           *uint64  `json:"seed,omitempty"`
	Guidance       *float64 `json:"guidance,omitempty"`
	PromptExtend   *bool    `json:"prompt_extend,omitempty"`
	Watermark      *bool    `json:"watermark,omitempty"`
}

// Ernie-Image-Turbo /v2/ernie-image/images/generations
type qianfanErnieImageRequest struct {
	Model     string `json:"model"`
	Prompt    string `json:"prompt"`
	Size      string `json:"size,omitempty"`
	Watermark *bool  `json:"watermark,omitempty"`
}

// 蒸汽机 Air-Image /v2/musesteamer/images/generations
type qianfanAirImageRequest struct {
	Model          string  `json:"model"`
	Prompt         string  `json:"prompt"`
	Size           string  `json:"size,omitempty"`
	Seed           *uint64 `json:"seed,omitempty"`
	PromptExtend   *bool   `json:"prompt_extend,omitempty"`
	ResponseFormat string  `json:"response_format,omitempty"`
}

// 通用图像编辑 /v2/images/edits. Image holds either a single reference string
// (URL or base64 data URL) or an array of them for multi-image edits.
type qianfanImageEditRequest struct {
	Model          string   `json:"model"`
	Image          any      `json:"image"`
	Prompt         string   `json:"prompt"`
	NegativePrompt string   `json:"negative_prompt,omitempty"`
	Size           string   `json:"size,omitempty"`
	Seed           *uint64  `json:"seed,omitempty"`
	Guidance       *float64 `json:"guidance,omitempty"`
	N              *int     `json:"n,omitempty"`
	PromptExtend   *bool    `json:"prompt_extend,omitempty"`
	Watermark      *bool    `json:"watermark,omitempty"`
}

// qianfanImageExtras carries the qianfan-specific image parameters that the
// shared dto.ImageRequest collects into Extra because they have no OpenAI
// image field. Unknown extras are dropped so unrelated client parameters do
// not leak into the upstream request.
type qianfanImageExtras struct {
	NegativePrompt string
	Steps          *int
	Seed           *uint64
	Guidance       *float64
	PromptExtend   *bool
}

func parseQianfanImageExtras(request dto.ImageRequest) (*qianfanImageExtras, error) {
	extras := &qianfanImageExtras{}
	for key, raw := range request.Extra {
		if len(raw) == 0 {
			continue
		}
		var err error
		switch key {
		case "negative_prompt":
			err = common.Unmarshal(raw, &extras.NegativePrompt)
		case "steps":
			err = common.Unmarshal(raw, &extras.Steps)
		case "seed":
			extras.Seed, err = parseQianfanSeed(raw)
		case "guidance":
			err = common.Unmarshal(raw, &extras.Guidance)
		case "prompt_extend":
			err = common.Unmarshal(raw, &extras.PromptExtend)
		}
		if err != nil {
			return nil, fmt.Errorf("invalid %s: %w", key, err)
		}
	}
	return extras, nil
}

func parseQianfanSeed(raw json.RawMessage) (*uint64, error) {
	var seed uint64
	if err := common.Unmarshal(raw, &seed); err != nil {
		return nil, errors.New("seed must be an integer")
	}
	if seed > qianfanMaxSeed {
		return nil, fmt.Errorf("seed must be an integer between 0 and %d", qianfanMaxSeed)
	}
	return &seed, nil
}

// qianfanImageN re-bounds the generation count locally: dto.ImageRequest.N is
// already capped by the shared validator, but the adaptor must not rely on
// every call path having passed through it, and the upstream API only accepts
// up to qianfanMaxImageN.
func qianfanImageN(n *uint) (*int, error) {
	if n == nil {
		return nil, nil
	}
	if *n < 1 || *n > qianfanMaxImageN {
		return nil, fmt.Errorf("n must be an integer between 1 and %d", qianfanMaxImageN)
	}
	value := int(*n)
	return &value, nil
}

func buildQianfanGeneralImageRequest(request dto.ImageRequest) (*qianfanImageGenerationRequest, error) {
	n, err := qianfanImageN(request.N)
	if err != nil {
		return nil, err
	}
	extras, err := parseQianfanImageExtras(request)
	if err != nil {
		return nil, err
	}
	if extras.Steps != nil && (*extras.Steps < 1 || *extras.Steps > qianfanMaxImageSteps) {
		return nil, fmt.Errorf("steps must be an integer between 1 and %d", qianfanMaxImageSteps)
	}
	if extras.Guidance != nil && (*extras.Guidance < 0 || *extras.Guidance > qianfanMaxGeneralGuidance) {
		return nil, fmt.Errorf("guidance must be a number between 0 and %v", qianfanMaxGeneralGuidance)
	}
	user := ""
	if len(request.User) > 0 {
		if err := common.Unmarshal(request.User, &user); err != nil {
			return nil, errors.New("user must be a string")
		}
	}
	return &qianfanImageGenerationRequest{
		Model:          request.Model,
		Prompt:         request.Prompt,
		NegativePrompt: extras.NegativePrompt,
		User:           user,
		N:              n,
		Size:           request.Size,
		Steps:          extras.Steps,
		Seed:           extras.Seed,
		Guidance:       extras.Guidance,
		PromptExtend:   extras.PromptExtend,
		Watermark:      request.Watermark,
	}, nil
}

func buildQianfanErnieImageRequest(request dto.ImageRequest) (*qianfanErnieImageRequest, error) {
	// The ernie-image-turbo API has no n parameter and always returns a single
	// image; billing keys off request.N, so a higher n would charge for images
	// that are never generated.
	if request.N != nil && *request.N > 1 {
		return nil, errors.New("n must be 1: the ernie-image-turbo API returns a single image")
	}
	return &qianfanErnieImageRequest{
		Model:     request.Model,
		Prompt:    request.Prompt,
		Size:      request.Size,
		Watermark: request.Watermark,
	}, nil
}

func buildQianfanAirImageRequest(request dto.ImageRequest) (*qianfanAirImageRequest, error) {
	// The musesteamer-air-image API has no n parameter and always returns a
	// single image; billing keys off request.N, so a higher n would charge for
	// images that are never generated.
	if request.N != nil && *request.N > 1 {
		return nil, errors.New("n must be 1: the musesteamer-air-image API returns a single image")
	}
	req := &qianfanAirImageRequest{
		Model:          request.Model,
		Prompt:         request.Prompt,
		Size:           request.Size,
		ResponseFormat: request.ResponseFormat,
	}
	if raw, ok := request.Extra["seed"]; ok && len(raw) > 0 {
		seed, err := parseQianfanSeed(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid seed: %w", err)
		}
		req.Seed = seed
	}
	if raw, ok := request.Extra["prompt_extend"]; ok && len(raw) > 0 {
		if err := common.Unmarshal(raw, &req.PromptExtend); err != nil {
			return nil, errors.New("prompt_extend must be a boolean")
		}
	}
	return req, nil
}

// buildQianfanImageEditRequest converts a JSON image edit request. The qianfan
// edit API takes the image as a URL/base64 reference inside the JSON body, so
// an OpenAI-style multipart edit has to be rebuilt as JSON (see
// buildQianfanImageEditRequestFromForm).
func buildQianfanImageEditRequest(request dto.ImageRequest) (*qianfanImageEditRequest, error) {
	imageRefs, err := qianfanEditImageRefs(request.Image)
	if err != nil {
		return nil, err
	}
	n, err := qianfanImageN(request.N)
	if err != nil {
		return nil, err
	}
	extras, err := parseQianfanImageExtras(request)
	if err != nil {
		return nil, err
	}
	if extras.Guidance != nil && (*extras.Guidance < 0 || *extras.Guidance > qianfanMaxEditGuidance) {
		return nil, fmt.Errorf("guidance must be a number between 0 and %v", qianfanMaxEditGuidance)
	}
	return &qianfanImageEditRequest{
		Model:          request.Model,
		Image:          imageRefs,
		Prompt:         request.Prompt,
		NegativePrompt: extras.NegativePrompt,
		Size:           request.Size,
		Seed:           extras.Seed,
		Guidance:       extras.Guidance,
		N:              n,
		PromptExtend:   extras.PromptExtend,
		Watermark:      request.Watermark,
	}, nil
}

// qianfanEditImageRefs validates the image reference of an edit request: a
// single URL/base64 string, or an array of them (qwen-image-edit multi-image).
func qianfanEditImageRefs(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, errors.New("image is required")
	}
	var single string
	if err := common.Unmarshal(raw, &single); err == nil {
		if single == "" {
			return nil, errors.New("image is required")
		}
		return single, nil
	}
	var multiple []string
	if err := common.Unmarshal(raw, &multiple); err != nil || len(multiple) == 0 {
		return nil, errors.New("image must be a URL, a base64 data URL, or an array of them")
	}
	return multiple, nil
}

// buildQianfanImageEditRequestFromForm converts an OpenAI-style multipart
// image edit request into the qianfan JSON format: uploaded image files
// become base64 data URLs, and plain form values are kept as references. The
// shared validator already parsed and model-mapped the common OpenAI fields,
// so only the qianfan-specific parameters are read from the form here.
func buildQianfanImageEditRequestFromForm(c *gin.Context, request dto.ImageRequest) (*qianfanImageEditRequest, error) {
	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return nil, fmt.Errorf("failed to parse multipart form: %w", err)
	}
	values := url.Values(form.Value)

	n, err := qianfanImageN(request.N)
	if err != nil {
		return nil, err
	}
	edit := &qianfanImageEditRequest{
		Model:          request.Model,
		Prompt:         request.Prompt,
		NegativePrompt: values.Get("negative_prompt"),
		Size:           request.Size,
		N:              n,
		Watermark:      request.Watermark,
	}
	if seedValue := strings.TrimSpace(values.Get("seed")); seedValue != "" {
		seed, parseErr := strconv.ParseUint(seedValue, 10, 64)
		if parseErr != nil || seed > qianfanMaxSeed {
			return nil, fmt.Errorf("seed must be an integer between 0 and %d", qianfanMaxSeed)
		}
		edit.Seed = &seed
	}
	if guidanceValue := strings.TrimSpace(values.Get("guidance")); guidanceValue != "" {
		guidance, parseErr := strconv.ParseFloat(guidanceValue, 64)
		if parseErr != nil {
			return nil, errors.New("guidance must be a number")
		}
		if guidance < 0 || guidance > qianfanMaxEditGuidance {
			return nil, fmt.Errorf("guidance must be a number between 0 and %v", qianfanMaxEditGuidance)
		}
		edit.Guidance = &guidance
	}
	if values.Has("prompt_extend") {
		promptExtend, parseErr := strconv.ParseBool(values.Get("prompt_extend"))
		if parseErr != nil {
			return nil, errors.New("prompt_extend must be a boolean")
		}
		edit.PromptExtend = &promptExtend
	}

	// Client image fields follow the OpenAI edit conventions: "image",
	// "image[]" or any "image[...]" variant may carry one or more files.
	var imageHeaders []*multipart.FileHeader
	if files := form.File["image"]; len(files) > 0 {
		imageHeaders = files
	} else if files := form.File["image[]"]; len(files) > 0 {
		imageHeaders = files
	} else {
		for fieldName, files := range form.File {
			if strings.HasPrefix(fieldName, "image[") && len(files) > 0 {
				imageHeaders = append(imageHeaders, files...)
			}
		}
	}
	var imageRefs []string
	for _, fileHeader := range imageHeaders {
		dataURL, fileErr := qianfanImageDataURL(fileHeader)
		if fileErr != nil {
			return nil, fileErr
		}
		imageRefs = append(imageRefs, dataURL)
	}
	if len(imageRefs) == 0 {
		// No uploaded files: accept the image as URL form values instead.
		imageRefs = values["image"]
	}
	switch {
	case len(imageRefs) == 1:
		edit.Image = imageRefs[0]
	case len(imageRefs) > 1:
		edit.Image = imageRefs
	default:
		return nil, errors.New("image is required")
	}
	return edit, nil
}

func qianfanImageDataURL(fileHeader *multipart.FileHeader) (string, error) {
	if fileHeader.Size > qianfanMaxEditImageBytes {
		return "", fmt.Errorf("image file %s exceeds the 10MB limit", fileHeader.Filename)
	}
	file, err := fileHeader.Open()
	if err != nil {
		return "", fmt.Errorf("failed to open image file %s: %w", fileHeader.Filename, err)
	}
	data, err := io.ReadAll(file)
	_ = file.Close()
	if err != nil {
		return "", fmt.Errorf("failed to read image file %s: %w", fileHeader.Filename, err)
	}
	mimeType := http.DetectContentType(data)
	if !strings.HasPrefix(mimeType, "image/") {
		return "", fmt.Errorf("file %s is not a supported image format", fileHeader.Filename)
	}
	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data)), nil
}
