# Estructura de un Daemon en Go

```bash
mydaemon/
  cmd/
    mydaemon/
      main.go
  internal/
    app/
      service.go        # loop principal, señales, schedule
    sources/
      file_reader.go    # lee archivos (polling) + opcional fsnotify
      proc_reader.go    # helpers para /proc
    parser/
      continfo.go       # parsea continfo -> struct
      meminfo.go        # parsea meminfo -> struct
    sink/
      stdout.go         # salida simple
      jsonfile.go       # escribe JSON a archivo
      valkey.go         # (luego) manda a Valkey/Redis
    model/
      metrics.go        # structs de dominio (RAM, container stats)
  pkg/ (opcional)
  go.mod
```

***Regla:*** app/service.go no debería “saber” cómo se lee o parsea; solo orquesta.
