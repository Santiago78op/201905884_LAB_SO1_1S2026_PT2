# Manual de Implementación Modulo Kernel en Rust

# Indice
1. [Introducción](#introducción)
2. [Requisitos](#requisitos)
3. [Estructura del Proyecto](#estructura-del-proyecto)
4. [Implementación del Módulo Kernel](#implementación-del-módulo-kernel)
5. [Compilación y Pruebas](#compilación-y-pruebas)

## Introducción

Rust ha surgido como una herramienta poderosa para la programación de sistemas, ofreciendo seguridad en la memoria sin la carga de un recogedor de basura. Este tutorial se centra en desarrollar un módulo de kernel de alto rendimiento usando Rust, una tarea tradicionalmente dominada por C. Al final, aprenderás a crear un módulo de kernel, interactuar con hardware y manejar operaciones de bajo nivel de forma segura.

## Requisitos

- Conocimientos básicos de Rust y programación de sistemas.
- Un entorno de desarrollo Linux con herramientas de compilación y acceso al kernel.
- Familiaridad con la estructura del kernel de Linux y su API.

## Estructura del Proyecto

El proyecto se organiza en los siguientes archivos:

- `Cargo.toml`: Archivo de configuración de Rust.
- `src/lib.rs`: Código fuente del módulo kernel.
- `Makefile`: Script para compilar el módulo kernel.

## Implementación del Módulo Kernel

En `src/lib.rs`, implementamos el módulo kernel utilizando la crate `kernel_module`. El código define dos funciones principales: `on_init` para la inicialización del módulo y `on_cleanup` para la limpieza al descargar el módulo.

```rust
// src/lib.rs
#![no_std] // No usamos la biblioteca estándar de Rust
#![feature(custom_attributes)] // Habilitamos atributos personalizados
#![feature(abi_x86_interrupt)] // Habilitamos el ABI para interrupciones x86

use core::ffi::c_void; // Importamos tipos C para interoperabilidad con el kernel
use kernel_module::{c_types, module::{int_module, cleanup_module}}; // Importamos macros para definir funciones de inicialización y limpieza

// Función de inicialización del módulo kernel
#[inti]
fn on_init() -> c_types::c_int {
    println!("Rust module init");
    println!("Hello, Kernel! 201905884");
    0 // Return 0 to indicate successful initialization
}

// Función de limpieza del módulo kernel
#[cleanup]
fn on_cleanup() {
    println!("Rust module cleanup");
}
```

## Compilación y Pruebas

Para compilar el módulo kernel, utilizamos el `Makefile` que invoca `cargo build` con las opciones adecuadas para generar un módulo compatible con el kernel de Linux. Después de compilar, puedes cargar el módulo usando `insmod` y verificar su funcionamiento con `dmesg`.

**Paso para compilar:**

- 1. Creación de .cargo/config.toml
    * Se debe crear un archivo de configuración para especificar el objetivo de compilación, ya que estamos trabajando con `no_std` y necesitamos apuntar a un entorno específico del kernel.

```toml
[build]
target = "x86_64-unknown-linux-gnu" # for no_std
```

- 2. Creación del Makefile
    * El Makefile debe contener las reglas para compilar el módulo kernel utilizando `cargo` y generar el archivo `.ko` que se puede cargar en el kernel.

```makefile 
[package]
version = "0.1.0"
edition = "2021"

[dependencies]
kernel-module = { version = "0.1.0", features = ["module"] }
```

- 3. Compilación del módulo
    * Ejecuta `make` en la terminal para compilar el módulo kernel. Esto generará un archivo `.ko` que se puede cargar en el kernel.

```bash
make
```

- 4. Carga del módulo
    * Usa `insmod` para cargar el módulo en el kernel y `dmesg` para verificar que se ha cargado correctamente.

```bash
sudo insmod rust_module.ko
dmesg | tail
```

- 5. Limpieza del módulo
    * Para descargar el módulo, utiliza `rmmod` y verifica la limpieza con `dmesg`.

```bash
sudo rmmod rust_module
dmesg | tail
```