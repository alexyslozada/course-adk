package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// claudeResult son los campos que nos interesan de la respuesta de
// `claude -p --output-format json`
type claudeResult struct {
	IsError bool   `json:"is_error"`
	Result  string `json:"result"`
}

// AskClaude ejecuta Claude Code en modo headless (sin interfaz) dentro de
// workDir y devuelve su respuesta final.
func AskClaude(ctx context.Context, workDir, prompt string) (string, error) {
	cmd := exec.CommandContext(ctx, "claude",
		"-p", prompt,
		"--output-format", "json",
		// Solo lectura: Claude Code únicamente puede leer y buscar archivos.
		// Sin Bash, Edit ni Write, aunque alguien se lo pida por A2A.
		"--tools", "Read,Grep,Glob",
		// Cualquier otra acción se niega sin preguntar (no hay nadie para aprobarla)
		"--permission-mode", "dontAsk",
	)
	cmd.Dir = workDir

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("error al ejecutar claude: %w", err)
	}

	var result claudeResult
	err = json.Unmarshal(output, &result)
	if err != nil {
		return "", fmt.Errorf("la respuesta de claude no es un JSON válido: %w", err)
	}
	if result.IsError {
		return "", fmt.Errorf("claude respondió con error: %s", result.Result)
	}

	return result.Result, nil
}
