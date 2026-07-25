# Fixtures

Respuestas **capturadas de un servidor Holocron real** (no escritas a mano), que
usan los tests de contrato para verificar que los modelos Swift siguen
coincidiendo con lo que devuelve la API en Go.

Para regenerarlas, con el server corriendo y un token generado:

```sh
TOKEN=...            # Ajustes → App iOS → Generar token
B=localhost:8080
A="Authorization: Bearer $TOKEN"

for ep in system disk naming media subtitles torrents; do
  curl -s -H "$A" "$B/api/v1/$ep" -o "$ep.json"
done
curl -s -H "$A" "$B/api/v1/disk/1"        -o disk_detail.json
curl -s -H "$A" "$B/api/v1/disk/1/browse" -o disk_browse.json
```

**Excepción:** `torrents_populated.json` está escrito a mano a partir de la
struct `apiTorrent` de `internal/httpserver/api.go`, porque capturar torrents
reales requiere un qBittorrent andando. Si cambia esa struct, hay que
actualizarlo a mano.
