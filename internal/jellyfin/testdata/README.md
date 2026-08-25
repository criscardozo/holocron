# Fixtures de Jellyfin

`jellyfin-fixtures.json` son respuestas **capturadas del servidor Jellyfin real**
de ObiWan, no escritas a mano. Cubren los seis casos que nos costaron descubrir:

| caso | por qué está |
|---|---|
| `pelicula_con_subtitulo_externo` | `.srt` suelto con su `Path` y `ProviderIds` con clave con espacio |
| `pelicula_con_subtitulos_internos` | subtítulos embebidos |
| `pelicula_multi_version` | dos archivos, y los subtítulos en español están **sólo en uno** |
| `serie` | apunta a un directorio, sin `MediaSources` |
| `episodio_con_archivo` | caso normal |
| `episodio_fantasma_sin_archivo` | existe en el proveedor de metadata, no en disco |

Para regenerarlas, con acceso al Pi:

```sh
ssh cristian@ObiWan.local 'python3 ~/obiwan-tuning/dump-api.py'
scp cristian@ObiWan.local:~/obiwan-tuning/jellyfin-fixtures.json .
```

En `~/obiwan-tuning/` viven también los scripts que miden la biblioteca entera
(`metricas.py` es la especificación ejecutable de cada criterio) y la
verificación de que `/Items` sin `userId` devuelve las 344 películas
(`sin-userid.py`). Si un criterio de Holocron y uno de esos scripts dan números
distintos para lo mismo, uno de los dos está mal.
