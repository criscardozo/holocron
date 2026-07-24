# Handoff: Holocron — rediseño visual «Noir»

## Overview
Rediseño visual y de interacción del panel Holocron (dashboard + 7 pantallas de detalle) para el HTPC en Raspberry Pi. Misma arquitectura funcional y mismos datos que hoy; cambia solo el look & feel: dirección **Noir** — el layout tranquilo y con "glow" de un tema oscuro compacto, re-teñido a **negro + naranja**, tipografía **Archivo**, íconos SVG inline. En español rioplatense (voseo).

## About the Design Files
Los archivos de este bundle son **referencias de diseño hechas en HTML** — prototipos que muestran el aspecto y el comportamiento buscados, **no** código de producción para copiar tal cual. La tarea es **recrear estos diseños dentro del entorno del codebase Holocron existente**: vistas `templ` (Go tipado que compila a HTML), interactividad con **HTMX** (`hx-*`) y **un solo archivo CSS plano** embebido con `//go:embed`. No hay build step de front-end, ni bundler, ni JS propio, ni recursos externos (fuentes/CDN/imágenes). Todo lo que se propone abajo se expresa como CSS plano (custom properties, grid, flexbox) + SVG inline, compatible con esas restricciones y con la CSP estricta.

> Nota sobre fuentes: los prototipos cargan **Archivo** vía Google Fonts para poder verse en el navegador. En producción, respetando "self-contained y CSP estricta", hay que **embeber Archivo** (woff2 vía `//go:embed` + `@font-face`) o, si el peso del binario preocupa, caer a la stack del sistema. No dejar el `@import` remoto.

## Fidelity
**Alta fidelidad (hifi).** Colores, tipografía, espaciado, estados y micro-interacciones son finales. Recrear la UI pixel-perfect usando los tokens y componentes que se listan acá. El único ítem no-productivo es la carga remota de la fuente (ver nota arriba).

## Design Tokens
Definidos como custom properties. Los tokens de tema Noir **sobre-escriben** los del tema base; el resto (espaciado, radios, sombras, rampas) viene del sistema base y se reutiliza tal cual.

### Color (tema Noir)
```
--color-bg:        #121110   fondo
--color-surface:   #1a1817   tarjetas
--color-surface-2: #221f1d   inputs / hover / job status
--color-text:      #f1ede9   texto
--color-divider:   color-mix(in srgb, #f1ede9 12%, transparent)
--color-accent:    #ff6a2b   naranja (línea, glow, barras, primary)
--color-accent-200:#ffcbaf   texto sobre tinte de acento
--color-accent-300:#ffb088   links / texto de acento
--color-accent-400:#ff8a4d   gradiente "hot" de barras
--color-accent-800:#48250f   tinte de fondo (chips/estados)
--color-accent-900:#331808   tinte de fondo más profundo
```
Semántica de estado:
```
--ok:     #6bbf8f   verde (sí / sembrando / online / guardado)
--danger: #e08a8a   rojo suave (no / borrar / error)
--warn:   var(--color-accent-300)   ámbar-naranja (conteos de problemas)
```

### Tipografía
- Familia única: **Archivo** (headings y body), pesos 400/500/600/700.
- H1 de página: 30px / 600. Título de widget (`.section-title`, `.w-title`): 12px / 600 / uppercase / `letter-spacing:.1em` / color texto al ~55%.
- Números grandes (`.big-num`): 64px / 700 / `letter-spacing:-.03em`, color acento.
- Números tabulares en todo dato numérico: `font-variant-numeric: tabular-nums`.
- Monospace para rutas y nombres de archivo (`--mono: ui-monospace, "SF Mono", Menlo, monospace`, 12.5px).

### Espaciado / radios / sombras
Escala de espaciado del sistema base (`--space-1:4 · -2:8 · -3:12 · -4:16 · -6:24 · -8:32`). Radios: `--radius-md` ≈ 8px (soft). Elevación: `--shadow-sm/md/lg` del sistema (borde + oscuridad ambiente, sin sombras pesadas). Usar siempre las variables, no números crudos.

### Iconografía
Set mínimo de **SVG inline** (stroke, 1.75, currentColor), embebido como sprite `<symbol>` por página y referenciado con `<use href="#id">`. Íconos usados: diamond (marca), refresh, drive, film, cap (subtítulos), activity, alert, check, folder, file, up, chev, adown/aup (velocidades), play, pause, trash, plus, search, download, plug (Plex), doc (.nfo), down-cloud (qBittorrent), key. Tamaños: `.ic` 18px, `.ic-sm` 14px. Reemplazan a los emojis/entidades actuales.

## Screens / Views

Chrome compartido en todas:
- **Topbar** (`.nav`): sticky, `backdrop-filter: blur(8px)`, borde inferior `--color-divider`. Izquierda: marca (diamante naranja + "Holocron") + tag "HTPC Manager". Derecha: 7 links; el activo lleva `aria-current="page"` y un subrayado naranja de 2px. 7 destinos: Dashboard, Disco, Nombres, Medios, Subtítulos, Torrents, Ajustes.
- **`.page`**: `max-width:1240px`, centrado, padding `--space-8`.
- **`.page-head`**: H1 + subtítulo `.sub` a la izquierda; acciones a la derecha.

