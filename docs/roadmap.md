# Roadmap por fases

Se construye escalado: cada fase deja algo usable y desplegable en la Pi antes de
pasar a la siguiente. Los números de feature refieren a los pedidos originales.

## Estado

Fases 0 a 5 implementadas, con tests (incluido `-race`) y binario `arm64`
cross-compilado. Ver «Ya sumado después de las fases» al final para lo que
llegó más tarde (rediseño Noir, CI, releases, API JSON, app iOS y la migración
del módulo de medios de Plex a Jellyfin).

## Fase 0 — Fundaciones

Base sobre la que se apoya todo. No entrega ninguna herramienta todavía, pero sí un
dashboard vacío que arranca en la Pi.

- Scaffold del proyecto (`go.mod`, estructura de carpetas, `Makefile`, git init).
- `config` (flags + env), `db` (SQLite + migraciones embebidas), arranque en
  `cmd/holocron/main.go` con graceful shutdown.
- `httpserver` con router stdlib, middleware (logging, gzip, headers) y assets
  embebidos.
- Layout base con templ + HTMX vendorizado + CSS.
- Framework de `widgets` y de `jobs`.
- **Widget de sistema**: CPU, RAM, temperatura, uptime y load de la Pi (lee `/proc`
  y `/sys/class/thermal`). Primer contenido real del dashboard.
- Pipeline de build y deploy (cross-compile + `scripts/deploy.sh`, unit de systemd).
- Tooling de calidad: `Makefile` con `make check` (test `-race -shuffle=on`, `vet`,
  `golangci-lint`, `govulncheck`, `gosec`) y `.golangci.yml`.

## Fase 1 — Dashboard + uso de disco (features 1 y 2)

- Grilla de widgets funcional (feature 1).
- **Widget de disco**: espacio libre/ocupado de una carpeta configurada. Click →
  página de detalle.
- **Página de detalle de disco**: escanea la carpeta y subcarpetas, muestra archivos
  y peso, con drill-down navegable.
- Portado del paquete `scanner` desde `diskusage-pi` (tamaños reales tipo `du`, stats
  vía `statfs`, drill-down seguro), integrado con `jobs` y cacheado en `scan_results`.
- ABM de carpetas vigiladas (`watched_folders`) desde la UI.

## Fase 2 — Validador de nombres (feature 3)

- `naming`: recorre las carpetas de Películas/Series y detecta las que no cumplen
  `Título (Año)`.
- **Widget** con botón de refresh chico a la derecha, que muestra el conteo de
  incumplimientos. Click → pantalla de errores.
- **Pantalla de errores**: lista de carpetas mal nombradas con lo esperado vs. lo
  encontrado. (Posible fase futura: renombrado asistido.)

## Fase 3 — Integración con el servidor de medios (feature 4a)

> Esta fase se construyó contra **Plex** y después se migró a **Jellyfin** (ver
> «Ya sumado» al final): el HTPC dejó de usar Plex. La forma de la fase no
> cambió, sí el cliente.

- Cliente del servidor de medios (`plex` en su momento, hoy `jellyfin`): auth sin
  copiar tokens a mano e inventario de la biblioteca.
- Generación de `.nfo` con la metadata que el servidor ya resolvió. **Quitada
  después**: Jellyfin escribe esos mismos archivos, y dos escritores sobre una
  ruta significa que gana el último.
- Inventario en `media_items`.
- Widget de estado del inventario + acción de sincronizar.

## Fase 4 — Subtítulos con OpenSubtitles (feature 4b)

- `subtitles`: detección de subtítulos presentes (busca `.srt`/`.ssa`/`.sub` o lo
  declarado por Jellyfin, embebido o aparte) y cliente de la API de
  [OpenSubtitles](https://opensubtitles.stoplight.io/docs/opensubtitles-api).
- **Widget** que lista películas/series sin subtítulos.
- Búsqueda y descarga de subtítulos desde la UI.

## Fase 5 — qBittorrent (features 5 y 6)

- `qbittorrent`: cliente de la
  [WebUI API](https://github.com/qbittorrent/qBittorrent/wiki/WebUI-API-(qBittorrent-4.1))
  (login por cookie, listar/pausar/reanudar/borrar torrents).
- **Pantalla de torrents**: administración (estado, progreso, velocidades, acciones).
- **Agregar magnet-links** para descargar por BitTorrent.
- Widget de resumen (descargas activas, velocidad total) en el dashboard.

## Ya sumado después de las fases

- **Rediseño visual «Noir»** (negro + naranja, íconos SVG, jerarquía por
  atención). Ver [ui.md](ui.md).
- **CI en GitHub Actions**: vet, tests con `-race`, `golangci-lint`, `govulncheck`
  y chequeos de `go mod tidy`/`templ generate` en cada push y PR.
- **Releases automáticas** por tag `v*` (binario `arm64` + checksum) e
  **instalador/actualizador** para la Pi headless (`scripts/install.sh`).
- **API JSON + app iOS** nativa (ver [api.md](api.md) y `ios/`).
- **Categorías de qBittorrent** al agregar un magnet, y visibles por torrent.
- **Migración de Plex a Jellyfin**: el HTPC dejó de usar Plex, así que el módulo
  de medios apuntaba a un servidor apagado. Auth por **Quick Connect** (un código
  de 6 dígitos que se aprueba en Jellyfin, sin copiar API keys), y los subtítulos
  salen de la API — Jellyfin ya conoce cada pista de cada archivo — en vez de
  recorrer el disco por título. Los paquetes `plex`/`plexauth` se eliminaron.
- **Actualización desde la UI**: chequeo contra las releases de GitHub y botón de
  instalar, vía un helper de systemd con privilegios (el servicio no puede
  reemplazarse a sí mismo).

## Ideas para más adelante (fuera de alcance inicial)

- Renombrado asistido de carpetas mal nombradas (Fase 2).
- Control del servicio de Jellyfin (reiniciar, refrescar bibliotecas).
- Notificaciones (descargas terminadas, disco casi lleno).
- Basic Auth opcional (con comparación constant-time).
