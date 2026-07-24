# Holocron

Panel de administración y dashboard web para un HTPC basado en Raspberry Pi.

Holocron es una aplicación web liviana, escrita en Go, para administrar de forma
remota una Raspberry Pi que actúa como HTPC: corre **Plex Media Server** como servidor
de medios y **qBittorrent** para descargas por torrent. Se accede desde el navegador
dentro de la red local, no expone nada a internet, y reúne en un solo lugar el estado
del equipo y las tareas de mantenimiento habituales de la biblioteca.

Es **un solo binario**, sin dependencias en el destino: la interfaz (plantillas, CSS y
HTMX) va embebida en el ejecutable. Corre como servicio de systemd en la Pi.

## Qué hace

- **Dashboard** — grilla de widgets con el estado del equipo (CPU, RAM, temperatura,
  uptime, load) y un resumen de cada herramienta.
- **Uso de disco** — espacio libre/ocupado por carpeta, con un explorador navegable
  que muestra el peso real de cada subcarpeta y archivo (tipo `du`, con drill-down).
- **Validador de nombres** — detecta las carpetas de Películas/Series que no cumplen
  la convención «Título (Año)», con lo esperado vs. lo encontrado.
- **Medios y archivos `.nfo`** — inventaría la biblioteca desde Plex y genera un `.nfo`
  por película/serie con la metadata que Plex ya resolvió (título, año, identificadores)
  e indica si hay subtítulos en español.
- **Subtítulos** — lista los medios sin subtítulo en español y permite buscarlos y
  descargarlos desde OpenSubtitles.
- **Torrents** — administra qBittorrent (estado, progreso, velocidades, pausar/reanudar/
  borrar) y agrega descargas pegando un magnet-link.

Los escaneos y trabajos pesados son **bajo demanda**: el usuario los dispara y los
resultados se cachean; nada corre en loop consumiendo la Pi de fondo.

## Instalación

En la Raspberry Pi (headless, por terminal). Descarga el último binario `arm64`
publicado, crea el usuario de servicio, instala la unit de systemd y arranca todo:

```sh
curl -fsSL https://raw.githubusercontent.com/criscardozo/holocron/main/scripts/install.sh | sudo bash
```

Cuando termina, imprime la URL de acceso (por defecto `http://<ip-de-la-pi>:8080`).
Abrila desde otra máquina de la LAN y configurá las carpetas de medios y las
credenciales de Plex/OpenSubtitles/qBittorrent desde **Ajustes**.

Opciones (variables de entorno antes del `sudo bash`, o `sudo bash -s -- <flags>`):

```sh
# Instalar una versión puntual en lugar de la última
curl -fsSL …/install.sh | sudo HOLOCRON_VERSION=v0.1.0 bash

# Cambiar el puerto de escucha (default :8080)
curl -fsSL …/install.sh | sudo HOLOCRON_ADDR=:9000 bash

# Dar acceso de lectura/escritura a las carpetas de medios (hardening de systemd).
# Sin esto, el servicio no puede escribir .nfo ni subtítulos fuera de su estado.
curl -fsSL …/install.sh | sudo HOLOCRON_MEDIA_PATHS="/mnt/media /srv/downloads" bash
```

> El instalador baja el binario desde las
> [releases de GitHub](https://github.com/criscardozo/holocron/releases). Requiere que
> haya al menos una release publicada (ver [Publicar una release](#publicar-una-release)).

## Actualización

Volvé a correr el mismo comando: el instalador es idempotente, detecta la instalación
existente, reemplaza el binario y reinicia el servicio.

```sh
curl -fsSL https://raw.githubusercontent.com/criscardozo/holocron/main/scripts/install.sh | sudo bash
```

Para desinstalar (borra el binario, la unit y el usuario; **conserva** la base de datos
en `/var/lib/holocron`):

```sh
curl -fsSL …/install.sh | sudo bash -s -- --uninstall
```

## Configuración

Dos niveles:

- **Arranque** (flags o variables de entorno del servicio): `--addr` (default `:8080`),
  `--db` (default `/var/lib/holocron/holocron.db` bajo systemd), `--log-level`. Los
  flags pisan a las variables `HOLOCRON_ADDR` / `HOLOCRON_DB` / `HOLOCRON_LOG_LEVEL`.
- **Aplicación** (desde la UI, persistido en SQLite): carpetas de medios, URL y token de
  Plex, API key y usuario de OpenSubtitles, URL y credenciales de qBittorrent. Las
  credenciales viven en la base con permisos owner-only; nunca en el binario.

## Compilación (desde la MacBook)

Holocron **se compila en la máquina de desarrollo**; a la Pi solo va el binario. Todo
el toolchain (Go, `templ`) vive en la Mac.

```sh
make build       # binario para la Mac (chequeo rápido)
make build-pi    # cross-compila el binario arm64 para la Pi → dist/holocron-arm64
make check       # vet + lint + tests con -race + govulncheck
make run         # corre local en :8080
```

### Publicar una release

El instalador de la Pi baja el binario desde las releases de GitHub. Para publicar
una, **empujá un tag** con formato `vX.Y.Z`: el workflow de GitHub Actions
([`.github/workflows/release.yml`](.github/workflows/release.yml)) cross-compila el
binario `arm64`, genera su checksum SHA-256 y crea la release con ambos adjuntos.

```sh
git tag v0.1.0
git push origin v0.1.0
```

Como alternativa, para publicar desde la Mac sin depender de Actions (requiere el
[CLI `gh`](https://cli.github.com/) autenticado):

```sh
make release VERSION=v0.1.0
```

> Cada push y PR a `main` pasa además por el workflow de CI
> ([`.github/workflows/ci.yml`](.github/workflows/ci.yml)): `vet`, tests con `-race`,
> `golangci-lint`, `govulncheck` y chequeos de `go mod tidy` y de plantillas al día.

### Deploy directo (sin release)

Si preferís empujar el binario a mano sin publicar una release:

```sh
make deploy PI=usuario@raspberrypi.local
```

## Documentación

- [Arquitectura y decisiones técnicas](docs/arquitectura.md) — stack, estructura,
  modelo de datos, seguridad, compilación y despliegue.
- [Especificación de features](docs/features.md) — detalle de cada pantalla.
- [Roadmap por fases](docs/roadmap.md) — orden de construcción.

## Licencia

[MIT](LICENSE).
