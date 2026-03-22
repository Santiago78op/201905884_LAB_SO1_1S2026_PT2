# Comandos para probar `module_kernel` en QEMU

Esta guía deja el flujo ordenado para que puedas levantar el kernel y el módulo rápidamente, incluso si cerraste la terminal.

## 1) Comando base para arrancar QEMU

```bash
qemu-system-x86_64 \
  -kernel arch/x86/boot/bzImage \
  -initrd ~/initramfs.img \
  -nographic \
  -append "console=ttyS0"
```

Comandos útiles dentro de la sesión:

```bash
rmmod module_kernel
dmesg | tail -n 30
```

## 2) Si cerraste la terminal, que no cunda el pánico

No perdiste lo importante. Mientras no hayas borrado archivos, todo sigue en disco:

- Kernel compilado: `~/linux/arch/x86/boot/bzImage`
- Módulo: `~/linux/samples/rust/module_kernel.ko`

## 3) Caso real de este proyecto

Tu sistema host y tu módulo no comparten versión de kernel.

```bash
uname -r
# 6.14.0-27-generic (host)
```

Tu módulo fue compilado para otra versión (por ejemplo, `7.0.0-rc3`), así que no lo vas a cargar directo en Lubuntu.

Por eso, para probarlo de forma consistente, debes levantarlo en QEMU.

## 4) Flujo rápido para levantar todo otra vez

### Paso 1: entrar al árbol del kernel

```bash
cd ~/linux
```

### Paso 2: verificar que existen los artefactos

```bash
ls arch/x86/boot/bzImage
ls samples/rust/module_kernel.ko
```

### Paso 3: arrancar QEMU (si ya tienes `initramfs.img`)

```bash
qemu-system-x86_64 \
  -kernel arch/x86/boot/bzImage \
  -initrd ~/initramfs.img \
  -nographic \
  -append "console=ttyS0"
```

Si todo está bien, deberías ver algo como:

```text
Cargando module_kernel.ko...
module_kernel: hola mundo desde Rust
```

## 5) Si borraste el initramfs, recréalo

```bash
mkdir -p ~/initramfs/{bin,proc,sys}
cp /usr/bin/busybox ~/initramfs/bin/
cp ~/linux/samples/rust/module_kernel.ko ~/initramfs/

cd ~/initramfs/bin
for x in sh mount echo dmesg insmod rmmod uname cat ls; do ln -sf busybox "$x"; done

cat > ~/initramfs/init <<'EOF'
#!/bin/sh
mount -t proc none /proc
mount -t sysfs none /sys

echo "Kernel:"
uname -r

echo "Cargando modulo..."
insmod /module_kernel.ko

dmesg | tail -n 20

exec sh
EOF

chmod +x ~/initramfs/init
cd ~/initramfs
find . | cpio -o -H newc | gzip -9 > ~/initramfs.img
```

## 6) Flujo corto para presentación

Si quieres mostrar todo en pocos pasos:

```bash
cd ~/linux
modinfo samples/rust/module_kernel.ko

qemu-system-x86_64 \
  -kernel arch/x86/boot/bzImage \
  -initrd ~/initramfs.img \
  -nographic \
  -append "console=ttyS0"
```

Con eso demuestras:

1. Que el módulo existe.
2. Que fue compilado.
3. Que carga y corre en el kernel levantado en QEMU.

## 7) Resumen

Si cierras la terminal:

- No pierdes el módulo.
- No necesitas recompilar todo.
- Solo necesitas volver a ejecutar QEMU (y recrear `initramfs` si lo borraste).