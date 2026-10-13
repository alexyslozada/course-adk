# Presentación del taller

La parte teórica antes de cada clase: un título por clase y, cuando hace falta explicar un flujo, una animación. Sigue el temario de Notion (5 módulos, 300 min) y el estilo de ed.team.

## Cómo abrirla

Abre `index.html` en el navegador (doble clic). No necesita servidor ni compilar nada. Las fuentes vienen de Google Fonts: sin internet se usa la fuente del sistema.

| Tecla | Acción |
|---|---|
| `→` `Espacio` `PageDown` | siguiente |
| `←` `PageUp` | anterior |
| `P` | pausar o seguir la animación |
| `R` | repetir la animación desde el inicio |
| `F` | pantalla completa |

También se avanza con clic (el tercio izquierdo retrocede). La URL guarda la diapositiva: `index.html#35` abre la del bucle.

## Archivos

| Archivo | Qué tiene |
|---|---|
| `slides.js` | El temario: módulos, clases, carpeta del repo y escena de cada clase |
| `scenes.js` | Las animaciones, escritas como datos: nodos, flechas y pasos |
| `engine.js` | El motor que dibuja las escenas en SVG y las reproduce en bucle |
| `app.js` | Arma las diapositivas y maneja la navegación |
| `styles.css` | Colores y fuentes de ed.team |

Una clase sin `scene` en `slides.js` se muestra solo con su título.

## Cómo editar una animación

Cada escena de `scenes.js` tiene nodos (`N`), flechas (`E`), grupos (`G`) y una lista de pasos. Por ejemplo, para que un mensaje viaje del usuario al agente y vuelva una respuesta:

```js
["send", "uA", "¿Qué hora es?"],
["send", "uA", "Son las 10:42", { back: true }],
["cap", "El subtítulo que aparece debajo"],
```

Los pasos disponibles están documentados al inicio de `engine.js`. Los colores de los nodos dicen qué son: persona (amarillo), agente (azul), modelo (lila), código Go (verde), otro proceso (cian) y datos (gris).
