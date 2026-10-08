package main

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
	"google.golang.org/genai"
)

// El prefijo de una clave del estado decide quién la ve y cuánto dura:
//
//	sin prefijo → solo esta conversación (customer_name, conversation, title...)
//	user:       → todas las conversaciones del usuario que inició sesión
//	app:        → todos los usuarios y todas sus conversaciones
//	temp:       → solo el turno en curso; no se guarda (ver combo.go)
const (
	// preferencesKey guarda cómo quiere ser atendido el usuario. Se escribe en una conversación y la
	// leen todas las siguientes: es la memoria entre conversaciones.
	preferencesKey = "user:preferencias"
	// noticesKey guarda los avisos para todo el equipo de la cafetería.
	noticesKey = "app:avisos"
	// titleKey es el título de la conversación (su primer mensaje), para listarla en la app.
	titleKey = "title"
)

// maxMemoryText limita lo que se guarda en una clave que ven todas las conversaciones: va en la
// instrucción de cada llamada al modelo.
const maxMemoryText = 600

// MemoryTextRequest es la entrada de las tools que reemplazan un texto guardado.
type MemoryTextRequest struct {
	Text string `json:"text" jsonschema:"el texto completo actualizado (vacío para borrarlo)"`
}

type MemoryTextResponse struct {
	Saved string `json:"saved"`
}

// saveTextTool crea una tool que reemplaza el texto guardado en key. Se guarda el texto completo, no
// un agregado: así el modelo también puede corregir u olvidar algo.
func saveTextTool(name, description, key string) (tool.Tool, error) {
	run := func(ctx agent.Context, req MemoryTextRequest) (MemoryTextResponse, error) {
		text := strings.TrimSpace(req.Text)
		if utf8.RuneCountInString(text) > maxMemoryText {
			return MemoryTextResponse{}, fmt.Errorf("el texto es muy largo (máximo %d caracteres): resúmelo", maxMemoryText)
		}
		if err := ctx.State().Set(key, text); err != nil {
			return MemoryTextResponse{}, err
		}
		return MemoryTextResponse{Saved: text}, nil
	}
	return functiontool.New(functiontool.Config{Name: name, Description: description}, run)
}

func preferencesTool() (tool.Tool, error) {
	return saveTextTool("save_preferences",
		"Guarda las preferencias del usuario sobre cómo atenderlo (por ejemplo: cómo llamarlo, respuestas cortas). "+
			"Se recuerdan en todas sus conversaciones. Recibe la lista completa actualizada.",
		preferencesKey)
}

func noticesTool() (tool.Tool, error) {
	return saveTextTool("save_notices",
		"Guarda los avisos para todo el equipo de la cafetería (por ejemplo: hoy no hay pan de queso). "+
			"Los ven todos los usuarios en todas sus conversaciones. Recibe la lista completa actualizada.",
		noticesKey)
}

// titleConversation es un callback BeforeAgent del coordinador: la primera vez guarda el primer
// mensaje del usuario como título de la conversación, para que la app la liste.
func titleConversation(ctx agent.Context) (*genai.Content, error) {
	if _, err := ctx.State().Get(titleKey); err == nil {
		return nil, nil
	}
	text := strings.Join(strings.Fields(textOf(ctx.UserContent())), " ")
	if text == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(text) > 60 {
		text = string([]rune(text)[:57]) + "…"
	}
	return nil, ctx.State().Set(titleKey, text)
}
