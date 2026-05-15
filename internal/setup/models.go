package setup

type ModelOption struct {
	RepoID string
	Params string
	Desc   string
}

var AvailableModels = []ModelOption{
	{RepoID: "unsloth/Qwen3-VL-4B-Instruct-GGUF", Params: "4B", Desc: "VL 4B — buena calidad y velocidad, recomendado"},
	{RepoID: "unsloth/Qwen3.5-4B-GGUF", Params: "4B", Desc: "Qwen3.5 4B — probado, buena velocidad"},
	{RepoID: "unsloth/Qwen3-VL-2B-Instruct-1M-GGUF", Params: "2B", Desc: "VL 2B 1M context — más rápido, menos VRAM"},
}

func DefaultModelIndex() int {
	for i, m := range AvailableModels {
		if m.RepoID == "unsloth/Qwen3-VL-4B-Instruct-GGUF" {
			return i
		}
	}
	return 0
}
