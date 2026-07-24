# UI brief — Holocron

> Documento de contexto para una sesión de diseño (Claude design / rediseño de UI).
> Describe **qué es** la app, **quién** la usa, **cómo se ve hoy**, **qué restricciones
> técnicas** hay y **qué se quiere mejorar**. No es una spec de implementación: es el
> insumo para que un diseñador proponga una dirección visual y de interacción.

## 1. Qué es Holocron

Panel de administración y dashboard web para un **HTPC casero** (Home Theater PC) que
corre en una **Raspberry Pi 4/5** en la LAN. La Pi corre Plex Media Server y
qBittorrent; Holocron es el tablero desde donde el dueño:

- ve el estado de la máquina (CPU, RAM, temperatura, disco),
- administra su biblioteca de películas y series (nombres, metadata `.nfo`, subtítulos),
- y controla las descargas de torrents.

Es una herramienta **personal, de una sola persona**, usada desde la red local. No hay
multiusuario, ni onboarding, ni marketing: es un tablero de trabajo denso y funcional,
tipo "homelab dashboard". La referencia mental es algo entre un panel de admin
(Grafana/Portainer/Sonarr) y una utility app minimalista.

## 2. Quién lo usa y en qué contexto

- **Un usuario técnico** (el dueño del HTPC), que entra cada tanto a revisar el estado o
  hacer una tarea puntual: "¿por qué está lento?", "generá los .nfo", "bajá subtítulos
  de esta peli", "pausá este torrent".
- **Dispositivos**: mayormente desde una laptop/desktop en la LAN; ocasionalmente desde
  el teléfono. **Responsive importa** pero el caso principal es pantalla grande.
- **Frecuencia**: sesiones cortas y orientadas a tareas. No se mira "de fondo" todo el
  día. Prioriza claridad y velocidad de lectura por sobre lo llamativo.
- **Tono**: la UI está en **español rioplatense** (voseo: "Configurá", "Agregá",
  "Clickeá"). Mantener ese registro.

## 3. Estructura de la app (mapa de pantallas)

Navegación global (topbar) con 7 destinos:

| Ruta | Pantalla | Qué muestra |
|---|---|---|
| `/` | **Dashboard** | Grilla de 6 widgets (resúmenes clickeables). Home. |
| `/disk` | **Disco** | Uso de disco por carpeta + drill-down navegable (tipo `du`). |
| `/naming` | **Nombres** | Carpetas que no cumplen la convención «Título (Año)». |
| `/media` | **Medios** | Inventario de Plex + estado de `.nfo` por ítem. |
| `/subtitles` | **Subtítulos** | Medios sin subtítulo en español + buscar/descargar. |
| `/torrents` | **Torrents** | Tabla de descargas (estado, progreso, velocidades) + agregar magnet. |
| `/settings` | **Ajustes** | Carpetas vigiladas + credenciales de Plex/OpenSubtitles/qBittorrent. |

### 3.1 Dashboard — la grilla de widgets

Grilla responsive (`auto-fill, minmax(260px, 1fr)`). Seis widgets, cada uno una tarjeta
con título en mayúsculas, un botón chico de refresh (↻) arriba a la derecha, y un cuerpo:

1. **Sistema** — lista de métricas: CPU, RAM, Temp, Uptime, Load. (Se refresca solo.)
2. **Disco** — una fila por carpeta vigilada: nombre, % usado, barra de proporción,
   "X usados / Y total". Clickeable → `/disk`.
3. **Nombres** — número grande de carpetas mal nombradas (o un ✓ "todo cumple"). El
   refresh dispara un re-escaneo. Clickeable → `/naming`.
4. **Medios** — 3 stats: Ítems, Con .nfo, Sin subs ES. Clickeable → `/media`.
5. **Subtítulos** — número grande de medios sin subtítulo ES (o ✓). Clickeable → `/subtitles`.
6. **Torrents** — Activos (n/total), velocidad ↓, velocidad ↑. Clickeable → `/torrents`.

Cada widget puede estar en 3 estados: **configurado con datos**, **no configurado**
("Plex no configurado. Conectar."), o **error/sin conexión**. El diseño tiene que
manejar los tres con gracia.

### 3.2 Páginas de detalle — patrones recurrentes

- **Disco**: tabs por carpeta; por carpeta un número grande de % usado, botón "Escanear"
  (job en background con spinner + polling), lista de "carpetas más grandes" con barras,
  y un explorador drill-down (entrar carpeta por carpeta con ↑ Subir, mostrando tamaños).
- **Nombres**: tabla Tipo / Nombre actual / Sugerido, con las rutas en monospace.
- **Medios**: fila de 3 stats grandes, dos botones de acción (Sincronizar desde Plex /
  Generar .nfo, ambos jobs en background), y una tabla de inventario con badges sí/no.
- **Subtítulos**: lista de medios; cada uno con botón "Buscar" que despliega una tabla de
  resultados de OpenSubtitles, cada resultado con botón "Descargar".
- **Torrents**: form para pegar magnet arriba; tabla que se auto-refresca cada 3s con
  Nombre / Estado / Progreso (barra) / Tamaño / ↓ / ↑ / Seeds-Leechs / acciones
  (pausar/reanudar/borrar con confirmación).
- **Ajustes**: ABM de carpetas vigiladas + 3 bloques de formulario para credenciales
  (Plex, OpenSubtitles, qBittorrent), con inputs de password que muestran "guardado" si
  ya hay valor.

### 3.3 Componentes reutilizables que ya existen

