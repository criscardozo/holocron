# Holocron para iOS

App nativa en SwiftUI para manejar el HTPC desde el teléfono. Habla con el
servidor por la [API JSON](../docs/api.md); no muestra la web adentro de un
WebView.

Cubre las cuatro áreas del panel: **estado** de la Pi, **torrents** (agregar
magnets, pausar, borrar), **subtítulos** (buscar y descargar) y **medios**
(inventario de Jellyfin) con el explorador de **disco**.

## Requisitos

- Xcode 16 o posterior (se desarrolló con Xcode 26).
- [xcodegen](https://github.com/yonaskolb/XcodeGen) (`brew install xcodegen`).
- Deployment target: **iOS 17**.

## Abrir el proyecto

El `.xcodeproj` **no está versionado**: lo genera xcodegen a partir de
`project.yml`. Un pbxproj es un archivo escrito por máquina, mergea mal y se
desincroniza; la definición declarativa no.

```sh
cd ios && xcodegen generate && open Holocron.xcodeproj
```

O desde la raíz del repo:

```sh
make ios-project   # genera el proyecto
make ios-test      # genera, compila y corre los tests en el simulador
```

Al agregar o borrar archivos Swift **no hace falta tocar nada**: se toman de la
carpeta, así que alcanza con volver a generar.

## Configurar la app

1. En la web de Holocron: **Ajustes → App iOS → Generar token**. Se muestra una
   sola vez (el servidor guarda sólo su digest).
2. En la app, pestaña **Ajustes**: cargá la dirección del servidor
   (`192.168.1.10:8090` alcanza, el `http://` se asume) y pegá el token.
3. **Probar conexión** confirma que llega y devuelve el nombre del host.

El token queda en el **Keychain**, no en `UserDefaults`.

## Estructura

```
ios/
├── project.yml              definición del proyecto (fuente de verdad)
├── Holocron/
│   ├── HolocronApp.swift    entrypoint
│   ├── APIClient.swift      cliente HTTP + mapeo de errores
│   ├── Models.swift         structs Codable espejo de la API
│   ├── AppSettings.swift    dirección del server + token (@Observable)
│   ├── Keychain.swift       guardado del token
│   ├── Theme.swift          paleta Noir, tarjetas, barras, pills
│   ├── Formatters.swift     bytes, velocidades, uptime, fechas
│   └── Views/               una pantalla por pestaña + drill-down de disco
└── HolocronTests/
    ├── ContractTests.swift  decodifica respuestas reales de la API
    ├── SupportTests.swift   parseo de URL, formateo, errores
    └── Fixtures/            JSON capturado del servidor
```

## Decisiones que conviene conocer

- **Tema oscuro fijo.** La app es dark-only, igual que la web: el tema Noir es
  una identidad, no una preferencia. Los colores de `Theme.swift` están
  sincronizados con `web/static/styles.css`.
- **HTTP en la LAN.** La Pi sirve HTTP plano, así que el `Info.plist` habilita
  `NSAllowsLocalNetworking`. Eso **no** permite cargas inseguras hacia internet.
- **Sin dependencias externas.** Sólo SwiftUI, Foundation y Security, igual que
  el servidor se mantiene en dos dependencias de Go.
- **Concurrencia estricta de Swift 6** activada.
- **Torrents se auto-refresca cada 3 s** mientras la pestaña está visible,
  igual que la tabla de la web; el resto de las pantallas usan pull-to-refresh.

## Tests

```sh
make ios-test
```

Los tests de contrato decodifican JSON **capturado de un servidor real**, de
modo que si la API en Go cambia un campo, lo agarrás acá en vez de en el
teléfono. **No corren en CI**: la app se compila y testea a mano, así que
acordate de `make ios-test` después de tocar la API. Para regenerar los fixtures, ver
[`HolocronTests/Fixtures/README.md`](HolocronTests/Fixtures/README.md).

## El ícono

`Holocron/Assets.xcassets/AppIcon.appiconset/icon-1024.png`, un solo tamaño de
1024 px del que Xcode deriva el resto. Es la misma marca que la web
(`web/static/favicon.svg`): diamante naranja `#ff6a2b` sobre `#121110`.

Dos cosas a respetar si se rehace:

- **Sin canal alfa y sin esquinas redondeadas.** iOS rechaza el alfa y aplica su
  propia máscara; venir con esquinas propias se ve mal.
- **Las puntas del diamante van en los puntos medios de los lados**, que es la
  parte del lienzo que la máscara nunca recorta. Un logo cuadrado tendría que
  quedar bastante más chico para sobrevivir al recorte.

## Instalarla en el teléfono

No hay distribución por App Store: es una app personal. Con el iPhone conectado,
elegilo como destino en Xcode y dale Run. Con una cuenta gratuita de Apple
Developer el perfil caduca cada 7 días; con una cuenta paga, cada año.

## Fuera de la LAN

La app necesita alcanzar la Pi. Adentro de casa, la IP local alcanza. Para usarla
afuera, lo razonable es una VPN tipo [Tailscale](https://tailscale.com) o
WireGuard en la Pi, y poner la IP de la VPN como dirección del servidor: el
servidor nunca queda expuesto a internet.

## Detrás de Cloudflare Access

Si el servidor se entra por el dominio público en vez de por la LAN, Access está
adelante y la app necesita además un **service token**: los dos campos de
Ajustes → *Cloudflare Access*. El id va a `UserDefaults` (no es secreto) y el
secreto al Keychain, que una vez guardado se muestra sólo por sus últimos
caracteres.

El rechazo de Access se reconoce y se reporta como tal. Hace falta porque no se
parece a un error de la API: `URLSession` sigue el redirect y devuelve la página
de login con status `200`, así que sin esto el mensaje sería «el servidor
respondió algo inesperado». Ver la sección de Access en
[`../docs/api.md`](../docs/api.md) para las tres formas que toma el rechazo.
