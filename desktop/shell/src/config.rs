//! 目标地址解析与本地配置读写。
//!
//! 优先级（spec: 窗口按配置地址装载现有 SPA）：
//!   启动参数 --server-url > 环境变量 SYNTOPICA_SERVER_URL > 用户本地配置 > 默认值。

use std::path::PathBuf;

pub const ENV_VAR: &str = "SYNTOPICA_SERVER_URL";
pub const DEFAULT_URL: &str = "http://127.0.0.1:5100/";
const CONFIG_FILE: &str = "shell.json";
#[cfg(target_os = "windows")]
const APP_DIR: &str = "Syntopica";
#[cfg(not(target_os = "windows"))]
const APP_DIR: &str = ".syntopica";

/// 地址来源，用于日志与失败页展示。
#[derive(Debug, Clone, Copy, PartialEq, Eq, serde::Serialize)]
pub enum Source {
    Arg,
    Env,
    File,
    Default,
}

impl Source {
    pub fn label(self) -> &'static str {
        match self {
            Source::Arg => "启动参数",
            Source::Env => "环境变量",
            Source::File => "本地配置",
            Source::Default => "默认值",
        }
    }
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize)]
pub struct Resolved {
    pub url: String,
    pub source: Source,
}

/// 应用数据目录：Windows 用 `%LOCALAPPDATA%\Syntopica`，其他平台用 `~/.syntopica`。
pub fn data_dir() -> Option<PathBuf> {
    #[cfg(target_os = "windows")]
    {
        std::env::var_os("LOCALAPPDATA").map(|p| PathBuf::from(p).join(APP_DIR))
    }
    #[cfg(not(target_os = "windows"))]
    {
        std::env::var_os("HOME").map(|p| PathBuf::from(p).join(APP_DIR))
    }
}

pub fn config_path() -> Option<PathBuf> {
    data_dir().map(|d| d.join(CONFIG_FILE))
}

/// 归一化并校验地址；空值返回 `Ok(None)`（视为未提供，交由下一优先级）。
/// 长度上限 8KB：防配置文件/参数里塞入异常长串（浏览器实际可用上限远高于此，这只是壳层防御）。
pub const MAX_URL_LEN: usize = 8 * 1024;

pub fn normalize(raw: &str) -> Result<Option<String>, String> {
    let trimmed = raw.trim();
    if trimmed.is_empty() {
        return Ok(None);
    }
    if trimmed.len() > MAX_URL_LEN {
        return Err(format!("地址过长（>{MAX_URL_LEN} 字符）"));
    }
    let url = tauri::Url::parse(trimmed).map_err(|e| format!("地址无法解析：{e}"))?;
    match url.scheme() {
        "http" | "https" => {}
        other => return Err(format!("仅支持 http/https，当前为 {other}")),
    }
    if url.host_str().unwrap_or("").is_empty() {
        return Err("地址缺少主机名".to_string());
    }
    Ok(Some(url.to_string()))
}

/// 从启动参数中取 `--server-url <value>` 或 `--server-url=<value>`。
pub fn arg_value(args: &[String]) -> Option<String> {
    let mut iter = args.iter();
    while let Some(a) = iter.next() {
        if let Some(v) = a.strip_prefix("--server-url=") {
            return Some(v.to_string());
        }
        if a == "--server-url" {
            return iter.next().cloned();
        }
    }
    None
}

/// 按优先级解析出最终地址。`saved` 为本地配置里的原始文本。
pub fn resolve(args: &[String], env: Option<String>, saved: Option<String>) -> Resolved {
    let candidates = [
        (arg_value(args), Source::Arg),
        (env, Source::Env),
        (saved, Source::File),
    ];
    for (value, source) in candidates {
        if let Some(raw) = value {
            if let Ok(Some(url)) = normalize(&raw) {
                return Resolved { url, source };
            }
        }
    }
    Resolved {
        url: DEFAULT_URL.to_string(),
        source: Source::Default,
    }
}

