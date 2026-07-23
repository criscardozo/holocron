# Holocron

Panel de administración y dashboard para un HTPC basado en Raspberry Pi.

Holocron es una aplicación web liviana, escrita en Go, pensada para administrar de
forma remota una Raspberry Pi que actúa como HTPC: corre **Plex Media Server** como
servidor de medios y **qBittorrent (qbittorrent-nox)** para descargas por torrent.

La app se administra desde el navegador dentro de la red local. No expone nada a
internet. Reúne en un solo lugar un conjunto de herramientas de mantenimiento del
HTPC y un dashboard con el estado del equipo.

## Objetivos de diseño

- **Liviana**: pensada para correr en una Raspberry Pi sin castigar CPU ni RAM.
- **Un solo binario**: sin dependencias externas en el destino. Todo (plantillas,
  CSS, JS) va embebido en el ejecutable.
- **Se compila en la MacBook, no en la Pi**: la Pi solo recibe el binario ya
  compilado. Ver [docs/arquitectura.md](docs/arquitectura.md#compilación-y-despliegue).
- **Escaneos bajo demanda**: nada de tareas pesadas corriendo en loop. El usuario
  dispara los escaneos y los resultados se cachean.
- **Modular por features**: cada herramienta es un módulo independiente que se
  suma como panel/widget al dashboard.

## Stack

- **Go 1.23+**, sin CGO (cross-compilación directa a ARM).
- **[templ](https://templ.guide)** + **[HTMX](https://htmx.org)** para la interfaz
  (server-side rendering con updates parciales, sin build de JS).
- **SQLite** vía `modernc.org/sqlite` (driver Go puro) para config, cache y estado.
- Reutiliza lógica ya escrita en Go de dos proyectos hermanos:
  - `diskusage-pi` → escaneo de disco.
  - `plexmatch-generator` → cliente de Plex Media Server.

## Estado

En planificación. Ver el [roadmap por fases](docs/roadmap.md).

## Documentación

- [Arquitectura y decisiones técnicas](docs/arquitectura.md)
- [Roadmap por fases](docs/roadmap.md)
- [Especificación de features](docs/features.md)
