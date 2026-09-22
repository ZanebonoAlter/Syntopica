import { describe, expect, it } from "vitest";
import { proxyImagesInHtml, proxiedImageUrl } from "./imageProxy";

describe("proxiedImageUrl", () => {
  it("F5: http(s) 外链改写为同源代理地址（encodeURIComponent 不吞特殊字符）", () => {
    const raw = "https://cdnfile.sspai.com/2026/09/21/a.png?imageView2/2/w/1120&q=90";
    expect(proxiedImageUrl(raw)).toBe(`/api/image-proxy?url=${encodeURIComponent(raw)}`);
    expect(proxiedImageUrl("http://example.com/a b.png")).toBe(
      `/api/image-proxy?url=${encodeURIComponent("http://example.com/a b.png")}`,
    );
  });

  it("F1: 空值/纯空白 falsy 原样（空串进空串出）", () => {
    expect(proxiedImageUrl("")).toBe("");
    expect(proxiedImageUrl(null)).toBe("");
    expect(proxiedImageUrl(undefined)).toBe("");
  });

  it("F4: 相对路径与协议相对地址原样", () => {
    expect(proxiedImageUrl("/icons/feeds/42.png")).toBe("/icons/feeds/42.png");
    expect(proxiedImageUrl("images/a.png")).toBe("images/a.png");
    expect(proxiedImageUrl("//cdn.example.com/a.png")).toBe("//cdn.example.com/a.png");
  });

  it("F3: data: 与 blob: 原样", () => {
    expect(proxiedImageUrl("data:image/png;base64,iVBORw0KGgo=")).toBe("data:image/png;base64,iVBORw0KGgo=");
    expect(proxiedImageUrl("blob:http://localhost/uuid")).toBe("blob:http://localhost/uuid");
  });

  it("F2: 已带代理前缀的地址原样（防二次嵌套）", () => {
    const proxied = "/api/image-proxy?url=https%3A%2F%2Fa.com%2Fb.png";
    expect(proxiedImageUrl(proxied)).toBe(proxied);
  });

  it("F6: 指向当前页面源的同源绝对地址原样（避免代理自指被 400）", () => {
    const origin = window.location.origin;
    expect(proxiedImageUrl(`${origin}/icons/x.png`)).toBe(`${origin}/icons/x.png`);
    expect(proxiedImageUrl("https://other.example/a.png")).toBe(
      `/api/image-proxy?url=${encodeURIComponent("https://other.example/a.png")}`,
    );
  });

  it("边界: scheme 大小写混合也识别为外链", () => {
    expect(proxiedImageUrl("HTTPS://CDN.Example.com/A.PNG")).toBe(
      `/api/image-proxy?url=${encodeURIComponent("HTTPS://CDN.Example.com/A.PNG")}`,
    );
  });
});

describe("proxyImagesInHtml", () => {
  it("外链 img src 批量改写，双引号与单引号都覆盖", () => {
    const html = `<p><img src="https://cdn.example.com/a.png"><img src='https://cdn.example.com/b.png'></p>`;
    const out = proxyImagesInHtml(html);
    expect(out).toContain(`src="/api/image-proxy?url=${encodeURIComponent("https://cdn.example.com/a.png")}"`);
    expect(out).toContain(`src='/api/image-proxy?url=${encodeURIComponent("https://cdn.example.com/b.png")}'`);
    expect(out).not.toContain('src="https://cdn');
  });

  it("data: 与相对路径 img 不动，无 img 的 HTML 原样", () => {
    const html = `<img src="data:image/png;base64,xxx"><img src="/local/a.png"><p>no img</p>`;
    expect(proxyImagesInHtml(html)).toBe(html);
    expect(proxyImagesInHtml("")).toBe("");
    expect(proxyImagesInHtml(null)).toBe("");
  });

  it("大小写混合的 <IMG SRC> 同样改写", () => {
    const out = proxyImagesInHtml(`<IMG SRC="https://cdn.example.com/a.png">`);
    expect(out).toContain("/api/image-proxy?url=");
  });
});
