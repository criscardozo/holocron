# Arquitectura y decisiones técnicas

Este documento fija las decisiones estructurales de Holocron. Las features
concretas se describen en [features.md](features.md); el orden de construcción en
[roadmap.md](roadmap.md).

## 1. Principios

1. **Un binario, cero dependencias en destino.** La Raspberry Pi solo recibe un
   ejecutable. No se instala Go, ni Node, ni compiladores, ni librerías del sistema.
2. **Se compila en la MacBook.** Todo el toolchain (Go, `templ generate`) vive en la
   máquina de desarrollo. Ver [Compilación y despliegue](#compilación-y-despliegue).
3. **Sin CGO.** Esto es lo que hace posible el punto 1 y 2: cross-compilar a ARM sin
   un toolchain de C. Condiciona la elección de librerías (ver SQLite).
4. **Bajo demanda.** Los escaneos y trabajos pesados los dispara el usuario; no hay
   procesos corriendo en loop consumiendo la Pi. Los resultados se cachean.
5. **Modular.** Cada feature es un paquete `internal/` independiente + uno o más
   widgets en el dashboard. Sumar una feature no toca a las demás.

## 2. Stack

| Capa | Elección | Motivo |
|---|---|---|
| Lenguaje | Go 1.26 | Liviano, binario estático, cross-compila a ARM. |
| HTTP | `net/http` (stdlib) | Router con métodos de Go 1.22 (`GET /ruta`). Sin framework. |
| Vistas | [templ](https://templ.guide) | Componentes tipados que compilan a Go. Se genera en la Mac. |
| Interactividad | [HTMX](https://htmx.org) | Updates parciales (refresh de widgets) sin escribir JS ni SPA. Se vendoriza embebido (~14 KB). |
| Estilos | CSS moderno propio (custom properties) | Sin build step. Igual criterio que `diskusage-pi`. |
| Persistencia | SQLite vía `modernc.org/sqlite` | Driver **Go puro, sin CGO**. Un archivo, cero servidor de DB. |
| Logging | `log/slog` (stdlib) | Estructurado, sin dependencias. |

**Dependencias externas totales: dos** (`github.com/a-h/templ` y
`modernc.org/sqlite`). Todo lo demás es stdlib. HTMX no es una dependencia de Go: es
un `.js` vendorizado y embebido.

> **Nota sobre SQLite:** el driver popular `mattn/go-sqlite3` **queda descartado**
> porque requiere CGO y un compilador C en el destino, lo que rompería la
> cross-compilación. `modernc.org/sqlite` es una traducción del motor a Go puro.

## 3. Estructura del proyecto

```
holocron/
  cmd/holocron/
    main.go                 # arranque, flags/env, señales, graceful shutdown
  internal/
    config/                 # config del server (flags + env)
    db/                     # conexión SQLite, migraciones embebidas
    jobs/                   # runner de trabajos en background (estado, progreso, shutdown)
    system/                 # stats de la Pi: CPU, RAM, temperatura, uptime, load
    scanner/                # uso de disco (portado de diskusage-pi), Browse con os.Root
    diskusage/              # orquesta scanner + folders + cache en scan_results
    folders/                # store de carpetas vigiladas (disk | movies | tv)
    naming/                 # validador de convención "Título (Año)" + service
    settings/               # store key/value de credenciales de servicios
    netaddr/                # normaliza las direcciones que se escriben en Ajustes
    jellyfin/               # cliente de Jellyfin: Quick Connect, inventario, subtítulos
    library/                # sync de inventario Jellyfin→media_items
    quality/                # informe de calidad de la biblioteca (huérfanos, metadata, numeración)
    subs/                   # detección de subtítulos presentes junto al medio
    opensubtitles/          # cliente de la API v1 de OpenSubtitles
    subtitles/              # medios sin subs ES + búsqueda/descarga
    qbittorrent/            # cliente de la WebUI API de qBittorrent
    torrents/               # service sobre qbittorrent (config desde settings)
    widgets/                # registro de widgets del dashboard
    apitoken/               # token de la API JSON (genera, revoca, verifica)
    version/                # versión estampada en el build (ldflags)
    updates/                # chequeo de releases en GitHub + pedido de instalación
    httpserver/             # rutas, middleware, handlers por feature, API JSON
  ios/                      # app iOS en SwiftUI (proyecto generado con xcodegen)
  web/
    templates/              # archivos .templ + view-models (structs ya formateados)
    static/                 # htmx.min.js, styles.css, favicon — embebidos con //go:embed
  docs/                     # esta documentación (en español)
  packaging/                # unit de systemd de referencia
  scripts/                  # install.sh (instala/actualiza en la Pi), deploy.sh (scp)
  .github/workflows/        # CI (test/lint/vulncheck) y release (binario arm64)
  Makefile
  go.mod
```

El paquete `scanner` se **porta** desde `diskusage-pi`, no se reescribe de cero.
Se adapta a la interfaz de `jobs` y a las tablas de `db`. Los clientes de Plex
(`plex`, `plexauth`) existieron hasta que el HTPC migró a Jellyfin y se
eliminaron: quedaba código que no corría contra nada.

## 4. Modelo de datos (SQLite)

La base vive en un único archivo (por defecto `~/.local/share/holocron/holocron.db`,
configurable). Migraciones SQL embebidas con `//go:embed`, aplicadas al arrancar.

Tablas previstas (crecen por fase):

- `settings` — pares clave/valor para config editable desde la UI (rutas de
  bibliotecas, API keys, dirección y token de Jellyfin, credenciales de qBittorrent).
- `watched_folders` — carpetas que el usuario elige vigilar (para el widget de disco
  y el validador de nombres): `id, label, path, purpose`.
- `scan_results` — cache del último escaneo de disco por carpeta (JSON + timestamp).
- `naming_issues` — resultados del validador de nombres: `path, type, expected,
  found, resolved`.
- `media_items` — inventario de películas y series: `path, type, title, year,
  server_item_id, provider_ids, has_subs_es`.
- `quality_reports` — el último informe de calidad, como un único documento JSON
  (`CHECK (id = 1)`): cubre episodios, que no están en `media_items`.
- `jobs` — trabajos en background: `id, kind, status, progress, error, started_at,
  finished_at, result`.

> Config sensible (API keys, tokens) se guarda en SQLite con permisos owner-only
> sobre el archivo de la DB. No se versiona.

## 5. Trabajos en background (`jobs`)

Varias features son lentas (escanear un disco grande, auditar la biblioteca
entera, buscar subtítulos contra una API). No pueden bloquear un request HTTP.

`jobs` provee:

- Lanzar un trabajo por su `kind` (p. ej. `disk-scan`, `media-sync`,
  `quality-scan`).
- Estado con máquina simple: `idle → running → done | error`, con `progress`
  (0–100) y contadores, inspirado en el cache de estados de `diskusage-pi-claude`.
- La UI hace polling con HTMX (`hx-trigger="every 2s"`) a un endpoint de estado
  mientras el trabajo corre, y muestra el resultado al terminar.
- Concurrencia acotada (un worker por `kind`, para no saturar la Pi).

## 6. Dashboard y widgets (`widgets`)

El dashboard es una grilla de widgets. Cada widget implementa una interfaz común:

- `Render()` → fragmento HTML (templ) con el resumen y, si aplica, un botón de
  refresh chico a la derecha.
- Un endpoint de refresh (`GET /widgets/{id}`) que devuelve solo el fragmento
  actualizado (HTMX lo swapea in-place).
- Un link opcional a una **página de detalle** (la pantalla completa de la feature).

Esto cumple los pedidos: paneles en grilla (feature 1), widget de disco clickeable
(2), widget de validación con refresh que linkea a la pantalla de errores (3), etc.

Patrones de HTMX a usar (fragmentos mínimos, sin JS propio):

- **`hx-boost`** en la navegación dashboard ↔ páginas de detalle: navegación tipo
  SPA sin recargar todo, con degradación elegante si no hay JS.
- **`hx-indicator`** para el estado "cargando" mientras corre un trabajo o refresca
  un widget (spinner chico en el widget).
- **Polling** con `hx-trigger="every 2s"` solo mientras un job está `running`; el
  fragmento deja de pedir solo cuando el job termina (el server devuelve el resumen
  final sin el atributo de polling).
- **`hx-confirm`** en toda acción destructiva (borrar torrent, quitar carpeta
  vigilada, borrar un torrent).
- Cada endpoint de fragmento devuelve **solo el HTML necesario**, no la página.

## 7. Configuración del server

Dos niveles, igual criterio que `diskusage-pi`:

- **Arranque** (flags + variables de entorno; los flags pisan al entorno):
  `--addr` (default `:8090`), `--db`, `--log-level`. Sin TLS por ahora (LAN de
  confianza, sin auth — decisión tomada).
- **Aplicación** (editable desde la UI, persistida en `settings`): rutas de medios,
  dirección y token de Jellyfin, credenciales de qBittorrent, API key de
  OpenSubtitles.

## 8. Seguridad

- **La interfaz web no tiene autenticación** (red interna de confianza). Sí la
  tiene la **API JSON** (`/api/v1`, ver [api.md](api.md)), porque la consume la
  app iOS y un teléfono se va de la LAN: bearer token generado con `crypto/rand`,
  almacenado **sólo como digest SHA-256** y comparado en tiempo constante
  (`crypto/subtle`). Ver `internal/apitoken`. Si algún día se quiere cerrar
  también la web, el mismo patrón sirve como middleware.
- **Path traversal**: el drill-down de disco (`GET /disk/browse?path=…`) es el único
  lugar donde una ruta llega desde el cliente, y se confina con **`os.Root`**
  (Go 1.24+). El handle del root queda **abierto durante toda la operación** y cada
  `stat`, `open`, `readdir` y walk recursivo pasa por él, así que el kernel bloquea
  el escape vía `..` o symlinks incluso si el árbol cambia en el medio (sin ventana
  TOCTOU). Se descarta el patrón `filepath.Clean` + `strings.HasPrefix` (frágil), y
  también usar `os.Root` sólo como validador previo. Tests:
  `internal/scanner/scanner_test.go`.
- **Escrituras derivadas del servidor de medios**: lo único que Holocron escribe
  en la biblioteca son subtítulos, y siempre en rutas que **reporta Jellyfin**
  (guardadas en `media_items`), nunca en rutas que elija el cliente: la descarga
  valida el destino contra el inventario antes de escribir. Es una confianza
  explícita en Jellyfin; un servidor comprometido podría inducir escrituras
  donde reporte. Los `.nfo` **no** se escriben: son de Jellyfin. El hardening de systemd
  (`ProtectSystem=strict` + `ReadWritePaths`) acota el daño a las carpetas de medios.
- **Errores hacia el usuario**: mensajes genéricos en la UI; el detalle técnico va al
  log del servidor. Nunca exponer rutas internas, stack traces ni errores crudos de
  la DB o de APIs externas al navegador.
- **Secretos** (token de Jellyfin, API key de OpenSubtitles, credenciales de
  qBittorrent): en SQLite con permisos owner-only sobre el archivo; nunca en logs ni
  en el binario. Client IDs/tokens que haya que generar usan `crypto/rand`.
- **Bind**: `--addr` por defecto escucha en todas las interfaces (`:8090`) porque el
  dashboard se accede desde otras máquinas de la LAN. Es una decisión consciente
  (LAN de confianza); se puede acotar a una IP específica vía el flag.
- CSP estricta en headers, HTMX y CSS servidos desde el mismo origen (embebidos).

## 9. Errores y logging

Convenciones (stdlib, sin dependencias extra):

- Errores **siempre chequeados**, nunca descartados con `_`.
- Wrapping con contexto: `fmt.Errorf("scan %s: %w", path, err)`; inspección con
  `errors.Is` (centinelas) y `errors.As`/`errors.AsType` (tipos).
- **Single handling rule**: un error se loguea **o** se propaga, nunca ambas cosas
  (evita logs duplicados).
- Logging estructurado con **`log/slog`**. Mensajes de log de baja cardinalidad
  (plantilla estable); los datos variables (rutas, IDs, conteos) van como atributos.
- Middleware que loguea cada request HTTP (método, path, status, duración).

> Se mantiene solo stdlib. Librerías como `samber/oops` o `tint` quedan como opción
> futura si algún día se quiere stack traces o salida coloreada, pero no se suman
> ahora para respetar el principio de dependencias mínimas.

## 10. Calidad y verificación

El objetivo es que un binario que llega a la Pi ya pasó por estos filtros:

- `go test -race -shuffle=on ./...` — tests con detección de data races y orden
  aleatorio.
- `go vet ./...` y **golangci-lint** (config en `.golangci.yml`).
- `go mod tidy` + `git diff --exit-code` — falla si `go.mod`/`go.sum` quedaron sucios.
- `templ generate` + `git diff --exit-code` — falla si las vistas generadas quedaron
  desactualizadas respecto de los `.templ`.
- `govulncheck ./...` — vulnerabilidades conocidas en el código realmente alcanzado.

Localmente se agrupan en el `Makefile` (`make check`). En el repo corren además en
**GitHub Actions** ([`.github/workflows/ci.yml`](../.github/workflows/ci.yml)) en
cada push y PR a `main`.

> `golangci-lint` se instala con `go install` (no con el binario precompilado de la
> action): las releases de golangci-lint se compilan con un Go más viejo que el que
> targetea el módulo y se niegan a correr.

## Compilación y despliegue

**Se compila en la MacBook. La Raspberry Pi solo recibe el binario.**

1. Generar las vistas (solo en la Mac, requiere el CLI de templ):
   ```
   templ generate
   ```
2. Cross-compilar para la Pi. Target confirmado: **Raspberry Pi 4/5 con SO de 64
   bits → `arm64`**:
   ```
   CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o dist/holocron ./cmd/holocron
   ```
3. Copiar el binario a la Pi:
   ```
   scp dist/holocron pi@raspberry:/usr/local/bin/holocron
   ```
4. En la Pi corre como servicio de **systemd**.

Todo esto se automatiza en el `Makefile` (`make build-pi`, `make deploy`).

### Vía recomendada: releases + instalador

En lugar de copiar el binario a mano, el flujo normal es:

1. **Publicar**: `git tag vX.Y.Z && git push origin vX.Y.Z`. El workflow
   [`release.yml`](../.github/workflows/release.yml) cross-compila el binario
   `arm64`, calcula su SHA-256 y crea la release de GitHub con ambos adjuntos.
   (`make release VERSION=vX.Y.Z` hace lo mismo desde la Mac con el CLI `gh`.)
2. **Instalar o actualizar en la Pi**, por terminal:
   ```
   curl -fsSL https://raw.githubusercontent.com/criscardozo/holocron/main/scripts/install.sh | sudo bash
   ```
   `scripts/install.sh` baja el binario de la última release, **verifica el
   checksum**, crea el usuario de servicio, escribe la unit de systemd y arranca
   el servicio. Es idempotente: el mismo comando actualiza. Acepta
   `HOLOCRON_VERSION`, `HOLOCRON_ADDR` y `HOLOCRON_MEDIA_PATHS`, y tiene
   `--uninstall` (que conserva la base de datos).

`packaging/holocron.service` queda como unit de referencia para una instalación
manual; si se cambia el hardening, hay que tocar los dos lugares.

> **Importante**: la unit usa `ProtectSystem=strict`, así que **descargar
> subtítulos requiere listar las carpetas de medios en `ReadWritePaths`**
> (el instalador lo hace con `HOLOCRON_MEDIA_PATHS`). Sin eso, esos trabajos fallan
> y ahora lo informan explícitamente en la UI en vez de quedarse en silencio.

> **Target confirmado:** Raspberry Pi 4/5 con SO de 64 bits → `GOARCH=arm64`.

## Convención de commits

El historial está en [Conventional Commits](https://www.conventionalcommits.org):
`tipo(scope): asunto`, en **inglés (australiano)**, imperativo y minúscula, sin
punto final. Se reescribió el historial entero para que sea así, así que un
mensaje que se salga del patrón queda como el raro.

- **Tipos**: `feat`, `fix`, `perf`, `refactor`, `docs`, `test`, `build`, `ci`,
  `chore`.
- **Scopes en uso**: `disk`, `naming`, `media`, `jellyfin`, `subtitles`,
  `torrents`, `updates`, `api`, `ui`, `install`. Se omite si el cambio es
  transversal (`fix: harden filesystem confinement…`).
- **Incompatibles**: `!` antes de los dos puntos (`feat(media)!: …`) más un
  footer `BREAKING CHANGE:` que diga qué hay que hacer al actualizar. El único
  hasta ahora es la migración a Jellyfin (renombra columnas y borra la config de
  Plex).
- El cuerpo explica **por qué** y qué se verificó, no qué líneas cambiaron: eso
  ya lo dice el diff.

No hay linter de commits en CI: sumar uno implicaría Node en el pipeline, y el
único que escribe acá es una persona.
