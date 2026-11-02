#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

//! Syntopica 桌面壳（远程模式）。
//!
//! 窗口装载服务端已托管的 SPA（同源 `/api`），远端不可达时加载本地失败页。
//! 平台适配：Linux 禁用 WebKitGTK 的 DMABUF 渲染器（否则整窗花屏）。

mod config;
mod nav;

use std::time::Duration;
use tauri::{Manager, WebviewUrl, WebviewWindowBuilder};

/// 启动探测超时：既要快（用户等窗口），又要容忍慢服务端。
const PROBE_TIMEOUT: Duration = Duration::from_secs(5);
const WINDOW_LABEL: &str = "main";

#[tauri::command]
fn probe_server(window: tauri::WebviewWindow, url: String) -> Result<(), String> {
    match nav::probe(&url, PROBE_TIMEOUT) {
        nav::Probe::Reachable => {
            let parsed = tauri::Url::parse(&url).map_err(|e| format!("地址无效：{e}"))?;
            window.navigate(parsed).map_err(|e| format!("导航失败：{e}"))
        }
        other => Err(other.reason()),
    }
}

#[tauri::command]
fn set_server_url(window: tauri::WebviewWindow, url: String) -> Result<(), String> {
    let normalized = config::normalize(&url)?.ok_or_else(|| "地址不能为空".to_string())?;
    config::save(&normalized)?;
    probe_server(window, normalized)
}

#[tauri::command]
fn current_target() -> config::Resolved {
    let args: Vec<String> = std::env::args().skip(1).collect();
    config::resolve(&args, std::env::var(config::ENV_VAR).ok(), config::load_saved())
}

fn initial_url(resolved: &config::Resolved) -> WebviewUrl {
    let probe = nav::probe(&resolved.url, PROBE_TIMEOUT);
    if probe.is_ok() {
        if let Ok(url) = tauri::Url::parse(&resolved.url) {
            return WebviewUrl::External(url);
        }
    }
    let query = format!(
        "error.html?target={}&reason={}&source={}",
        nav::encode(&resolved.url),
        nav::encode(&probe.reason()),
        nav::encode(resolved.source.label()),
    );
    WebviewUrl::App(query.into())
}

fn main() {
    // Linux（WebKitGTK）：DMABUF 零拷贝渲染在 Pi/Mesa 上整窗花屏；必须在建窗口前关闭。
    #[cfg(target_os = "linux")]
    std::env::set_var("WEBKIT_DISABLE_DMABUF_RENDERER", "1");

    ensure_webview2();

    let args: Vec<String> = std::env::args().skip(1).collect();
    let resolved = config::resolve(&args, std::env::var(config::ENV_VAR).ok(), config::load_saved());
    println!(
        "[shell] target={} source={}",
        resolved.url,
        resolved.source.label()
    );

    tauri::Builder::default()
        .plugin(tauri_plugin_single_instance::init(|app, _args, _cwd| {
            if let Some(window) = app.get_webview_window(WINDOW_LABEL) {
                let _ = window.unminimize();
                let _ = window.show();
                let _ = window.set_focus();
            }
        }))
        .invoke_handler(tauri::generate_handler![probe_server, set_server_url, current_target])
        .setup(move |app| {
            let url = initial_url(&resolved);
            WebviewWindowBuilder::new(app, WINDOW_LABEL, url)
                .title("Syntopica")
                .inner_size(1100.0, 720.0)
                .min_inner_size(900.0, 600.0)
                .build()?;
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("failed to run Syntopica shell");
}

/// Windows：启动前确认 WebView2 运行时存在，缺失时给出可见提示并退出（spec: 缺少 WebView2 Runtime）。
/// 安装包默认走 Evergreen bootstrapper，正常情况下此检查不会触发；它兜住「装完后被卸载/精简系统」的情况。
#[cfg(target_os = "windows")]
fn ensure_webview2() {
    const KEY: &str = r"HKLM\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}";

    let found = std::process::Command::new("reg")
        .args(["query", KEY, "/v", "pv"])
        .output()
        .map(|out| out.status.success())
        .unwrap_or(true); // 查询本身失败时不阻断启动

    if !found {
        message_box(
            "Syntopica 需要 Microsoft Edge WebView2 运行时。\n\n请安装后重新启动：\nhttps://developer.microsoft.com/microsoft-edge/webview2/",
            "Syntopica · 缺少 WebView2 运行时",
        );
        std::process::exit(2);
    }
}

#[cfg(target_os = "windows")]
fn message_box(text: &str, caption: &str) {
    #[link(name = "user32")]
    extern "system" {
        fn MessageBoxW(hwnd: *mut core::ffi::c_void, text: *const u16, caption: *const u16, utype: u32) -> i32;
    }
    const MB_ICONERROR: u32 = 0x0000_0010;

    let wide = |s: &str| -> Vec<u16> { s.encode_utf16().chain(std::iter::once(0)).collect() };
    let (t, c) = (wide(text), wide(caption));
    unsafe {
        MessageBoxW(std::ptr::null_mut(), t.as_ptr(), c.as_ptr(), MB_ICONERROR);
    }
}

#[cfg(not(target_os = "windows"))]
fn ensure_webview2() {}
