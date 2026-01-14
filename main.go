package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/BurntSushi/toml"
	"github.com/alexflint/go-arg"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type SolutionMeta struct {
	Score   float64 `toml:"score"`
	Comment string  `toml:"comment"`
}

type Solution struct {
	Code string
	Meta SolutionMeta
}

var state struct {
	Args struct {
		ConfigPath string `arg:"-c,--config,required"`
		DataPath   string `arg:"-d,--data,required"`
		Iterations int    `arg:"-i,--iterations,required"`
	}
	Config struct {
		LLM struct {
			Model   string
			BaseURL string `toml:"base_url"`
			APIKey  string `toml:"api_key"`
		}
		Prompt struct {
			System string
			User   string
		}
		Eval struct {
			Command   []string `toml:"command"`
			Extension string
		}
	}
	TempPath  string
	Client    openai.Client
	Solutions map[string]Solution
	BestScore float64
}

func main() {
	arg.MustParse(&state.Args)
	_, err := toml.DecodeFile(state.Args.ConfigPath, &state.Config)
	if err != nil {
		log.Fatalf("load config failed: %s\n", err.Error())
	}

	if state.Config.LLM.Model == "" {
		log.Fatalf("LLM model (llm.model) is not set\n")
	}
	if state.Config.LLM.BaseURL == "" {
		log.Fatalf("LLM base URL (llm.base_url) is not set\n")
	}
	if state.Config.LLM.APIKey == "" {
		log.Printf("warning: LLM API key (llm.api_key) is not set\n")
	}

	state.TempPath, err = os.MkdirTemp("", "tinyevolve")
	if err != nil {
		log.Fatalf("create temp dir failed: %s\n", err.Error())
	}

	state.Client = openai.NewClient(
		option.WithBaseURL(state.Config.LLM.BaseURL),
		option.WithAPIKey(state.Config.LLM.APIKey),
	)

	loadSolutions()
	for range state.Args.Iterations {
		runIteration()
	}
}

func loadSolutions() {
	state.Solutions = map[string]Solution{}
	state.BestScore = math.Inf(-1)

	entries, err := os.ReadDir(state.Args.DataPath)
	if err != nil {
		log.Fatalf("read data directory failed: %s\n", err.Error())
		return
	}

	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		id, found := strings.CutSuffix(entry.Name(), state.Config.Eval.Extension)
		if !found {
			continue
		}

		codePath := fmt.Sprintf("%s/%s%s", state.Args.DataPath, id, state.Config.Eval.Extension)
		codeBytes, err := os.ReadFile(codePath)
		if err != nil {
			log.Printf("warning: load solution %s code failed: %s\n", id, err.Error())
			continue
		}

		meta := SolutionMeta{}
		metaPath := fmt.Sprintf("%s/%s.meta", state.Args.DataPath, id)
		_, err = toml.DecodeFile(metaPath, &meta)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				log.Printf("solution %s meta not found, running evaluation", id)
				addSolutionFromCode(string(codeBytes), id)
			} else {
				log.Printf("warning: load solution %s meta failed: %s\n", id, err.Error())
			}
			continue
		}

		state.Solutions[id] = Solution{
			Code: string(codeBytes),
			Meta: meta,
		}
		state.BestScore = math.Max(state.BestScore, meta.Score)
	}

	if len(state.Solutions) == 0 {
		log.Printf("no initial solutions found, adding empty solution\n")
		addSolutionFromCode("", "empty")
	} else {
		log.Printf("loaded %d solutions from %s\n", len(state.Solutions), state.Args.DataPath)
	}
}

func runIteration() {
	baseID := getBaseSolutionID()
	base := state.Solutions[baseID]
	log.Printf("generating new solution from ancestor %s (score %f)\n", baseID, base.Meta.Score)

	systemPrompt := generatePrompt("system", state.Config.Prompt.System, base)
	userPrompt := generatePrompt("user", state.Config.Prompt.User, base)

	completion, err := state.Client.Chat.Completions.New(context.TODO(), openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(userPrompt),
		},
		Model: state.Config.LLM.Model,
	})
	if err != nil {
		log.Printf("warning: request failed: %s\n", err.Error())
		return
	}

	code := completion.Choices[0].Message.Content
	addSolutionFromCode(code, getNewSolutionID())
}

