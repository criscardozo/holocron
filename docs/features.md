# Especificación de features

Detalle de cada feature: qué hace, qué endpoints/pantallas expone, qué guarda y qué
reutiliza. El orden de construcción está en [roadmap.md](roadmap.md).

---

## Feature 1 — Dashboard con grilla de paneles

Pantalla principal: una grilla responsive de widgets. Cada widget muestra un resumen
y, según el caso, un botón de refresh chico arriba a la derecha y/o un link a su
página de detalle.

- **UI**: grilla CSS. Cada widget es un componente templ con un fragmento
  refrescable vía HTMX.
- **Endpoint**: `GET /` (dashboard), `GET /widgets/{id}` (fragmento de un widget).
- **Base para**: todas las demás features enganchan acá su widget.

---

## Feature 2 — Uso de disco de una carpeta

**Widget**: espacio libre/ocupado de una carpeta que el usuario configura. Click →
página de detalle.

**Página de detalle**: escanea la carpeta y subcarpetas y muestra archivos y peso,
con drill-down navegable (entrar carpeta por carpeta viendo el tamaño de cada hijo).
Equivale a lo que hace `diskusage-pi`.

- **Reutiliza**: paquete `scanner` de `diskusage-pi`. Ya calcula tamaños asignados
  reales (`st_blocks × 512`, como `du`), stats del filesystem con `syscall.Statfs`,
  top-N de carpetas, y drill-down (`Browse`) con validación anti path-traversal.
- **Datos**: `watched_folders` (carpetas elegidas), `scan_results` (cache del último
  escaneo). Escaneo vía `jobs` (`kind: disk-scan`) con progreso.
- **Endpoints**: `GET /disk` (detalle), `GET /disk/browse?path=…` (drill-down),
  `POST /disk/scan` (dispara escaneo), config de carpetas en `GET/POST /settings`.

---

## Feature 3 — Validador de convención "Título (Año)"

**Widget**: botón de refresh chico a la derecha; muestra cuántas carpetas de
Películas/Series no cumplen la norma `Título (Año)`. Click → pantalla de errores.

**Pantalla de errores**: lista de carpetas mal nombradas, con lo esperado vs. lo
encontrado, agrupadas por biblioteca.

- **Lógica** (`naming`): recorre el primer nivel de las carpetas de biblioteca
  configuradas y valida cada nombre contra un patrón `^.+ \(\d{4}\)$` (con matices:
  detectar año presente pero mal formateado, sufijos como `{edition-...}`, etc.).
- **Datos**: `naming_issues` (`path, type, expected, found, resolved`).
- **Endpoints**: `GET /naming` (pantalla), `POST /naming/scan` (refresh), `GET
  /widgets/naming` (fragmento del widget).
- **Futuro**: renombrado asistido (sugerir el nombre correcto y aplicarlo).

---

## Feature 4a — Generación de archivos .nfo desde Plex

Plex Server no mantiene un `.nfo` por película/serie. Holocron recorre cada
directorio de medio y escribe un `.nfo` con la metadata que Plex ya resolvió,
incluyendo un campo que indica si hay subtítulos en español.

- **Reutiliza**: `plex` y `plexauth` de `plexmatch-generator`. El cliente ya lista
  bibliotecas, pagina por headers (`X-Plex-Container-Start/Size`), y trae por item
  `title`, `year`, `guid`, `ratingKey`, `type` y las rutas de archivo
  (`Media[].Part[].file` para películas, `Location[].path` para series). El login por
  PIN + autodescubrimiento de servidor en LAN ya está resuelto.
- **`.nfo`**: formato XML estándar de Kodi/Jellyfin (`<movie>`, `<tvshow>`), que Plex
  también puede leer con el agente adecuado. Incluye `<title>`, `<year>`,
  identificadores, y `<subtitle language="spa">` según detección.
- **Datos**: `media_items` (`path, type, title, year, plex_guid, has_subs_es,
  nfo_written_at`). Generación vía `jobs` (`kind: nfo-generate`).
- **Endpoints**: `GET /media` (inventario + estado de .nfo), `POST /media/nfo`
  (generar), `GET/POST /settings` para credenciales/URL de Plex.
- **A cuidar**: el fix de `AltGUIDs` (`json:"Guid"` como array aparte del `guid`
  string) ya documentado en el port en Go; el remapping de rutas host↔Plex si Plex
  reporta paths distintos a los del filesystem local.

---

## Feature 4b — Búsqueda de subtítulos (OpenSubtitles)

**Widget**: lista de películas/series **sin** subtítulos. Desde ahí se pueden buscar
y descargar.

- **Detección de subtítulos presentes** (`subtitles`): para cada medio, se considera
  que tiene subtítulos si existe un archivo `.srt`/`.ssa`/`.sub` junto al video, o si
  el `.nfo` declara subtítulo (embebido o aparte).
- **Búsqueda/descarga**: cliente de la
  [API de OpenSubtitles](https://opensubtitles.stoplight.io/docs/opensubtitles-api/e3750fd63a100-getting-started).
  Requiere API key (se guarda en `settings`). Búsqueda por título/año o por hash del
  archivo; descarga del `.srt` al directorio del medio.
- **Endpoints**: `GET /subtitles` (faltantes), `POST /subtitles/search`,
  `POST /subtitles/download`.

---

## Feature 5 — Administración de torrents (qBittorrent)

Administra qBittorrent (qbittorrent-nox) vía su
[WebUI API](https://github.com/qbittorrent/qBittorrent/wiki/WebUI-API-(qBittorrent-4.1)).

- **Cliente** (`qbittorrent`): login (`/api/v2/auth/login`, cookie de sesión), listar
  (`/api/v2/torrents/info`), pausar/reanudar/borrar, ver estado global.
- **Pantalla**: tabla de torrents con estado, progreso, velocidades, seeds/peers y
  acciones. Widget de resumen (activos + velocidad total) en el dashboard.
- **Datos**: URL y credenciales de qBittorrent en `settings`.
- **Endpoints**: `GET /torrents`, `POST /torrents/{hash}/{action}`.

---

## Feature 6 — Agregar magnet-links

Agregar descargas por magnet-link desde la UI.

- Usa `/api/v2/torrents/add` de la WebUI API (acepta `urls` con el magnet).
- **UI**: un input para pegar el magnet (validación básica del esquema `magnet:?`),
  opción de categoría/carpeta de destino.
- **Endpoint**: `POST /torrents/add`.
