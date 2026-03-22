# Guía paso a paso: compilar, probar y presentar un módulo Linux en Rust en Lubuntu

## Objetivo

Esta guía documenta todo el proceso para:

1. Preparar el entorno en una VM con **Lubuntu**.
2. Compilar un **kernel Linux con soporte Rust**.
3. Agregar un módulo propio en Rust, por ejemplo `module_kernel`.
4. Compilar el módulo `.ko`.
5. Probarlo correctamente.
6. Entender dos caminos de ejecución:
   - **Camino 1:** arrancar con el kernel compilado.
   - **Camino 2:** compilar para el kernel activo `6.14.0-27-generic`.
7. Saber cómo **volver a montarlo** o repetir la demostración al momento de presentar.

---

## Contexto del problema

Durante el proceso se confirmó lo siguiente:

- El módulo generado fue:
  - `samples/rust/module_kernel.ko`
- El `modinfo` del módulo mostró:
  - `vermagic: 7.0.0-rc3-g79e25710e722-dirty ...`
- El kernel actualmente en ejecución en la VM es:
  - `uname -r` → `6.14.0-27-generic`

### ¿Qué significa esto?

El módulo `module_kernel.ko` **sí fue compilado correctamente**, pero fue compilado para un kernel distinto al que está corriendo la VM.

Por eso:

- **sí existe el `.ko`**
- **sí está bien construido**
- pero **no debe cargarse con `insmod` en el kernel 6.14.0-27-generic**

porque el `vermagic` no coincide.

---

# Parte 1. Preparación del entorno

## 1. Sistema usado

- Host: **Pop!_OS**
- VM: **Lubuntu**
- Arquitectura: **x86_64**

La compilación del kernel y del módulo se hace dentro de la **VM Lubuntu**.

---

## 2. Instalar dependencias

```bash
sudo apt update
sudo apt install -y \
  git curl wget build-essential bc kmod cpio rsync file \
  flex bison libncurses-dev libssl-dev openssl \
  libelf-dev libudev-dev libpci-dev libiberty-dev autoconf \
  qemu-system-x86 qemu-utils qemu-kvm \
  gdb llvm lld libclang-dev pkg-config \
  busybox-static cpio
```

---

## 3. Instalar Rust con rustup

```bash
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
source "$HOME/.cargo/env"
rustup update stable
rustup default stable
rustup component add rust-src
```

Verificar:

```bash
rustc --version
cargo --version
```

---

## 4. bindgen

Se detectó un problema con `bindgen 0.66.1`, por lo que debe evitarse usar esa versión del sistema si causa pánicos durante la generación de bindings.

Verificar:

```bash
bindgen --version
```

Si el sistema usa una versión problemática, conviene asegurarse de usar el binario correcto de Cargo en `~/.cargo/bin`.

---

# Parte 2. Clonar y preparar el kernel

## 5. Clonar el árbol del kernel con soporte Rust

```bash
git clone https://github.com/Rust-for-Linux/linux.git
cd linux
```

---

## 6. Verificar que Rust esté disponible para el kernel

```bash
make LLVM=1 rustavailable
```

Resultado esperado:

```text
Rust is available!
```

Esto indica que:

- el toolchain Rust está detectado
- LLVM/Clang está funcional
- el kernel puede habilitar soporte Rust

---

# Parte 3. Configuración del kernel

## 7. Crear una configuración base

```bash
make LLVM=1 defconfig
```

---

## 8. Habilitar Rust manualmente

En este proyecto fue necesario habilitar Rust así:

```bash
./scripts/config --enable RUST
make LLVM=1 olddefconfig
```

Verificar:

```bash
grep -n '^CONFIG_RUST=' .config
grep -n '^# CONFIG_RUST is not set' .config
```

Resultado esperado:

```text
CONFIG_RUST=y
```

---

## 9. Ajustes Kconfig que dieron conflicto

Durante el proceso se detectaron varias opciones que podían bloquear `CONFIG_RUST`.

Las más relevantes fueron:

- `MODVERSIONS`
- `CALL_PADDING`
- `MITIGATION_CALL_DEPTH_TRACKING`
- `CALL_THUNKS`

Se usaron estos ajustes:

```bash
./scripts/config --disable MODVERSIONS
./scripts/config --disable MITIGATION_CALL_DEPTH_TRACKING
./scripts/config --disable CALL_THUNKS
make LLVM=1 olddefconfig
```

Verificaciones útiles:

```bash
grep CONFIG_MODVERSIONS .config
grep CONFIG_RUST .config
```

---

## 10. Entrar a menuconfig