- **Widget card** (tarjeta con head + refresh + body).
- **Barra de proporción** (`.bar` / `.bar-fill`) — usada en disco y torrents.
- **Job status / scan status** — fragmento de progreso: spinner + "Escaneando…" mientras
  corre (polling cada 2-3s), luego "Listo" y recarga la sección.
- **Badges** sí/no (verde/rojo), **notice** (aviso), **spinner**, **tabs**, **tablas**,
  **botones** (primario, secundario, `danger`), **forms**.
- **Lista de stats** (clave a la izquierda, valor tabular a la derecha).

## 4. Look & feel actual (punto de partida)

Tema **oscuro por defecto**, con variante light vía `prefers-color-scheme`. Paleta y
tokens actuales (en `web/static/styles.css`, ~186 líneas de CSS propio, sin framework):

```
--bg:        #0f1216   (fondo)          --text:   #e6eaf0
--surface:   #171c23   (tarjetas)       --muted:  #9aa7b6
--surface-2: #1e252e   (inputs/hover)   --accent: #4c8dff  (azul)
--border:    #2a323d                    --radius: 12px
```

- Tipografía del sistema (`system-ui`), monospace para rutas y nombres de archivo.
- Densidad media, bordes redondeados 12px, acentos azules, semántica de color:
  verde = ok, rojo = error/danger, ámbar (#ffb454) = advertencia (conteos de problemas).
- Es funcional y limpio pero **genérico / "bootstrap oscuro"**: poca personalidad,
  jerarquía visual plana, los widgets se ven todos iguales, poco uso del color con
  intención, transiciones mínimas.

## 5. Restricciones técnicas duras (no negociables para el diseño)

Estas condicionan lo que el diseño puede pedir. **Respetarlas.**

1. **Cero build step de front-end.** No hay bundler, ni Sass, ni PostCSS, ni Tailwind, ni
   npm. Es **un solo archivo CSS** escrito a mano, embebido en el binario Go con
   `//go:embed`. Cualquier propuesta debe poder expresarse como CSS plano (custom
   properties, grid, flexbox son bienvenidos; CSS moderno de browser está OK).
2. **Sin JavaScript propio.** La interactividad es **HTMX** (un `.js` vendorizado) +
   atributos `hx-*` en el HTML. No se puede pedir componentes que requieran JS a mano
   (carruseles complejos, drag&drop, gráficos JS, animaciones con librerías). Micro-
   interacciones vía CSS (transitions, `:hover`, `@keyframes`) sí.
3. **Todo self-contained y embebido.** Nada de CDNs, fuentes de Google Fonts remotas,
   imágenes externas ni assets sueltos. Si se quiere una fuente custom, hay que
   embeberla (peso del binario importa). Preferencia fuerte por fuentes del sistema.
4. **CSP estricta, mismo origen.** Sin recursos externos. SVG inline / data-URIs OK.
5. **Las vistas son `templ`** (componentes Go tipados que compilan a HTML). El markup
   está estructurado en componentes; rediseñar clases/estructura es posible pero el
   diseñador debería pensar en términos de los componentes listados en §3.3.
6. **Corre en una Raspberry Pi** servida por LAN: liviano es mejor. Nada que pese o
   trabe. El HTML se sirve con gzip.
7. **Iconografía**: hoy se usan **emojis y entidades HTML** (📁 📄 🗑 ▶ ⏸ ↻ ✓ ↑). Si se
   quiere un set de íconos, tienen que ser **SVG inline** embebidos (no icon-font
   remota). Mantenerlo mínimo.

## 6. Qué se quiere mejorar (objetivos del rediseño)

En orden de prioridad:

1. **Más personalidad y jerarquía visual.** Que no parezca un template genérico. Que el
   dashboard "se lea" de un vistazo: qué necesita atención (problemas de nombres, medios
   sin subs, disco lleno) tiene que saltar; lo que está OK, quedar tranquilo de fondo.
2. **Estados con más significado.** Diferenciar visualmente "todo bien" vs "hay N cosas
   para hacer" vs "no configurado" vs "error". Hoy son todos texto plano parecido.
3. **El dashboard como verdadero centro de control.** Los 6 widgets podrían tener
   tamaños/pesos distintos según importancia, mejores micro-visualizaciones (la barra de
   disco, sparklines de sistema si se puede sin JS, etc.).
4. **Densidad de datos legible.** Las tablas (torrents, medios, inventario) son el corazón
   de las páginas de detalle: mejorar su legibilidad, alineación numérica, escaneabilidad.
5. **Micro-interacciones y feedback.** Los jobs en background (escaneo, .nfo, subtítulos)
   necesitan feedback de progreso claro y agradable. Hoy es un spinner + texto.
6. **Coherencia del sistema visual.** Tokens de color/espaciado/tipografía consistentes,
   un ritmo vertical claro, estados de foco/hover accesibles.
7. **Accesibilidad**: contraste suficiente en ambos temas, foco visible, `aria-label` en
   los botones-ícono (ya hay algunos), navegable por teclado.

**No cambiar**: la información que muestra cada pantalla, el idioma/tono rioplatense, ni
la arquitectura de navegación (7 secciones). El rediseño es visual y de interacción,
sobre la misma estructura funcional.

## 7. Archivos relevantes para el diseño

- `web/static/styles.css` — todo el CSS actual (punto de partida).
- `web/templates/*.templ` — las vistas (layout, dashboard, widget, y una por pantalla).
- `web/templates/*.go` — view-models (structs con strings ya formateados que las vistas
  consumen; útil para entender qué datos hay disponibles en cada pantalla).
- `web/static/favicon.svg` — favicon actual.
- `docs/features.md` — spec funcional detallada de cada pantalla.
