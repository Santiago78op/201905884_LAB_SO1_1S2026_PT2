
// src/lib.rs
/** 
 * Rust Module for Linux Kernel
 * Author: 201905884
 * 
 * no_std: This attribute indicates that the Rust standard library is not linked to the crate.
 * 
 * custom_attributes: This feature allows the use of custom attributes in the code, 
 * which can be used for various purposes such as marking functions or 
 * data structures with specific metadata.
 * 
 * abi_x86_interrupt: This feature enables the use of the x86 interrupt calling convention,
 * which is necessary for writing interrupt handlers in Rust for the Linux kernel.
 * 
*/
#![no_std]
#![feature(custom_attributes)]
#![feature(abi_x86_interrupt)]

use core::ffi::c_void;
use kernel_module::{c_types, module::{int_module, cleanup_module}};

#[inti]
fn on_init() -> c_types::c_int {
    println!("Rust module init");
    println!("Hello, Kernel! 201905884");
    0 // Return 0 to indicate successful initialization
}

#[cleanup]
fn on_cleanup() {
    println!("Rust module cleanup");
}