### 1. Dashboard (`/`)
- **Propósito**: leer de un vistazo qué necesita atención y saltar a cada sección.
- **Layout**: tira "Atención" (chips clickeables a las secciones con problema) + grilla de 4 columnas (`repeat(4,1fr)`, gap `--space-4`) con pesos distintos: Sistema y Disco `span 2`; Nombres y Subtítulos 1 col; Medios `span 2`; Torrents `span 4`. Responsive: colapsar a menos columnas en pantallas chicas.
- **Widgets** (tarjeta `.card .elev-sm`, head con título + botón refresh ↻ `.btn-ghost.btn-icon`):
  - **Sistema**: `stat-list` (CPU/RAM/Temp/Uptime/Load) + sparkline CSS (barras flex, última en acento) con número grande y "En línea".
  - **Disco**: fila por carpeta (nombre + %, barra de proporción, "X usados / Y"); la carpeta caliente (≥90%) usa barra con gradiente `hot` y % en acento. Total abajo. Clickeable → /disk.
  - **Nombres** (widget de atención, `inset 3px` de acento a la izquierda): número grande + "carpetas con nombre inválido" + link "Revisar". Estado ✓ si todo cumple.
  - **Subtítulos** (atención): número grande + "medios sin subtítulo en español" + link.
  - **Medios**: trío de stats (Ítems / Con .nfo / Sin subs ES) + 2 botones (primary "Sincronizar desde Plex", secondary "Generar .nfo").
  - **Torrents**: resumen (activos n/total, ↓, ↑) + tabla compacta con barra de progreso y acciones.

### 2. Disco (`/disk`)
Tabs por carpeta vigilada. Gauge grande de % usado + `.bar` + "usados/libres/total". Botón "Escanear" (job en background: spinner + polling, luego "Escaneado hace…"). Tarjeta "Carpetas más grandes" (barras). Tarjeta "Explorador": breadcrumb monospace + botón ↑ Subir + filas `du`-style (ícono carpeta/archivo, nombre mono, mini-barra, tamaño); las filas de carpeta son clickeables (drill-down).

### 3. Nombres (`/naming`)
`notice` de resumen ("N carpetas con nombre inválido", patrón `Título (Año)`). Grupos por biblioteca (Películas / Series), cada uno con tabla Tipo / Nombre actual (mono, rojo) / Sugerido (mono, verde) + motivo `.why` bajo el actual. Botón "Re-escanear" (job).

### 4. Medios (`/media`)
Banda con trío de stats grandes + acciones (primary "Sincronizar desde Plex" con ícono plug, secondary "Generar .nfo" con ícono doc). `job` de progreso ("Generando .nfo… N de M"). Tabla de inventario: Título (con ruta mono debajo) / Tipo / Año / .nfo (badge sí/no) / Subs ES (badge sí/no).

### 5. Subtítulos (`/subtitles`)
Badge de pendientes en el head. Lista de tarjetas, una por medio (título + ruta mono + botón primary "Buscar"). Al buscar, la tarjeta despliega `results`: tabla de OpenSubtitles (Release mono / Descargas / Puntaje / botón secondary "Descargar" por fila). En el prototipo la primera está expandida.

### 6. Torrents (`/torrents`)
Form arriba (`.card`): ícono +, input "Pegá un magnet: aquí…", botón primary "Agregar". Resumen (activos, ↓, ↑) + indicador "En vivo" con punto que pulsa (auto-refresh cada 3s). Tabla: Nombre (mono) / Estado (pill: Descargando/Sembrando/Pausado/Error) / Progreso (barra + %) / Tamaño / ↓ / ↑ / S/L / acciones (pausar-reanudar + borrar en `--danger`).

### 7. Ajustes (`/settings`)
Grilla de 2 columnas. Bloque ancho "Carpetas vigiladas": ABM (filas con badge de tipo + ruta mono + borrar; fila de alta con input + selector de tipo + "Agregá carpeta"). Tres bloques de credenciales: **Plex** (URL + Token), **OpenSubtitles** (Usuario + API key), **qBittorrent** (URL + Usuario + Contraseña). Los inputs `type=password` muestran una etiqueta "✓ guardado" en verde cuando ya hay valor. Botón "Guardá" por bloque.

