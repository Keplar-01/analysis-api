package analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/diploma/analysis-api-service/internal/model"
)

type Analyzer struct {
	binaryPath string
}

func New(binaryPath string) *Analyzer {
	return &Analyzer{binaryPath: binaryPath}
}

type analyzerConfig struct {
	Input        string      `json:"input"`
	Output       string      `json:"output"`
	OutputFormat string      `json:"output_format"`
	Analysis     analysisCfg `json:"analysis"`
	Debug        debugCfg    `json:"debug"`
	Features     featuresCfg `json:"features"`
}

type analysisCfg struct {
	MaxLoopDepth        int  `json:"max_loop_depth"`
	AnalyzeDependencies bool `json:"analyze_dependencies"`
	AnalyzeSCEV         bool `json:"analyze_scev"`
}

type debugCfg struct {
	Verbose    bool `json:"verbose"`
	DumpLoops  bool `json:"dump_loops"`
	DumpSCEV   bool `json:"dump_scev"`
	DumpMemory bool `json:"dump_memory"`
}

type featuresCfg struct {
	EnableFingerprint    bool `json:"enable_fingerprint"`
	EnableClassification bool `json:"enable_classification"`
}

func (a *Analyzer) RunSource(ctx context.Context, filename string, content []byte) ([]model.StaticArtifactPattern, error) {
	workDir, err := os.MkdirTemp("", "analysis-api-analyzer-*")
	if err != nil {
		return nil, fmt.Errorf("create workdir: %w", err)
	}
	defer os.RemoveAll(workDir)

	if filename == "" {
		filename = "input.c"
	}
	sourcePath := filepath.Join(workDir, filepath.Base(filename))
	if err := os.WriteFile(sourcePath, content, 0o644); err != nil {
		return nil, fmt.Errorf("write source: %w", err)
	}

	confPath := filepath.Join(workDir, "conf.json")
	outPath := filepath.Join(workDir, "out.json")
	conf := analyzerConfig{
		Input:        filepath.Base(sourcePath),
		Output:       "out.json",
		OutputFormat: "json",
		Analysis: analysisCfg{
			MaxLoopDepth:        4,
			AnalyzeDependencies: true,
			AnalyzeSCEV:         true,
		},
		Debug:    debugCfg{},
		Features: featuresCfg{EnableFingerprint: true, EnableClassification: true},
	}

	confBytes, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal conf: %w", err)
	}
	if err := os.WriteFile(confPath, confBytes, 0o644); err != nil {
		return nil, fmt.Errorf("write conf: %w", err)
	}

	cmd := exec.CommandContext(ctx, a.binaryPath, "conf.json", "--quiet")
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("run analyzer (%s): %w: %s", a.binaryPath, err, string(output))
	}

	raw, err := os.ReadFile(outPath)
	if err != nil {
		return nil, fmt.Errorf("read out.json: %w", err)
	}

	var patterns []model.StaticArtifactPattern
	if err := json.Unmarshal(raw, &patterns); err != nil {
		return nil, fmt.Errorf("parse out.json: %w", err)
	}
	return patterns, nil
}