func addSolutionFromCode(code string, id string) {
	score, comment := evaluateCode(code)
	log.Printf("solution %s score %f, comment \"%s\"\n", id, score, comment)

	state.Solutions[id] = Solution{
		Code: code,
		Meta: SolutionMeta{
			Score:   score,
			Comment: comment,
		},
	}

	if score > state.BestScore {
		log.Printf("new best solution %s found, score %f\n", id, score)
		state.BestScore = score
	}

	codePath := fmt.Sprintf("%s/%s%s", state.Args.DataPath, id, state.Config.Eval.Extension)
	os.WriteFile(codePath, []byte(code), 0644)

	metaPath := fmt.Sprintf("%s/%s.meta", state.Args.DataPath, id)
	metaFile, err := os.Create(metaPath)
	if err != nil {
		log.Printf("warning: create meta file %s failed: %s\n", metaPath, err.Error())
		return
	}
	err = toml.NewEncoder(metaFile).Encode(state.Solutions[id].Meta)
	if err != nil {
		log.Printf("warning: encode meta file %s failed: %s\n", metaPath, err.Error())
		return
	}
	err = metaFile.Close()
	if err != nil {
		log.Printf("warning: close meta file %s failed: %s\n", metaPath, err.Error())
	}
}

func getBaseSolutionID() string {
	type ListItem struct {
		ID    string
		Score float64
	}
	solutions := []ListItem{}
	for id, sol := range state.Solutions {
		solutions = append(solutions, ListItem{
			ID:    id,
			Score: sol.Meta.Score,
		})
	}
	sort.Slice(solutions, func(a, b int) bool {
		return solutions[a].Score < solutions[b].Score
	})

	sum := 0
	for idx := range solutions {
		sum += (idx + 1) * (idx + 1)
	}
	pos := rand.Intn(sum)
	for idx := range solutions {
		pos -= (idx + 1) * (idx + 1)
		if pos <= 0 {
			return solutions[idx].ID
		}
	}
	return solutions[0].ID
}

func generatePrompt(name string, tmpl string, base Solution) string {
	promptTmpl, err := template.New(name).Parse(tmpl)
	if err != nil {
		log.Fatalf("parse %s prompt failed: %s\n", name, err.Error())
	}

	promptBuffer := bytes.Buffer{}
	err = promptTmpl.Execute(&promptBuffer, base)
	if err != nil {
		log.Fatalf("execute %s prompt failed: %s\n", name, err.Error())
	}

	return promptBuffer.String()
}

func evaluateCode(code string) (float64, string) {
	path := state.TempPath + "/solution" + state.Config.Eval.Extension
	err := os.WriteFile(path, []byte(code), 0644)
	if err != nil {
		return math.Inf(-1), fmt.Sprintf("write temp file failed: %s", err)
	}

	command := append(state.Config.Eval.Command, path)
	resultBytes, err := exec.Command(command[0], command[1:]...).Output()
	if err != nil {
		return math.Inf(-1), fmt.Sprintf("run evaluator failed: %s", err)
	}

	score, comment := parseResult(string(resultBytes))
	return score, comment
}

func parseResult(result string) (float64, string) {
	separator := strings.IndexByte(result, '\n')
	if separator < 0 {
		separator = len(result)
	}

	scoreStr := strings.TrimSpace(result[:separator])
	score, err := strconv.ParseFloat(scoreStr, 64)
	if err != nil {
		return math.Inf(-1), fmt.Sprintf("parse score failed: %s", err)
	}

	if separator == len(result) {
		return score, ""
	}

	comment := strings.Clone(result[separator+1:])
	comment = strings.Trim(comment, " \n")
	return score, comment
}

func getNewSolutionID() string {
	for {
		id := ""
		chars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
		for range 8 {
			id += string(chars[rand.Intn(len(chars))])
		}
		if _, ok := state.Solutions[id]; !ok {
			return id
		}
	}
}
