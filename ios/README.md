# Holocron para iOS

App nativa en SwiftUI para manejar el HTPC desde el teléfono. Habla con el
servidor por la [API JSON](../docs/api.md); no muestra la web adentro de un
WebView.

Cubre el **estado** del servidor con sus pantallas de **hardware** (CPU por
núcleo, memoria, red, discos y la batería que hace de UPS) y **servicios**
(unidades de systemd, tareas programadas y SMART), la **actividad** (quién está
mirando, qué se baja, los pedidos de Seerr y lo que viene), los **torrents**
(agregar magnets, pausar, borrar), los **medios** (inventario de Jellyfin) y el
explorador de **disco**.

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
   (`https://holocron.merli.store`, la de Caddy) y pegá el token.
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
│   ├── LiveModels.swift     las pantallas en vivo (hardware, actividad, servicios)
│   ├── AppSettings.swift    dirección del server + token (@Observable)
│   ├── Keychain.swift       guardado del token
│   ├── Theme.swift          paleta de Ginebra, tarjetas, barras, pills
│   ├── Formatters.swift     bytes, velocidades, uptime, fechas
│   └── Views/               una pantalla por pestaña + drill-down de disco
└── HolocronTests/
    ├── ContractTests.swift  decodifica respuestas reales de la API
    ├── SupportTests.swift   parseo de URL, formateo, errores
    └── Fixtures/            JSON capturado del servidor
```

## Decisiones que conviene conocer

- **Tema oscuro fijo.** La app es dark-only, igual que la web: viste la paleta
  del portal de Ginebra, que es una identidad, no una preferencia. Los colores de `Theme.swift` están
  sincronizados con `web/static/styles.css`.
- **Sólo HTTPS.** Holocron escucha en `127.0.0.1` y se entra por Caddy, así que
  el `Info.plist` no tiene excepciones de ATS.
- **Sin dependencias externas.** Sólo SwiftUI, Foundation y Security, igual que
  el servidor se mantiene en dos dependencias de Go.
- **Concurrencia estricta de Swift 6** activada.
- **Las pantallas en vivo se refrescan solas mientras están a la vista**:
  Hardware cada 3 s, Torrents cada 3 s, Actividad cada 5 s, Servicios cada
  15 s. La web usa SSE; acá alcanza con un poll corto, porque el servidor
  muestrea sólo mientras alguien pregunta y la app deja de preguntar en cuanto
  la pantalla se va. El resto usa pull-to-refresh.

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
1024 px del que Xcode deriva el resto. Es la marca de Ginebra, la misma que usa
la web (`web/static/ginebra.svg`): la G y el ▶ en facetas rosa y violeta sobre
`#141a26`. Se genera así:

```sh
sed 's/rx="30" //' web/static/ginebra.svg > /tmp/g.svg
rsvg-convert -w 1024 -h 1024 /tmp/g.svg -o /tmp/g.png
magick /tmp/g.png -background '#141a26' -alpha remove -alpha off \
  PNG24:ios/Holocron/Assets.xcassets/AppIcon.appiconset/icon-1024.png
```

Dos cosas a respetar si se rehace:

- **Sin canal alfa y sin esquinas redondeadas.** iOS rechaza el alfa y aplica su
  propia máscara; venir con esquinas propias se ve mal.
- **El dibujo queda adentro del centro del lienzo.** La G▶ ya viene con margen
  de sobra para la máscara de iOS; no agrandarlo.

## Instalarla en el teléfono

No hay distribución por App Store: es una app personal. Con el iPhone conectado
y desbloqueado, desde la raíz del repo:

```sh
make ios-install IOS_TEAM=XXXXXXXXXX IOS_DEVICE=00008150-XXXXXXXXXXXX
```

- `IOS_TEAM`: el `OU` del certificado de desarrollo —
  `security find-identity -v -p codesigning` y después
  `security find-certificate -c "Apple Development: …" -p | openssl x509 -noout -subject`.
- `IOS_DEVICE`: el UDID que lista `xcrun xctrace list devices`.

Ninguno de los dos está en el repo a propósito: es público, y los dos
identifican la cuenta y el teléfono. Xcode crea el perfil solo con
`-allowProvisioningUpdates`.

También sirve abrir el proyecto en Xcode, elegir el teléfono y darle Run. Con
una cuenta gratuita de Apple Developer el perfil caduca **cada 7 días** y hay
que reinstalar; con una cuenta paga, cada año.

## Fuera de casa

La dirección es la misma adentro y afuera: `holocron.merli.store` resuelve a
Caddy, y desde afuera se llega por [Tailscale](https://tailscale.com). El
servidor nunca queda expuesto a internet, así que no hace falta nada más en la
app: ni service tokens ni excepciones.