## Componentes reutilizables (clases)
Ver `noir.css` para el detalle. Del sistema base se reusan: `.card`/`.elev-sm`, `.btn`+`.btn-primary`/`.btn-secondary`/`.btn-ghost`/`.btn-icon` (el **primary es outline de acento, no relleno**), `.table`, `.input`, `.field`. Propias del rediseño (en `noir.css`):
- `.bar` / `.bar-fill` (+ `.bar.hot` gradiente) — barra de proporción (disco, torrents, explorador).
- `.badge` (`-yes` verde / `-no` rojo / `-warn` naranja / `-neutral`) — sí/no e inventario.
- `.st` (`-dl` / `-seed` / `-pause` / `-err`) — pill de estado de torrent.
- `.notice` — aviso (patrón de nombres, "no configurado").
- `.job` (+ `.spinner`, `.job.done`) — feedback de jobs en background.
- `.tabs` / `.tab.active` — tabs por carpeta en Disco.
- `.stat-list`, `.stat-trio`, `.big-num`, `.section-title`, `.prog-cell`, `.row-act`, `.mono`, `.num`, `.muted`.

## Interactions & Behavior
- **Navegación**: links de la topbar (hoy con `hx-boost`). Widgets clickeables llevan a su detalle.
- **Refresh de widget**: botón ↻ → `GET /widgets/{id}`, reemplaza el fragmento (HTMX `hx-get` + `hx-target`).
- **Jobs en background** (disk-scan, nfo-generate, naming-scan, subtitles-search): botón dispara `POST`, se muestra `.job` con `.spinner` + texto "Escaneando…/Generando…", polling cada 2-3s, al terminar `.job.done` con ✓ y recarga de la sección.
- **Torrents**: tabla con auto-refresh cada 3s (HTMX `hx-trigger="every 3s"`); acciones `POST /torrents/{hash}/{pause|resume|delete}` (borrar con confirmación). Alta por magnet: `POST /torrents/add` con validación básica del esquema `magnet:?`.
- **Subtítulos**: "Buscar" → `POST /subtitles/search` despliega resultados; "Descargar" → `POST /subtitles/download`.
- **Ajustes**: `POST /settings` por bloque; ABM de carpetas.
- **Estados por widget/pantalla** (diferenciar visualmente, no todo texto plano igual): **con datos** · **no configurado** (usar `.notice`: "Plex no configurado. Conectá.") · **error/sin conexión** (`.st-err` / `.badge-no` / notice en danger). Alta prioridad de diseño: que "todo bien" quede tranquilo de fondo y "hay N cosas para hacer" salte (acento/ámbar + `inset` de acento en la tarjeta).
- **Micro-interacciones**: transiciones de hover en filas/botones; punto "En vivo" que pulsa; spinner del job. Todo CSS (`transition`, `@keyframes`), sin JS.

## Estados de foco / accesibilidad
- `:focus-visible` con anillo de acento (2px, offset 2px); no dejar el azul del browser.
- `aria-label` en todos los botones-ícono (refresh, pausar/reanudar, borrar, subir nivel).
- Contraste suficiente en el tema oscuro; para texto tamaño párrafo en acento usar el step profundo (`--color-accent-300`), no el acento puro.
- Navegable por teclado; `aria-current="page"` en el link activo.

## State Management
Sin estado de front-end propio (no hay JS). El estado vive en el server (Go) y llega renderizado por `templ`; HTMX intercambia fragmentos. Variables relevantes por pantalla: resultado de escaneo (disco/nombres), inventario + flags `has_subs_es`/`nfo_written_at` (medios), lista de torrents (polling), credenciales guardadas (mostrar "guardado" sin exponer el valor).

## Assets
- Íconos: **SVG inline** propios (sprite `<symbol>`), sin icon-font remota. Reemplazan emojis/entidades del actual.
- Fuente: **Archivo** — embeber woff2 en producción (ver nota).
- Sin imágenes externas.

## Screenshots
Capturas de referencia de cada pantalla en `screenshots/` (01-dashboard … 07-ajustes). Nota: en las capturas los íconos SVG inline pueden verse como puntos rellenos — es un artefacto del capturador; en el navegador renderizan como line-icons.

## Files (en este bundle)
- `dash-noir.html` — Dashboard (entrypoint; navegá desde acá).
- `disco.html`, `nombres.html`, `medios.html`, `subtitulos.html`, `torrents.html`, `ajustes.html` — pantallas de detalle.
- `noir.css` — tokens del tema Noir + chrome (nav, page) + componentes propios. **Fuente de verdad de estilos** del rediseño.
- Los prototipos linkean además la hoja de tokens/componentes del sistema base (buttons/card/table/input); en producción, portar esos valores al único `styles.css` embebido.

## Cómo mapear al codebase actual
1. Trasladar los tokens Noir + `noir.css` al único `web/static/styles.css` (mantener un solo archivo, CSS plano).
2. Ajustar los componentes `templ` existentes (widget card, `.bar`/`.bar-fill`, job status, badges, notice, spinner, tabs, tablas, botones, forms) a estas clases/tokens — la estructura de componentes no cambia, sí su estilo.
3. Reemplazar emojis/entidades por el sprite SVG inline.
4. Embeber Archivo (o caer a system-ui) y quitar cualquier carga remota. Verificar CSP.
5. No introducir JS propio: todo el comportamiento sigue en HTMX + CSS.
