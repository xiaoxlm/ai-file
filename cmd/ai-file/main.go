package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/xiaoxlm/ai-file/internal/app"
	"github.com/xiaoxlm/ai-file/internal/config"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
) int {
	flags := flag.NewFlagSet("ai-file", flag.ContinueOnError)
	flags.SetOutput(stderr)
	provider := flags.String("provider", "", "LLM provider: deepseek, openai, custom")
	baseURL := flags.String("base-url", "", "OpenAI-compatible API base URL")
	model := flags.String("model", "", "LLM model name")
	verbose := flags.Bool("verbose", false, "print agent steps to stderr")
	outPath := flags.String("out", "", "also write the complete result to this file")
	flags.Usage = func() {
		fmt.Fprintln(
			stderr,
			"用法: ai-file [-provider value] [-base-url value] [-model value] [-verbose] [-out path] <文件路径>",
		)
		flags.PrintDefaults()
	}

	if err := flags.Parse(args); err != nil {
		return int(app.ExitUsage)
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return int(app.ExitUsage)
	}

	loadOptions := config.LoadOptions{}
	flags.Visit(func(current *flag.Flag) {
		switch current.Name {
		case "provider":
			loadOptions.Provider = provider
		case "base-url":
			loadOptions.BaseURL = baseURL
		case "model":
			loadOptions.Model = model
		case "verbose":
			loadOptions.Verbose = verbose
		}
	})
	cfg, err := config.Load(loadOptions)
	if err != nil {
		fmt.Fprintln(stderr, "错误:", err)
		return int(app.ExitUsage)
	}

	return int(app.Run(ctx, app.Options{
		Path:    flags.Arg(0),
		OutPath: *outPath,
		Config:  cfg,
		Stdout:  stdout,
		Stderr:  stderr,
	}))
}
