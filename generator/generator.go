package generator

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/ollama"
)

func Generate(prompt, model string) (string, error) {

	if model == "" {
		// default model is llama2
		model = "llama2"
	}

	llm, err := ollama.New(ollama.WithModel(model))
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	completion, err := llm.Call(ctx, prompt,
		llms.WithTemperature(0.8),
		llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
			fmt.Print(string(chunk))
			return nil
		}),
	)
	if err != nil {
		log.Fatal(err)
	}

	// remove the question if it appears in the response
	completion = strings.ReplaceAll(completion, prompt, "")

	if strings.Contains(completion, "```") {
		split := strings.Split(completion, "```")
		if len(split) > 1 {
			return split[1], nil
		}
	}

	return completion, nil
}