```bash
make LLVM=1 menuconfig
```

Opciones importantes:

- `General setup -> Rust support`
- `Kernel hacking -> Sample kernel code`
- `Kernel hacking -> Sample kernel code -> Rust samples`

---

# Parte 4. Agregar el módulo propio en Rust

## 11. Crear el archivo del módulo

Archivo:

```text
samples/rust/module_kernel.rs
```

Ejemplo base:

```rust
// SPDX-License-Identifier: GPL-2.0

//! Modulo Linux en Rust: hola mundo.

use kernel::prelude::*;

module! {
    type: ModuleKernel,
    name: b"module_kernel",
    author: b"Julian Reyes",
    description: b"Modulo Linux en Rust: hola mundo",
    license: b"GPL",
}

struct ModuleKernel;

impl kernel::Module for ModuleKernel {
    fn init(_module: &'static ThisModule) -> Result<Self> {
        pr_info!("module_kernel: hola mundo desde Rust\n");
        Ok(ModuleKernel)
    }
}

impl Drop for ModuleKernel {
    fn drop(&mut self) {
        pr_info!("module_kernel: adios desde Rust\n");
    }
}
```

---

## 12. Registrar el módulo en `samples/rust/Makefile`

Agregar una línea como esta:

```make
obj-$(CONFIG_SAMPLE_RUST_HELLO) += module_kernel.o
```

> Nota: el nombre del archivo `.rs` y el `.o` deben coincidir.

---

## 13. Registrar el módulo en `samples/rust/Kconfig`

Agregar un bloque como este:

```text
config SAMPLE_RUST_HELLO
	tristate "Hello"
	help
	  This option builds the Rust hello module sample.
	  To compile this as a module, choose M here:
	  the module will be called module_kernel.
```

---

## 14. Activarlo como módulo

Desde `menuconfig`, dentro de:

```text
Kernel hacking
  -> Sample kernel code
    -> Rust samples
```

activar en modo módulo:

```text
<M> Hello
```

Si se usó el sample oficial mínimo, también puede activarse:

```text
<M> Minimal
```

---

# Parte 5. Compilación

## 15. Compilar el kernel y los módulos

```bash
make LLVM=1 -j"$(nproc)"
```

Resultado esperado:

```text
Kernel: arch/x86/boot/bzImage is ready (#1)
```

---

## 16. Verificar que el módulo fue generado

```bash
find . -name '*.ko' | grep rust
```

Resultado obtenido en este caso:

```text
./samples/rust/module_kernel.ko
```

Esto confirma que el módulo propio fue compilado correctamente.

---

## 17. Verificar metadata del módulo

```bash
modinfo ./samples/rust/module_kernel.ko
```

Resultado relevante:

```text
name:           module_kernel
vermagic:       7.0.0-rc3-g79e25710e722-dirty SMP preempt mod_unload
```

---

# Parte 6. ¿Por qué no se puede cargar directamente en la VM?

## 18. Verificar kernel activo

```bash
uname -r
```

Resultado real:

```text
6.14.0-27-generic
```

---

## 19. Explicación técnica

El módulo fue compilado para:

```text
7.0.0-rc3-g79e25710e722-dirty
```

Pero la VM está corriendo:

```text
6.14.0-27-generic
```

Por tanto, si se intenta:

```bash
sudo insmod ./samples/rust/module_kernel.ko
```

lo normal es obtener un error de incompatibilidad de formato o `vermagic`.

---

# Parte 7. Camino 1: arrancar con el kernel que compilaste

Este es el **camino recomendado** para ver el mensaje `hola mundo` de tu módulo.

## ¿En qué consiste?

Consiste en arrancar el `bzImage` que compilaste y cargar dentro de ese mismo kernel el módulo `module_kernel.ko`.

Como ambos pertenecen al mismo árbol de compilación, el módulo sí es compatible con ese kernel.

---

## 20. Crear un initramfs mínimo

```bash
mkdir -p ~/initramfs/{bin,proc,sys}
cp /usr/bin/busybox ~/initramfs/bin/
cp ~/linux/samples/rust/module_kernel.ko ~/initramfs/
```

---

## 21. Crear enlaces de BusyBox

```bash
cd ~/initramfs/bin
for x in sh mount echo dmesg insmod rmmod uname cat ls; do ln -sf busybox "$x"; done
```

---

## 22. Crear el script `init`

