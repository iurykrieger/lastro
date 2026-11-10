//! Minimal WASM ABI (no wasm-bindgen):
//!
//! 1. `lastro_alloc(len) -> ptr` and write the UTF-8 request at `ptr`.
//! 2. `lastro_prove(ptr, len) -> u64`: high 32 bits = result ptr, low 32 bits = result len.
//! 3. Read the UTF-8 JSON result, then `lastro_free(ptr, len)` both buffers.

use std::alloc::{alloc, dealloc, Layout};

#[no_mangle]
pub extern "C" fn lastro_alloc(len: usize) -> *mut u8 {
    if len == 0 {
        return std::ptr::null_mut();
    }
    unsafe { alloc(Layout::from_size_align(len, 1).expect("layout")) }
}

/// # Safety
/// `ptr` must come from `lastro_alloc(len)` (or a result of `lastro_prove`) and not be freed twice.
#[no_mangle]
pub unsafe extern "C" fn lastro_free(ptr: *mut u8, len: usize) {
    if !ptr.is_null() && len > 0 {
        dealloc(ptr, Layout::from_size_align(len, 1).expect("layout"));
    }
}

/// # Safety
/// `ptr..ptr+len` must be a valid UTF-8 buffer written by the host.
#[no_mangle]
pub unsafe extern "C" fn lastro_prove(ptr: *const u8, len: usize) -> u64 {
    let bytes = std::slice::from_raw_parts(ptr, len);
    let out = match std::str::from_utf8(bytes) {
        Ok(s) => crate::prove_json(s),
        Err(_) => r#"{"error":"request is not UTF-8"}"#.to_string(),
    };
    let out = out.into_bytes();
    let n = out.len();
    let dst = lastro_alloc(n);
    std::ptr::copy_nonoverlapping(out.as_ptr(), dst, n);
    ((dst as u64) << 32) | n as u64
}
