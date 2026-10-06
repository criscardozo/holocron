# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

La web es el producto. La app de iOS (`ios/`, SwiftUI nativa) la acompaña con la
misma identidad y las mismas vistas, por la API JSON; el trabajo de diseño parte de
la web y después se lleva a la app.

## Users

Un solo usuario: Cristian, que administra Ginebra, su servidor de medios en casa.
Nadie más entra a Holocron. Lo abre desde la compu o, muy seguido, desde el
teléfono, en casa o afuera por Tailscale.

Las personas de la casa aparecen en Holocron sólo como datos (quién pide en Seerr,
quién está mirando en Jellyfin), no como usuarios.

## Product Purpose

Holocron es el panel de Ginebra: un solo lugar para saber cómo está el servidor y
para ocuparse de él, sin entrar a cada aplicación por separado. Se usa para cuatro
cosas, todas frecuentes:

- **Ver si todo anda**: un vistazo rápido, muchas veces desde el teléfono.
- **Ver qué hay y qué llega**: qué se reproduce, qué se baja, qué se agregó, cómo
  van los pedidos.
- **Arreglar cosas**: nombres de carpetas, calidad de la biblioteca, torrents
  trabados, reiniciar servicios, apagar o reiniciar el equipo.
- **Atender un problema**: se cortó la luz, se cayó un servicio, un disco se llena.

Funciona bien cuando, de un vistazo, queda claro si hace falta hacer algo, y
cuando hace falta, se llega en un toque a donde se hace.

## Positioning

Holocron es la herramienta del propio servidor, no un producto aparte: lee de
primera mano la máquina (`/proc`, `/sys`, systemd, SMART) y las aplicaciones que
corren en ella (Jellyfin, qBittorrent, Radarr, Sonarr, Prowlarr, Seerr, Bazarr), y
junta todo con las palabras de la casa. Las claves de esas aplicaciones las entrega
el servidor y nunca llegan al navegador ni al teléfono.

## Operating Context

- **El servidor**: Ginebra, una notebook Acer x86 (reemplazó a la Raspberry Pi
  «ObiWan»). Su batería hace de UPS. No se puede encender a distancia (la placa de
  red no despierta desde apagado) ni vuelve sola cuando vuelve la luz: si la
  batería se agota, hay que prenderla a mano.
- **Acceso**: Holocron escucha en loopback detrás de Caddy, con HTTPS, en
  `holocron.merli.store`; desde casa por la LAN y desde afuera por Tailscale. Nada
  queda expuesto a internet. La web no pide login dentro de esa red; la API de la
  app sí pide token.
- **Administración del servidor**: la hace otra sesión («Ginebra»), que instala
  las versiones, gestiona las credenciales (systemd `LoadCredential`) y mide en la
  máquina. Holocron se publica como release de GitHub y Ginebra la instala.
- **Rituales**: pruebas reales en la máquina (corte de luz desenchufando el
  cargador, mediciones de CPU por pantalla) antes de dar algo por bueno.

## Capabilities and Constraints

- **Pantallas**: Inicio, Actividad, Hardware y Servicios (en vivo por SSE); Medios,
  Calidad, Nombres y Disco (biblioteca); Torrents; Gestión (reiniciar servicios,
  reiniciar o apagar el equipo); Ajustes.
- **Stack**: Go con `net/http`, templ y HTMX (con su extensión SSE), SQLite
  (`modernc.org/sqlite`), sin CGO, todo embebido en un solo binario. JavaScript
  propio permitido sólo en archivos `.js` embebidos, nunca inline.
- **CSP estricta**: `default-src 'self'`, sin `style=""` ni scripts inline. Las
  barras y gráficos van como SVG o clases precomputadas.
- **Liviano**: lo pesado va bajo demanda; lo en vivo sólo muestrea mientras alguien
  mira. Holocron comparte la máquina con Jellyfin, que es quien manda en la CPU.
- **Pósters**: los sirve Holocron por `/art/…` desde su caché en disco; el
  navegador nunca habla con Jellyfin ni con TMDb.
- **Costo cero**: sin servicios pagos; el CI de GitHub en Linux.
- **Idioma**: la interfaz, en español de Argentina.

## Brand Commitments

- **Nombre**: Holocron, la herramienta de Ginebra. La etiqueta «GINEBRA» acompaña
  al nombre y dice de qué servidor es.
- **Marca**: el holocrón, un cubo isométrico que dibujó Ginebra con los colores del
  G▶ (`web/static/holocron.svg`; ícono de iOS en
  `ios/Holocron/Assets.xcassets/AppIcon.appiconset/`).
- **Familia visual**: Holocron se ve como parte de la portada de Ginebra
  (`ginebra.merli.store`), por decisión explícita de Cristian («me gusta el
  estilo de la web de Ginebra»). El sistema visual concreto está en el código
  (`web/static/styles.css`, `docs/ui.md`), no acá.
- **Voz**: directa y concreta, con las palabras de la casa («el equipo», el nombre
  de la máquina, «sin luz», «no responde»). Dice qué pasa y qué hacer, no
  tecnicismos.

## Evidence on Hand

- Datos reales del servidor: la biblioteca de Jellyfin (366 ítems), pedidos de
  Seerr, servicios de systemd, SMART y batería, todo leído en vivo.
- Mediciones hechas en Ginebra: CPU por pantalla, costo de los pósters (13 MB para
  366, 36 KB cada uno), prueba real de corte de luz.
- Documentación: `docs/features.md`, `docs/ui.md`, `docs/api.md`,
  `docs/arquitectura.md`.
- No hay testimonios, clientes ni métricas de uso: es una herramienta personal y
  no se inventan.

## Product Principles

1. **Lo que está bien queda tranquilo; lo que falta, salta.** El estado normal es
   silencioso, y lo que requiere atención se ve de lejos y lleva directo a donde
   se arregla.
2. **Un vistazo antes que un recorrido.** Cada área se resume en un número que dice
   si vale la pena entrar; el detalle está a un toque, no en la cara.
3. **Mirar no le cuesta nada a la máquina.** Holocron no le saca recursos a lo que
   el servidor está para hacer; lo que no se mira, no se mide.
4. **Las claves se quedan en el servidor.** Ni el navegador ni el teléfono ven una
   credencial, y nada se expone a internet.
5. **Las acciones irreversibles se dicen.** Apagar o borrar explica qué se pierde y
   pide un gesto explícito; no se esconden «por las dudas».
