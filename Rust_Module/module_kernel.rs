// SPDX-License-Identifier: GPL-2.0

use kernel::prelude::*;

module! {
    type: RustHola,
    name: "rust_hola",
    authors: ["Julian Reyes"],
    description: "Modulo Linux en Rust: hola mundo",
    license: "GPL",
}

struct RustHola;

impl kernel::Module for RustHola {
    fn init(_module: &'static ThisModule) -> Result<Self> {
        pr_info!("Hola mundo desde un modulo Linux en Rust\n");
        pr_info!("Saluda Carnet: 201905884\n");
        Ok(RustHola)
    }
}

impl Drop for RustHola {
    fn drop(&mut self) {
        pr_info!("Adios desde rust_hola\n");
        pr_info!("Adios Carnet: 201905884\n");
    }
}