```bash
cat > ~/initramfs/init <<'EOF'
#!/bin/sh
/bin/mount -t proc none /proc
/bin/mount -t sysfs none /sys

echo "Kernel en ejecucion:"
uname -r

echo "Cargando module_kernel.ko..."
insmod /module_kernel.ko

echo "Ultimos mensajes del kernel:"
dmesg | tail -n 30

exec sh
EOF
chmod +x ~/initramfs/init
```

---

## 23. Empaquetar initramfs

```bash
cd ~/initramfs
find . | cpio -o -H newc | gzip -9 > ~/initramfs.img
```

---

## 24. Arrancar el kernel compilado con QEMU

```bash
cd ~/linux
qemu-system-x86_64 \
  -kernel arch/x86/boot/bzImage \
  -initrd ~/initramfs.img \
  -nographic \
  -append "console=ttyS0"
```

---

## 25. Qué debes ver

Dentro de la consola de QEMU deberías ver:

- el nombre del kernel compilado
- la carga de `module_kernel.ko`
- el mensaje de `pr_info!` del módulo

Ejemplo esperado:

```text
module_kernel: hola mundo desde Rust
```

Si implementaste `Drop`, al descargarlo:

```bash
rmmod module_kernel
dmesg | tail -n 30
```

Deberías ver algo como:

```text
module_kernel: adios desde Rust
```

---

## Ventajas del Camino 1

- Es el camino correcto para este tipo de práctica.
- Garantiza compatibilidad entre kernel y módulo.
- Permite demostrar el mensaje del módulo sin depender del kernel de la distro.
- Sigue mejor la lógica de Rust-for-Linux.

---

## Desventajas del Camino 1

- Requiere preparar initramfs.
- Requiere usar QEMU o arrancar otro kernel.
- Toma más pasos que una simple carga con `insmod` en la VM.

---

# Parte 8. Camino 2: compilar para el kernel activo `6.14.0-27-generic`

Este camino busca que el módulo se pueda cargar directamente en la VM actual.

## ¿En qué consiste?

Consiste en compilar el módulo específicamente para el kernel que está corriendo:

```text
6.14.0-27-generic
```

Eso requiere que el kernel activo y su árbol de build estén preparados para soportar módulos en Rust.

---

## Problema importante de este camino

No basta con tener headers normales. Para módulos Rust se necesita que el kernel activo incluya soporte Rust y preparación adecuada del build.

En una distro estándar, esto puede no estar listo por defecto.

Eso vuelve este camino **más delicado y menos garantizado**.

---

## Flujo general del Camino 2

### 1. Verificar kernel activo

```bash
uname -r
```

### 2. Instalar headers

```bash
sudo apt install -y linux-headers-$(uname -r)
```

### 3. Usar el árbol de build del kernel activo

Ruta típica:

```text
/lib/modules/$(uname -r)/build
```

### 4. Compilar módulo externo

Flujo general:

```bash
make -C /lib/modules/$(uname -r)/build M=$PWD
```

---

## ¿Por qué no fue el camino elegido aquí?

Porque el trabajo ya se hizo sobre un árbol propio de kernel con soporte Rust y el módulo ya quedó compilado para ese kernel.

El objetivo de la práctica era demostrar compilación y ejecución de un módulo Rust en un entorno compatible, y el camino más fiable era el **Camino 1**.

---

## Ventajas del Camino 2

- Si funciona, permite cargar el módulo directamente en la VM con `insmod`.
- No obliga a arrancar otro kernel.

---

## Desventajas del Camino 2

- Requiere que el kernel activo de la distro soporte realmente módulos Rust.
- Puede fallar aunque existan headers.
- Es más difícil de reproducir en una presentación corta.

---

# Parte 9. Cómo volverlo a montar al presentar

## Escenario recomendado para presentar

Usar **Camino 1 con QEMU**.

¿Por qué?

Porque ya tienes:

- `arch/x86/boot/bzImage`
- `samples/rust/module_kernel.ko`

Solo necesitas volver a preparar el initramfs y arrancar QEMU.

---

## 26. Verificar que siguen existiendo los archivos

```bash
cd ~/linux
ls arch/x86/boot/bzImage
ls samples/rust/module_kernel.ko
```

---

## 27. Reconstruir el initramfs si hace falta

```bash
mkdir -p ~/initramfs/{bin,proc,sys}
cp /usr/bin/busybox ~/initramfs/bin/
cp ~/linux/samples/rust/module_kernel.ko ~/initramfs/
cd ~/initramfs/bin
for x in sh mount echo dmesg insmod rmmod uname cat ls; do ln -sf busybox "$x"; done
```

Crear `init` otra vez:

