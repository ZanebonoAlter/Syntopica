//! 目标可达性探测与失败分类（spec: 远端不可达时提供可恢复的失败页）。

use std::io::{Read, Write};
use std::net::{TcpStream, ToSocketAddrs};
use std::time::Duration;

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Probe {
    Reachable,
    Refused,
    Timeout,
    Status(u16),
    BadResponse(String),
    Invalid(String),
}

impl Probe {
    pub fn is_ok(&self) -> bool {
        matches!(self, Probe::Reachable)
    }

    /// 失败页展示给用户的原因（中文、面向用户，不暴露内部细节）。
    pub fn reason(&self) -> String {
        match self {
            Probe::Reachable => "服务端可达".to_string(),
            Probe::Refused => "服务端未启动，或该地址不可达".to_string(),
            Probe::Timeout => "连接超时，服务端可能繁忙或网络不通".to_string(),
            Probe::Status(code) => format!("服务端返回异常状态 {code}"),
            Probe::BadResponse(detail) => format!("服务端响应异常：{detail}"),
            Probe::Invalid(msg) => format!("地址无效：{msg}"),
        }
    }
}

/// HTTP 状态分类：2xx/3xx 视为可达，其余视为失败。
pub fn classify_status(code: u16) -> Probe {
    if (200..400).contains(&code) {
        Probe::Reachable
    } else {
        Probe::Status(code)
    }
}

/// 解析响应首行 `HTTP/1.1 200 OK` 为状态码。
pub fn parse_status_line(line: &str) -> Option<u16> {
    let mut parts = line.split_whitespace();
    let version = parts.next()?;
    if !version.starts_with("HTTP/") {
        return None;
    }
    parts.next()?.parse::<u16>().ok()
}

/// 探测目标：`http://` 取真实状态码；`https://` 只做 TCP 可达性检查（本壳不需要 TLS 握手，
/// 明文 HTTP 场景才是文档化用法，见 desktop/README.md「已知限制」）。
pub fn probe(raw: &str, timeout: Duration) -> Probe {
    let url = match tauri::Url::parse(raw) {
        Ok(u) => u,
        Err(e) => return Probe::Invalid(e.to_string()),
    };
    let scheme = url.scheme().to_string();
    let default_port = match scheme.as_str() {
        "http" => 80,
        "https" => 443,
        other => return Probe::Invalid(format!("仅支持 http/https，当前为 {other}")),
    };
    let host = match url.host_str() {
        Some(h) if !h.is_empty() => h.to_string(),
        _ => return Probe::Invalid("地址缺少主机名".to_string()),
    };
    let port = url.port().unwrap_or(default_port);
    let mut path = url.path().to_string();
    if path.is_empty() {
        path.push('/');
    }
    if let Some(q) = url.query() {
        path.push('?');
        path.push_str(q);
    }

    let mut addrs = match (host.as_str(), port).to_socket_addrs() {
        Ok(a) => a,
        Err(e) => return Probe::BadResponse(format!("域名解析失败：{e}")),
    };
    let addr = match addrs.next() {
        Some(a) => a,
        None => return Probe::BadResponse("域名解析无结果".to_string()),
    };

    let mut stream = match TcpStream::connect_timeout(&addr, timeout) {
        Ok(s) => s,
        Err(e) => {
            return if e.kind() == std::io::ErrorKind::TimedOut {
                Probe::Timeout
            } else {
                Probe::Refused
            }
        }
    };
    let _ = stream.set_read_timeout(Some(timeout));
    let _ = stream.set_write_timeout(Some(timeout));

    if scheme == "https" {
        // 只验证端口可连（不做 TLS 握手）。
        return Probe::Reachable;
    }

    let request = format!(
        "GET {path} HTTP/1.1\r\nHost: {host}:{port}\r\nUser-Agent: syntopica-shell/{}\r\nAccept: text/html\r\nConnection: close\r\n\r\n",
        env!("CARGO_PKG_VERSION")
    );
    if let Err(e) = stream.write_all(request.as_bytes()) {
        return Probe::BadResponse(format!("写入请求失败：{e}"));
    }

    let mut buf = [0u8; 512];
    let n = match stream.read(&mut buf) {
        Ok(n) => n,
        Err(e) => {
            return if e.kind() == std::io::ErrorKind::TimedOut || e.kind() == std::io::ErrorKind::WouldBlock {
                Probe::Timeout
            } else {
                Probe::BadResponse(format!("读取响应失败：{e}"))
            }
        }
    };
    if n == 0 {
        return Probe::BadResponse("连接被关闭且无响应".to_string());
    }
    let head = String::from_utf8_lossy(&buf[..n]);
    match head.lines().next().and_then(parse_status_line) {
        Some(code) => classify_status(code),
        None => Probe::BadResponse("响应不是 HTTP".to_string()),
    }
}

/// 把任意文本编码成 URL query 值（只保留 unreserved 字符）。
pub fn encode(value: &str) -> String {
    let mut out = String::with_capacity(value.len());
    for byte in value.as_bytes() {
        match byte {
            b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'_' | b'.' | b'~' => {
                out.push(*byte as char)
            }
            _ => out.push_str(&format!("%{byte:02X}")),
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn two_hundred_is_reachable() {
        assert_eq!(classify_status(200), Probe::Reachable);
    }

    #[test]
    fn redirect_is_reachable() {
        assert_eq!(classify_status(302), Probe::Reachable);
    }

    #[test]
    fn server_error_is_status() {
        assert_eq!(classify_status(500), Probe::Status(500));
        assert_eq!(classify_status(404), Probe::Status(404));
    }

    #[test]
    fn invalid_url_reported() {
        assert!(matches!(probe("not a url", Duration::from_millis(50)), Probe::Invalid(_)));
    }

    #[test]
    fn status_line_parsing() {
        assert_eq!(parse_status_line("HTTP/1.1 204 No Content"), Some(204));
        assert_eq!(parse_status_line("HTTP/1.0 500 Internal Server Error"), Some(500));
        assert_eq!(parse_status_line("garbage"), None);
        assert_eq!(parse_status_line(""), None);
    }

    #[test]
    fn non_http_scheme_rejected_without_network() {
        match probe("ftp://example.com/", Duration::from_millis(50)) {
            Probe::Invalid(msg) => assert!(msg.contains("仅支持")),
            other => panic!("expected Invalid, got {other:?}"),
        }
    }

    #[test]
    fn encode_escapes_reserved_chars() {
        assert_eq!(encode("http://a:1/?x=1&y=2"), "http%3A%2F%2Fa%3A1%2F%3Fx%3D1%26y%3D2");
        assert_eq!(encode("中文"), "%E4%B8%AD%E6%96%87");
        assert_eq!(encode("abc-._~"), "abc-._~");
    }

    #[test]
    fn reasons_are_non_empty() {
        for p in [
            Probe::Refused,
            Probe::Timeout,
            Probe::Status(502),
            Probe::BadResponse("x".into()),
            Probe::Invalid("x".into()),
        ] {
            assert!(!p.reason().is_empty());
        }
    }
}
