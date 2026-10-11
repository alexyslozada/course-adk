// El temario del curso: 5 módulos, 300 minutos.
// Cada clase tiene un título, su carpeta del repo (si aplica) y, si necesita
// explicación, una escena animada de scenes.js.

const COURSE = {
  title: "Arquitectura multiagentes con el ADK de Google",
  subtitle: "Construye un sistema multiagente real con el ADK de Google",
  meta: ["Taller presencial", "5 horas", "Go + ADK"],
};

const MODULES = [
  {
    title: "Tu primer agente y cómo piensa ADK",
    minutes: 40,
    desc: "Del entorno vacío a un agente conversando en tu terminal y en el navegador.",
    classes: [
      { title: "Qué construimos hoy: demo de la cafetería con agentes", folder: "cafe_app" },
      { title: "Preparación del entorno y del repositorio" },
      { title: "Cómo avanzar sin quedarte atrás: un ejemplo por concepto" },
      { title: "El modelo mental de ADK: agente, runner, sesión y eventos", scene: "mental" },
      { title: "Crear el agente: nombre, descripción, instrucción y modelo", folder: "my_agent", scene: "config" },
      { title: "El runner y el flujo de eventos", scene: "events" },
      { title: "Consola e interfaz web con el mismo programa" },
      { title: "El mismo agente con Gemini, OpenAI o Anthropic", folder: "my_agent_with_other_models", scene: "providers" },
    ],
  },
  {
    title: "Herramientas y MCP: que el agente actúe",
    minutes: 65,
    desc: "El agente deja de conversar y empieza a operar, con aprobación humana cuando hace falta.",
    classes: [
      { title: "De texto a acción: cómo el modelo decide usar una herramienta", scene: "toolLoop" },
      { title: "Herramientas que ya vienen en ADK", folder: "my_agent_with_tools" },
      { title: "Exponer una función como herramienta del agente", folder: "my_agent_with_own_tools" },
      { title: "El nombre y la descripción son el contrato" },
      { title: "Acciones irreversibles: pedir confirmación humana", folder: "my_agent_with_hitl", scene: "hitl" },
      { title: "Confirmar y modificar: aprobar un descuento distinto", folder: "my_agent_with_hitl_payload", scene: "hitlPayload" },
      { title: "MCP en la práctica: conecta tu agente a un servidor", folder: "my_agent_with_mcp", scene: "mcp" },
    ],
  },
  {
    title: "Skills, estado y salida estructurada",
    minutes: 50,
    desc: "Conocimiento empaquetado, memoria de la conversación y respuestas que el código puede usar.",
    classes: [
      { title: "Skill, herramienta o MCP: cuándo usar cada una", scene: "whichOne" },
      { title: "Anatomía de una skill y cómo la descubre el agente", folder: "my_agent_with_skills", scene: "progressive" },
      { title: "Recargar skills sin reiniciar el agente", folder: "my_agent_with_skills_reload", scene: "reload" },
      { title: "Sesiones y estado", folder: "my_agent_with_sessions", scene: "state" },
      { title: "Sesiones persistentes con SQLite", folder: "my_agent_with_sessions_db", scene: "sqlite" },
      { title: "Salida estructurada y OutputKey", folder: "my_agent_with_output_schema", scene: "output" },
    ],
  },
  {
    title: "Multiagente: todos los flujos",
    minutes: 80,
    desc: "De «el modelo decide» a «el código decide».",
    classes: [
      { title: "Los patrones de orquestación y cuándo usar cada uno", scene: "patterns" },
      { title: "Delegación automática: el coordinador reparte el trabajo", folder: "my_multiagent_delegation", scene: "delegation" },
      { title: "Los tres modos de sub-agente", folder: "my_multiagent_modes", scene: "modes" },
      { title: "Flujo secuencial: pasos que dependen del anterior", folder: "my_workflow_sequential", scene: "sequential" },
      { title: "Flujo paralelo: tareas independientes a la vez", folder: "my_workflow_parallel", scene: "parallel" },
      { title: "Flujo en bucle: un agente revisa el trabajo de otro", folder: "my_workflow_loop", scene: "loop" },
      { title: "Grafos: nodos, rutas condicionales y código Go", folder: "my_workflow_graph", scene: "graph" },
      { title: "Workflow dinámico: el código decide cuántas veces llamar a un agente", folder: "my_workflow_dynamic", scene: "dynamic" },
      { title: "A2A: un agente de otra empresa como sub-agente", folder: "my_agent_a2a", scene: "a2a" },
      { title: "Extra: Claude Code como agente A2A", folder: "my_agent_a2a_claude", scene: "claude" },
    ],
  },
  {
    title: "Proyecto final: la cafetería con agentes",
    minutes: 65,
    desc: "Todo lo aprendido junto en una aplicación real, empaquetada en contenedores.",
    classes: [
      { title: "La arquitectura: app, servidor MCP, agentes y panadería", folder: "cafe_app", scene: "arch" },
      { title: "El servidor MCP propio de la app, protegido con token", folder: "cafe_app/mcp" },
      { title: "Coordinador y barista: la venta se aprueba con una tarjeta", folder: "cafe_app/agents", scene: "barista" },
      { title: "El administrador: herramientas propias frente a MCP", folder: "cafe_app/agents" },
      { title: "El analista: paralelo, secuencial, skill y salida estructurada", folder: "cafe_app/agents", scene: "analyst" },
      { title: "Poner límites a la delegación", folder: "cafe_app/agents", scene: "limits" },
      { title: "Flujos: el modelo entiende, el código calcula", folder: "cafe_app/agents", scene: "flows" },
      { title: "Memoria: conversación, usuario y toda la app", folder: "cafe_app/agents", scene: "memory" },
      { title: "Reabastecimiento con la panadería por A2A", folder: "cafe_app/bakery", scene: "restock" },
      { title: "Despliegue con contenedores", folder: "cafe_app", scene: "deploy" },
      { title: "Mapa de lo construido y qué explorar después" },
    ],
  },
];

window.COURSE = COURSE;
window.MODULES = MODULES;
