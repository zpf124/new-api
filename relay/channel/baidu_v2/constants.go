package baidu_v2

// ModelList mirrors the Qianfan model list updated 2026-10-29
// (https://cloud.baidu.com/doc/qianfan/s/rmh4stp0j). ERNIE 3.5 / 4.0 /
// 4.5 Turbo / 5.0 were retired on that date, leaving ernie-5.1 as the only
// ERNIE text model.
var ModelList = []string{
	// 文本生成 / 深度思考
	"ernie-5.1",
	"deepseek-v4.1-flash",
	"deepseek-v4.1-flash-1017",
	"deepseek-v4-pro",
	"deepseek-v4-pro-0813",
	"deepseek-v4-flash-0731",
	"qwen3.5-397b-a17b",
	"qwen3.5-122b-a10b",
	"qwen3.5-27b",
	"qwen3.5-35b-a3b",
	"glm-5.3-flash",
	"glm-5.3",
	"glm-5.2",
	"glm-5.1",
	// 视觉理解
	"internvl3-38b",
	// 图像生成：蒸汽机 Air-Image /v2/musesteamer/images/generations
	"musesteamer-air-image",
	// 图像生成：通用图像生成 /v2/images/generations
	"qwen-image",
	// 图像生成：Ernie-Image-Turbo /v2/ernie-image/images/generations
	// 不在模型列表页的图像生成表格里，独立接口文档仍然有效，保留。
	"ernie-image-turbo",
	// 图像编辑：通用图像编辑 /v2/images/edits
	"qwen-image-edit",
	// 文本向量 /v2/embeddings
	"embedding-v1",
	"tao-8k",
	"bge-large-zh",
	"bge-large-en",
	"qwen3-embedding-0.6b",
	"qwen3-embedding-4b",
	"qwen3-embedding-8b",
	// 重排序 /v2/rerank
	"bce-reranker-base",
	"qwen3-reranker-0.6b",
	"qwen3-reranker-4b",
	"qwen3-reranker-8b",
}

var ChannelName = "baidu_v2"
