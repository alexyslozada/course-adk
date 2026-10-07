package main

import (
	"fmt"
	"strings"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
	"google.golang.org/genai"
)

// maxTranscriptLines limita cuánta conversación se le pasa a un flujo.
const maxTranscriptLines = 24

// conversationKey es la clave del estado donde se guarda la conversación para los flujos.
const conversationKey = "conversation"

// transcript resume la conversación en líneas "Cliente: ..." y "Asistente: ...".
// Solo cuenta los textos: no las llamadas a tools ni los pensamientos del modelo.
func transcript(contents []*genai.Content) string {
	var lines []string
	for _, content := range contents {
		if content == nil {
			continue
		}
		var text strings.Builder
		for _, part := range content.Parts {
			if part != nil && part.Text != "" && !part.Thought {
				text.WriteString(part.Text)
			}
		}
		if text.Len() == 0 {
			continue
		}
		switch content.Role {
		case genai.RoleUser:
			lines = append(lines, "Cliente: "+strings.TrimSpace(text.String()))
		case genai.RoleModel:
			lines = append(lines, "Asistente: "+strings.TrimSpace(text.String()))
		}
	}
	if len(lines) > maxTranscriptLines {
		lines = lines[len(lines)-maxTranscriptLines:]
	}
	return strings.Join(lines, "\n")
}

// rememberConversation es un callback BeforeModel del coordinador: la solicitud que va a recibir el
// modelo trae toda la conversación, y aquí se guarda en el estado para que las tools la lean.
//
// Una tool no puede leer la sesión (su contexto no lo soporta), y agenttool corre el agente en una
// sesión nueva y vacía. Sin esto, un flujo solo sabe lo que el LLM del coordinador le reenvíe en
// "request", y un mensaje como "ok, aumenta el presupuesto" llega sin saber de qué se hablaba.
func rememberConversation(ctx agent.Context, req *model.LLMRequest) (*model.LLMResponse, error) {
	if err := ctx.State().Set(conversationKey, transcript(req.Contents)); err != nil {
		return nil, err
	}
	return nil, nil
}

// ConversationRequest es lo que el coordinador envía a un flujo.
type ConversationRequest struct {
	Request string `json:"request" jsonschema:"Lo que pide el cliente ahora, resumido con todo el contexto de la conversación"`
}

type ConversationResponse struct {
	Result string `json:"result"`
}

// runAgent corre un agente en una sesión aparte (no ensucia la conversación) y devuelve el texto de
// su último evento completo con contenido.
func runAgent(ctx agent.Context, a agent.Agent, input string) (string, error) {
	sessions := session.InMemoryService()
	r, err := runner.New(runner.Config{AppName: a.Name(), Agent: a, SessionService: sessions})
	if err != nil {
		return "", err
	}
	created, err := sessions.Create(ctx, &session.CreateRequest{AppName: a.Name(), UserID: ctx.UserID()})
	if err != nil {
		return "", err
	}

	var last string
	events := r.Run(ctx, ctx.UserID(), created.Session.ID(), genai.NewContentFromText(input, genai.RoleUser), agent.RunConfig{})
	for event, err := range events {
		if err != nil {
			return "", fmt.Errorf("el flujo %s falló: %w", a.Name(), err)
		}
		if event.ErrorCode != "" || event.ErrorMessage != "" {
			return "", fmt.Errorf("el flujo %s falló: %s %s", a.Name(), event.ErrorCode, event.ErrorMessage)
		}
		if content := event.LLMResponse.Content; content != nil && !event.Partial {
			var text strings.Builder
			for _, part := range content.Parts {
				if part != nil && part.Text != "" && !part.Thought {
					text.WriteString(part.Text)
				}
			}
			if text.Len() > 0 {
				last = text.String()
			}
		}
	}
	return last, nil
}

// conversationTool expone un agente como tool y le pasa la conversación guardada por
// rememberConversation.
func conversationTool(name string, a agent.Agent) (tool.Tool, error) {
	run := func(ctx agent.Context, req ConversationRequest) (ConversationResponse, error) {
		history, _ := ctx.State().Get(conversationKey)
		input := fmt.Sprintf("Conversación hasta ahora:\n%v\n\nLo que pide el cliente ahora: %s", history, req.Request)
		result, err := runAgent(ctx, a, input)
		return ConversationResponse{Result: result}, err
	}
	return functiontool.New(functiontool.Config{Name: name, Description: a.Description()}, run)
}

// NoArgs es la entrada de una tool que no recibe nada.
type NoArgs struct{}

// fixedInputTool expone un agente como tool que NO recibe texto: siempre lo arranca con la misma
// orden. Sirve para un agente que solo debe contar lo que traen sus herramientas, como el reporte
// del día: con una entrada libre, lo que dijera el cliente se colaba en el reporte como si fuera un dato.
func fixedInputTool(name string, a agent.Agent, input string) (tool.Tool, error) {
	run := func(ctx agent.Context, _ NoArgs) (ConversationResponse, error) {
		result, err := runAgent(ctx, a, input)
		return ConversationResponse{Result: result}, err
	}
	return functiontool.New(functiontool.Config{Name: name, Description: a.Description()}, run)
}
