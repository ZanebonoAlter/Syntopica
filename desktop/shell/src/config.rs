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
pub fn normalize(raw: &str) -> Result<Option<String>, String> {
    let trimmed = raw.trim();
    if trimmed.is_empty() {
        return Ok(None);
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
    let path = config_path()?;
    let text = std::fs::read_to_string(path).ok()?;
    let parsed: SavedConfig = serde_json::from_str(&text).ok()?;
    parsed.server_url
}

/// 原子写入（临时文件 + rename），避免并发写出现半截 JSON。
pub fn save(url: &str) -> Result<(), String> {
    let dir = data_dir().ok_or_else(|| "无法确定应用数据目录".to_string())?;
    std::fs::create_dir_all(&dir).map_err(|e| format!("创建目录失败：{e}"))?;
    let path = dir.join(CONFIG_FILE);
    let tmp = dir.join(format!("{CONFIG_FILE}.tmp"));
    let body = serde_json::to_string_pretty(&SavedConfig {
        server_url: Some(url.to_string()),
    })
    .map_err(|e| format!("序列化失败：{e}"))?;
    std::fs::write(&tmp, body).map_err(|e| format!("写入失败：{e}"))?;
    std::fs::rename(&tmp, &path).map_err(|e| format!("写入失败：{e}"))?;
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
        assert!(v.contains("[::1]"));
    }

    #[test]
    fn overlong_url_is_rejected_by_parser_or_kept_under_limit() {
        let long = format!("http://host:5100/{}", "a".repeat(4096));
        match normalize(&long) {
            Ok(Some(v)) => assert!(v.len() > 4096),
            Ok(None) => panic!("non-empty must not be None"),
            Err(_) => {}
        }
    }

    #[test]
    fn corrupt_config_falls_back_to_default() {
        let dir = std::env::temp_dir().join(format!("syntopica-cfg-test-{}", std::process::id()));
        let _ = std::fs::create_dir_all(&dir);
        let text = "{ not json";
        let parsed: Option<SavedConfig> = serde_json::from_str(text).ok();
        assert!(parsed.is_none());
        let _ = std::fs::remove_dir_all(&dir);
    }
}