#[derive(Debug, Clone, Default, serde::Serialize, serde::Deserialize)]
struct SavedConfig {
    #[serde(default)]
    server_url: Option<String>,
}

/// 读取本地配置；文件不存在或内容损坏时返回 `None`（损坏时不覆写原文件）。
pub fn load_saved() -> Option<String> {
    read_config(&config_path()?).ok().flatten()
}

/// 路径参数化的读取：`Ok(None)` = 文件不存在；`Err` = 存在但损坏（调用方回退且不覆写）。
fn read_config(path: &std::path::Path) -> Result<Option<String>, ()> {
    let text = match std::fs::read_to_string(path) {
        Ok(t) => t,
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => return Ok(None),
        Err(_) => return Err(()),
    };
    let parsed: SavedConfig = serde_json::from_str(&text).map_err(|_| ())?;
    Ok(parsed.server_url)
}

/// 原子写入（临时文件 + rename），避免并发写出现半截 JSON。
pub fn save(url: &str) -> Result<(), String> {
    let dir = data_dir().ok_or_else(|| "无法确定应用数据目录".to_string())?;
    write_config(&dir.join(CONFIG_FILE), url)
}

/// 路径参数化的写入（测试与 `save` 共用）。
fn write_config(path: &std::path::Path, url: &str) -> Result<(), String> {
    let dir = path.parent().ok_or_else(|| "路径缺少父目录".to_string())?;
    std::fs::create_dir_all(dir).map_err(|e| format!("创建目录失败：{e}"))?;
    let tmp = dir.join(format!("{}.tmp", path.file_name().and_then(|n| n.to_str()).unwrap_or(CONFIG_FILE)));
    let body = serde_json::to_string_pretty(&SavedConfig {
        server_url: Some(url.to_string()),
    })
    .map_err(|e| format!("序列化失败：{e}"))?;
    std::fs::write(&tmp, body).map_err(|e| format!("写入失败：{e}"))?;
    std::fs::rename(&tmp, path).map_err(|e| format!("写入失败：{e}"))?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn args(items: &[&str]) -> Vec<String> {
        items.iter().map(|s| s.to_string()).collect()
    }

    #[test]
    fn arg_wins_over_everything() {
        let r = resolve(
            &args(&["--server-url", "http://10.0.0.5:5100/"]),
            Some("http://env:5100/".into()),
            Some("http://file:5100/".into()),
        );
        assert_eq!(r.url, "http://10.0.0.5:5100/");
        assert_eq!(r.source, Source::Arg);
    }

    #[test]
    fn arg_equals_form_is_supported() {
        let r = resolve(&args(&["--server-url=http://10.0.0.6:5100/"]), None, None);
        assert_eq!(r.url, "http://10.0.0.6:5100/");
        assert_eq!(r.source, Source::Arg);
    }

    #[test]
    fn env_wins_over_file() {
        let r = resolve(&[], Some("http://env:5100/".into()), Some("http://file:5100/".into()));
        assert_eq!(r.url, "http://env:5100/");
        assert_eq!(r.source, Source::Env);
    }

    #[test]
    fn file_used_when_no_arg_or_env() {
        let r = resolve(&[], None, Some("http://file:5100/".into()));
        assert_eq!(r.url, "http://file:5100/");
        assert_eq!(r.source, Source::File);
    }

    #[test]
    fn default_when_nothing_provided() {
        let r = resolve(&[], None, None);
        assert_eq!(r.url, DEFAULT_URL);
        assert_eq!(r.source, Source::Default);
    }

    #[test]
    fn blank_and_whitespace_fall_through() {
        let r = resolve(&args(&["--server-url", "   "]), Some("\t".into()), None);
        assert_eq!(r.source, Source::Default);
        assert_eq!(r.url, DEFAULT_URL);
    }

    #[test]
    fn invalid_arg_falls_through_to_next_level() {
        let r = resolve(
            &args(&["--server-url", "10.0.0.5:5100"]),
            Some("http://env:5100/".into()),
            None,
        );
        assert_eq!(r.url, "http://env:5100/");
        assert_eq!(r.source, Source::Env);
    }

    #[test]
    fn unsupported_scheme_is_rejected() {
        assert!(normalize("ftp://host:5100").is_err());
        assert!(normalize("file:///tmp/x").is_err());
    }

    #[test]
    fn missing_scheme_is_rejected() {
        assert!(normalize("10.11.12.55:5100").is_err());
    }

    #[test]
    fn ipv6_and_trailing_slash_normalized() {
        let v = normalize("http://[::1]:5100").unwrap().unwrap();
        assert_eq!(v, "http://[::1]:5100/");
        // 带路径时尾斜杠不追加
        let v = normalize("http://h:5100/app").unwrap().unwrap();
        assert_eq!(v, "http://h:5100/app");
        // 已有尾斜杠保持不变
        let v = normalize("http://h:5100/").unwrap().unwrap();
        assert_eq!(v, "http://h:5100/");
    }

    #[test]
    fn overlong_url_has_closed_verdict() {
        // ≤8KB 接受
        let ok = format!("http://host:5100/{}", "a".repeat(4 * 1024));
        assert!(normalize(&ok).unwrap().is_some());
        // >8KB 拒绝（壳层防御上限 MAX_URL_LEN）
        let bad = format!("http://host:5100/{}", "a".repeat(MAX_URL_LEN));
        assert!(normalize(&bad).is_err());
    }

    #[test]
    fn save_load_roundtrip_via_parameterized_paths() {
        let dir = std::env::temp_dir().join(format!("syntopica-cfg-{}-{}", std::process::id(), line!()));
        let path = dir.join(CONFIG_FILE);
        // 不存在 → Ok(None)
        assert_eq!(read_config(&path).unwrap(), None);
        // 写入 → 读回一致
        write_config(&path, "http://192.168.5.24:5100/").unwrap();
        assert_eq!(read_config(&path).unwrap(), Some("http://192.168.5.24:5100/".to_string()));
        // 覆写 → 读回新值，无残留 tmp
        write_config(&path, "http://10.11.12.55:5100/").unwrap();
        assert_eq!(read_config(&path).unwrap(), Some("http://10.11.12.55:5100/".to_string()));
        assert!(!dir.join(format!("{CONFIG_FILE}.tmp")).exists());
        let _ = std::fs::remove_dir_all(&dir);
    }

    #[test]
    fn corrupt_config_reports_error_and_is_not_overwritten() {
        let dir = std::env::temp_dir().join(format!("syntopica-cfg-{}-{}", std::process::id(), line!()));
        let path = dir.join(CONFIG_FILE);
        write_config(&path, "http://a:5100/").unwrap();
        std::fs::write(&path, "{ not json").unwrap();
        // 损坏 → Err（调用方回退且不覆写）
        assert!(read_config(&path).is_err());
        // 仅读不写：文件内容保持损坏原样（save 只由用户显式保存触发）
        assert_eq!(std::fs::read_to_string(&path).unwrap(), "{ not json");
        let _ = std::fs::remove_dir_all(&dir);
    }

    #[cfg(unix)]
    #[test]
    fn readonly_dir_yields_visible_error() {
        use std::os::unix::fs::PermissionsExt;
        let dir = std::env::temp_dir().join(format!("syntopica-cfg-{}-{}", std::process::id(), line!()));
        std::fs::create_dir_all(&dir).unwrap();
        std::fs::set_permissions(&dir, std::fs::Permissions::from_mode(0o500)).unwrap();
        let err = write_config(&dir.join(CONFIG_FILE), "http://a:5100/");
        std::fs::set_permissions(&dir, std::fs::Permissions::from_mode(0o700)).unwrap();
        let _ = std::fs::remove_dir_all(&dir);
        assert!(err.is_err(), "只读目录写入应返回可见错误，实际 {err:?}");
    }
}
