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

## Feature 4a — Inventario de medios desde Jellyfin

Holocron inventaría la biblioteca desde Jellyfin: título, año, identificadores
externos y si hay subtítulos en español. **Los subtítulos los reporta Jellyfin**,
que ya conoce cada pista de cada archivo (incluidas las embebidas), así que no se
recorre el disco para averiguarlo.

> Holocron **no escribe `.nfo`**. Lo hizo hasta la v0.4.1: Jellyfin escribe los
> mismos archivos cuando la biblioteca tiene activado «guardar metadata», y dos
> escritores sobre la misma ruta significa que gana el último. El dueño es
> Jellyfin; Holocron sólo lee.

- **Cliente** (`jellyfin`): Quick Connect para el token, e inventario con
  `GET /Items` **sin** `userId` — al filtrar por usuario, Jellyfin esconde las
  películas que pertenecen a una colección (46 de 344 en la biblioteca real) y a
  la vez devuelve las colecciones como si fueran títulos. Se descartan los ítems
  sin `Path`: son los que existen en el proveedor de metadata pero no en disco.
- **Datos**: `media_items` (`path, type, title, year, server_item_id,
  provider_ids, has_subs_es`). El sync corre por `jobs` (`kind: media-sync`).
- **Endpoints**: `GET /media` (inventario), `POST /media/sync` (traer de
  Jellyfin), y en Ajustes `POST /settings/jellyfin` +
  `/settings/jellyfin/link` para la dirección y el Quick Connect.
- **A cuidar**: `ProviderIds` se modela como mapa, no como struct, porque trae
  claves con espacios (`"official website"`). Un título puede tener varios
  archivos (misma película en 1080p y 4K) y los subtítulos cuelgan de uno solo,
  así que hay que unir las pistas de todos.

---

## Feature 4c — Panel de calidad de biblioteca

**Pantalla** con cinco contadores, cada uno con su lista. Responde «qué le falta
a la biblioteca», que es distinto de «cuánto hay»:

| Categoría | Qué es |
|---|---|
| Sin subtítulos ES | Ni pista en español ni audio en español |
| Sin sinopsis | Jellyfin no tiene descripción del ítem |
| Título genérico | `Episode 4`, o el nombre del archivo: no lo identificó |
| Fantasmas | Jellyfin lo lista y el archivo ya no está |
| Numeración repetida | Dos episodios con el mismo `SxxEyy`; uno queda tapado |

- **Datos**: un único documento JSON en `quality_reports` (`CHECK (id = 1)`),
  reemplazado por cada análisis. Cubre **episodios**, que no están en
  `media_items` — meter dos mil filas de episodio ahí cambiaría lo que significa
  el inventario.
- **Análisis** (`quality`): `jobs` con `kind: quality-scan`. Pide a Jellyfin
  `Movie,Series,Episode` **paginado** (los episodios traen sus pistas, así que
  pedir dos mil de una es un decode de varios MB en la Pi) y clasifica en
  memoria. `Analyse` es una función pura sobre la respuesta, así que las reglas
  se testean contra respuestas capturadas y no contra un servidor prendido.
- **Reglas que importan**: una serie es un directorio y no tiene pistas propias,
  así que **no** se le pregunta por subtítulos (sería un falso hallazgo por cada
  serie). Un fantasma se reporta una sola vez, como fantasma. Dos episodios «sin
  número» no colisionan entre sí.
- **Acción**: donde el problema es metadata (sin sinopsis, título genérico) hay
  un botón que le pide a Jellyfin un `FullRefresh` del ítem. Requiere cuenta
  administradora —se avisa antes, no después del 403— y el id se valida contra
  el informe vigente antes de mandar nada.
- **Tope**: se muestran 200 hallazgos por categoría; los contadores son los
  totales reales y la UI dice cuándo la lista está cortada.

---

## Feature 4b — Búsqueda de subtítulos (OpenSubtitles)

**Widget**: lista de películas/series **sin** subtítulos. Desde ahí se pueden buscar
y descargar.

- **Detección de subtítulos presentes** (`subtitles`): para cada medio, se considera
  que tiene subtítulos si existe un archivo `.srt`/`.ssa`/`.sub` junto al video, o si
  Jellyfin declara una pista de subtítulos (embebida o aparte).
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
