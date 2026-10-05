# my_multiagent_modes — sub-agentes en `ModeTask` y `ModeSingleTurn`

El coordinador (`coffee_host`) **encarga** trabajo a dos sub-agentes y siempre recupera el control:

| Sub-agente | Modo | Qué hace |
|---|---|---|
| `loyalty_signup` | `ModeTask` | Inscribe al cliente en el club. **Conversa** hasta tener nombre, bebida favorita y mes de cumpleaños. |
| `review_classifier` | `ModeSingleTurn` | Clasifica una reseña. **No habla** con el usuario. |

```mermaid
graph TD
    U((Usuario)) <--> H[coffee_host<br/>coordinador]
    H -- "tool: loyalty_signup()" --> L[loyalty_signup<br/>ModeTask]
    L -- "finish_task → loyaltySchema" --> H
    L <-. conversa hasta terminar .-> U
    H -- "tool: review_classifier(review)" --> C[review_classifier<br/>ModeSingleTurn]
    C -- "JSON reviewSchema" --> H
```

## Los tres modos de un sub-agente

| Modo | ¿Habla con el usuario? | ¿Quién atiende tu siguiente mensaje? | ¿Cómo lo alcanza el padre? |
|---|---|---|---|
| `ModeChat` (ver [`my_multiagent_delegation`](../my_multiagent_delegation)) | Sí | El último agente que respondió, **hasta que preguntes algo diferente** a lo que sabe hacer y él decida transferir. | `transfer_to_agent` |
| `ModeTask` | Sí, solo lo necesario para su tarea | El sub-agente, **hasta que termina la tarea**. Al llamar `finish_task`, el control **vuelve solo** al coordinador con el resultado. | Como una **tool** |
| `ModeSingleTurn` | **Nunca** | Siempre el coordinador. El sub-agente hace su trabajo en un solo turno y devuelve el resultado. | Como una **tool** |

## El ADK los convierte en tools

Al crear el coordinador con `llmagent.New`, el ADK recorre sus `SubAgents` (`agent/llmagent/llmagent.go`, `installTaskTools`). A cada uno en `ModeTask` o `ModeSingleTurn` lo envuelve en una **tool del coordinador**:

- `name` = `Name` del sub-agente
- `description` = `Description` del sub-agente
- `parameters` = `InputSchema` del sub-agente. Si no tiene, usa `{"request": string}`.

Al agente en `ModeTask` además le agrega la tool **`finish_task`**, cuyos parámetros son su `OutputSchema`.

### Modo single turn: el `InputSchema` lo llena el LLM del coordinador

Nadie en nuestro código envía `reviewInputSchema`. Ese esquema se vuelve los parámetros de la tool `review_classifier(review)`, y es el **LLM del coordinador** quien extrae la reseña del mensaje del usuario para llenarlo.

```mermaid
sequenceDiagram
    actor U as Usuario
    participant H as LLM coffee_host
    participant T as SingleTurnTool (ADK)
    participant C as LLM review_classifier

    U->>H: El croissant estaba delicioso
    H->>T: review_classifier({"review": "El croissant estaba delicioso"})
    T->>T: valida args contra reviewInputSchema
    T->>C: ejecuta el sub-agente con {"review": ...}
    C->>T: {"sentiment":"positivo","rating":5,"summary":...}
    T->>H: function response {"result": {...}}
    H->>U: ¡Gracias por tu comentario!
```

### Modo task: conversa y luego devuelve el control

La tool `loyalty_signup` no ejecuta nada por sí misma. Cuando el coordinador la llama, el ADK le pasa la conversación al sub-agente. Este pregunta lo que falte y, al terminar, llama a `finish_task` con un JSON que cumple `loyaltySchema`. Ese JSON llega al coordinador como respuesta de la tool, y el control vuelve a él.

```mermaid
sequenceDiagram
    actor U as Usuario
    participant H as coffee_host
    participant L as loyalty_signup

    U->>H: Quiero unirme al club
    H->>L: tool loyalty_signup()
    L->>U: ¿Cómo te llamas?
    U->>L: Alexys
    L->>U: ¿Cuál es tu bebida favorita?
    U->>L: Latte
    L->>U: ¿En qué mes cumples años?
    U->>L: Marzo
    L->>H: finish_task({"name":"Alexys","favorite_drink":"Latte","birthday_month":"Marzo"})
    Note over H: El control vuelve solo al coordinador
    H->>U: ¡Bienvenido al club, Alexys!
```

## Por qué ahora el grafo sí tiene flechas

El generador de grafos de la UI solo dibuja líneas de **agente → tool**. Como el ADK convirtió a estos sub-agentes en tools del coordinador, aparecen como nodos 🔧 unidos a `coffee_host`. En `my_multiagent_delegation` eran sub-agentes en `ModeChat` y aparecían como nodos 🤖 sueltos.

## Archivos

- `schemas.go`: `reviewSchema` (salida del clasificador), `reviewInputSchema` (entrada del clasificador) y `loyaltySchema` (salida de la inscripción).
- `main.go`: los tres agentes. Los sub-agentes solo se diferencian por su `Mode`.

## Prueba

```bash
export GOOGLE_API_KEY=...
go run . web api -include_debug_api webui
```

> Desde ADK v2.5.0, el grafo y las trazas de la UI necesitan `-include_debug_api`. La UI queda en http://localhost:8080/ui/.

1. "Quiero unirme al club" → `loyalty_signup` pregunta los datos y, al terminar, `coffee_host` te da la bienvenida por tu nombre.
2. "El croissant estaba delicioso" → `coffee_host` llama a `review_classifier`, recibe el JSON y responde él mismo.
