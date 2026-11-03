package baidu_v2

var ModelList = []string{
	// ERNIE 3.5 / 4.0 / 4.5 Turbo / 5.0 were retired on 2026-10-29; 5.1 is the
	// only ERNIE model left on the Qianfan model list.
	"ernie-5.1",
	"deepseek-v4-pro-0813",
	"deepseek-v4-flash-0731",
	"glm-5.3-flash",
	"glm-5.3",
	"glm-5.2-fast",
	"glm-5.2",
	// 图像生成：蒸汽机 Air-Image /v2/musesteamer/images/generations
	"musesteamer-air-image",
	// 图像生成：通用图像生成 /v2/images/generations
	"qwen-image",
	// 图像生成：Ernie-Image-Turbo /v2/ernie-image/images/generations
	"ernie-image-turbo",
	// 图像编辑：通用图像编辑 /v2/images/edits
	"qwen-image-edit",
	// 向量 Embedding /v2/embeddings
	"embedding-v1",
	"tao-8k",
	"bge-large-zh",
	"bge-large-en",
	"qwen3-embedding-0.6b",
	"qwen3-embedding-4b",
	"qwen3-embedding-8b",
	// 重排序 Rerank /v2/rerank
	"qwen3-reranker-0.6b",
	"qwen3-reranker-4b",
	"qwen3-reranker-8b",
}

var ChannelName = "baidu_v2"
