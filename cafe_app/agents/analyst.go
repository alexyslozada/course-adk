package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"time"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/agent/workflowagents/parallelagent"
	"google.golang.org/adk/v2/agent/workflowagents/sequentialagent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/agenttool"
	"google.golang.org/adk/v2/tool/skilltoolset"
	"google.golang.org/adk/v2/tool/skilltoolset/skill"
)

// Las skills viajan dentro del binario: no dependen de desde qué carpeta se lance.
//
//go:embed skills
var skillsFS embed.FS

// today da la fecha de hoy para que los especialistas consulten solo el día del reporte.
func today() string { return time.Now().Format(time.DateOnly) }

// specialist crea un analista que consulta una parte del negocio con las tools MCP que se le
// permiten y deja su resumen en el estado, bajo outputKey, para el redactor.
func specialist(m model.LLM, cafeTools tool.Toolset, name, description, task, outputKey string, allowed ...string) (agent.Agent, error) {
	specialistConfig := llmagent.Config{
		Name:        name,
		Model:       m,
		Description: description,
		// El modelo no sabe qué día es: la fecha se inserta en cada ejecución.
		InstructionProvider: func(agent.ReadonlyContext) (string, error) {
			return fmt.Sprintf(`
				Eres un analista de una cafetería. Hoy es %s.
				%s
				Usa solo los datos de las tools, sin inventar cifras. Responde en español,
				en texto plano, con un máximo de 6 líneas.
			`, today(), task), nil
		},
		Toolsets:  []tool.Toolset{tool.FilterToolset(cafeTools, tool.StringPredicate(allowed))},
		OutputKey: outputKey,
	}
	return llmagent.New(specialistConfig)
}

// dailyReport arma el pipeline del reporte del día:
//
//	secuencial [ paralelo [ ventas, inventario, reseñas ], redactor ]
//
// y lo devuelve como una tool para que el coordinador lo use.
func dailyReport(m model.LLM, cafeTools tool.Toolset) (tool.Tool, error) {
	// ----------------------------------------------
	// Tres especialistas, cada uno con sus tools de solo lectura.
	// ----------------------------------------------
	sales, err := specialist(m, cafeTools, "sales_analyst",
		"Analiza las ventas de hoy.",
		`Analiza las ventas de hoy: consulta "list_sales" con la fecha de hoy y reporta
		 cuántas ventas hubo, el total en pesos (cada total ya incluye su descuento),
		 los productos más vendidos y cuántas ventas llevaron descuento.`,
		"sales_analysis", "list_sales")
	if err != nil {
		return nil, err
	}
	stock, err := specialist(m, cafeTools, "stock_analyst",
		"Revisa el inventario.",
		`Revisa el inventario: consulta "get_stock" y reporta qué productos están en o
		 bajo su mínimo, con su cantidad y su mínimo.`,
		"stock_analysis", "get_stock")
	if err != nil {
		return nil, err
	}
	reviews, err := specialist(m, cafeTools, "reviews_analyst",
		"Analiza las reseñas de hoy.",
		`Analiza las reseñas de hoy: consulta "list_reviews" con la fecha de hoy y reporta
		 cuántas hubo, la calificación promedio y las de 1 o 2 estrellas con su comentario.`,
		"reviews_analysis", "list_reviews")
	if err != nil {
		return nil, err
	}

	// ----------------------------------------------
	// Workflow paralelo: los tres especialistas trabajan al mismo tiempo.
	// ----------------------------------------------
	analysisConfig := parallelagent.Config{AgentConfig: agent.Config{
		Name:        "daily_analysis",
		Description: "Analiza ventas, inventario y reseñas al mismo tiempo.",
		SubAgents:   []agent.Agent{sales, stock, reviews},
	}}
	analysis, err := parallelagent.New(analysisConfig)
	if err != nil {
		return nil, err
	}

	// ----------------------------------------------
	// Redactor: lee los tres análisis del estado, sigue la skill del reporte
	// y responde con salida estructurada (reportSchema).
	// ----------------------------------------------
	skillsDir, err := fs.Sub(skillsFS, "skills")
	if err != nil {
		return nil, err
	}
	skillTools, err := skilltoolset.New(context.Background(), skilltoolset.Config{
		Source: skill.NewFileSystemSource(skillsDir),
	})
	if err != nil {
		return nil, err
	}
	writerConfig := llmagent.Config{
		Name:        "report_writer",
		Model:       m,
		Description: "Redacta el reporte del día con los tres análisis.",
		Instruction: `
			Redacta el reporte del día de la cafetería con los tres análisis siguientes.
			Antes de escribir, usa la skill "reporte-del-dia" y sigue sus criterios.

			Ventas: {sales_analysis}
			Inventario: {stock_analysis}
			Reseñas: {reviews_analysis}
		`,
		Toolsets:     []tool.Toolset{skillTools},
		OutputSchema: &reportSchema,
	}
	writer, err := llmagent.New(writerConfig)
	if err != nil {
		return nil, err
	}

	// ----------------------------------------------
	// Workflow secuencial: primero el análisis en paralelo, luego el redactor.
	// ----------------------------------------------
	pipelineConfig := sequentialagent.Config{AgentConfig: agent.Config{
		Name:        "daily_report",
		Description: "Genera el reporte del día de la cafetería: ventas, inventario, reseñas y recomendaciones.",
		SubAgents:   []agent.Agent{analysis, writer},
	}}
	pipeline, err := sequentialagent.New(pipelineConfig)
	if err != nil {
		return nil, err
	}
	// agenttool lo convierte en una tool del coordinador: la llama como a cualquier función.
	return agenttool.New(pipeline, nil), nil
}