```bash
cat > ~/initramfs/init <<'EOF'
#!/bin/sh
/bin/mount -t proc none /proc
/bin/mount -t sysfs none /sys

echo "Kernel en ejecucion:"
uname -r

echo "Cargando module_kernel.ko..."
insmod /module_kernel.ko

echo "Ultimos mensajes del kernel:"
dmesg | tail -n 30

exec sh
EOF
chmod +x ~/initramfs/init
```

Empaquetar otra vez:

```bash
cd ~/initramfs
find . | cpio -o -H newc | gzip -9 > ~/initramfs.img
```

---

## 28. Arrancarlo el día de la presentación

```bash
cd ~/linux
qemu-system-x86_64 \
  -kernel arch/x86/boot/bzImage \
  -initrd ~/initramfs.img \
  -nographic \
  -append "console=ttyS0"
```

---

## 29. Qué mostrar en la presentación

### Evidencia 1: kernel activo dentro de QEMU

```bash
uname -r
```

Debe coincidir con el `vermagic` del módulo.

### Evidencia 2: metadata del módulo

Antes de arrancar QEMU:

```bash
modinfo ~/linux/samples/rust/module_kernel.ko
```

### Evidencia 3: mensaje del módulo

En la consola de QEMU, al ejecutar `insmod`, mostrar:

```text
module_kernel: hola mundo desde Rust
```

### Evidencia 4: descarga del módulo

Dentro de QEMU:

```bash
rmmod module_kernel
dmesg | tail -n 30
```

Mostrar el mensaje de salida del módulo.

---

# Parte 10. Comandos de diagnóstico útiles

## Verificar kernel actual

```bash
uname -r
```

## Verificar módulo compilado

```bash
find ~/linux -name '*.ko' | grep rust
```

## Ver metadata del módulo

```bash
modinfo ~/linux/samples/rust/module_kernel.ko
```

## Ver si Rust está habilitado

```bash
grep '^CONFIG_RUST=' ~/linux/.config
```

## Ver si el módulo está configurado como `m`

```bash
grep 'CONFIG_SAMPLE_RUST' ~/linux/.config
```

---

# Parte 11. Conclusión

## Estado final logrado

Se logró:

- preparar el entorno Rust dentro del kernel
- resolver conflictos de Kconfig
- habilitar `CONFIG_RUST=y`
- agregar un módulo propio en Rust
- compilar exitosamente `module_kernel.ko`

Archivo generado:

```text
samples/rust/module_kernel.ko
```

## Decisión recomendada

Para ver y demostrar el mensaje de `hola mundo`, el camino recomendado es:

### **Camino 1: arrancar con el kernel que compilaste**

porque garantiza compatibilidad con el módulo y permite mostrar claramente la carga y los mensajes del kernel.

---

# Parte 12. Checklist rápido para el día de presentación

## Antes de presentar

- [ ] Verificar `bzImage`
- [ ] Verificar `module_kernel.ko`
- [ ] Verificar `modinfo`
- [ ] Reconstruir `initramfs.img`
- [ ] Tener listo el comando de QEMU

## Durante la presentación

- [ ] Mostrar `modinfo module_kernel.ko`
- [ ] Arrancar QEMU
- [ ] Mostrar `uname -r`
- [ ] Mostrar carga de `module_kernel.ko`
- [ ] Mostrar `dmesg`
- [ ] Descargar módulo con `rmmod`

---

# Parte 13. Comandos finales resumidos

## Compilar

```bash
cd ~/linux
make LLVM=1 -j"$(nproc)"
find . -name '*.ko' | grep rust
modinfo ./samples/rust/module_kernel.ko
```

## Preparar demo

```bash
mkdir -p ~/initramfs/{bin,proc,sys}
cp /usr/bin/busybox ~/initramfs/bin/
cp ~/linux/samples/rust/module_kernel.ko ~/initramfs/
cd ~/initramfs/bin
for x in sh mount echo dmesg insmod rmmod uname cat ls; do ln -sf busybox "$x"; done
```

```bash
cat > ~/initramfs/init <<'EOF'
#!/bin/sh
/bin/mount -t proc none /proc
/bin/mount -t sysfs none /sys
uname -r
insmod /module_kernel.ko
dmesg | tail -n 30
exec sh
EOF
chmod +x ~/initramfs/init
```

```bash
cd ~/initramfs
find . | cpio -o -H newc | gzip -9 > ~/initramfs.img
```

```bash
cd ~/linux
qemu-system-x86_64 \
  -kernel arch/x86/boot/bzImage \
  -initrd ~/initramfs.img \
  -nographic \
  -append "console=ttyS0"
```
