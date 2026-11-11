// Command fetch pre-seeds the embedded-postgres binary cache.
//
// 为什么需要它：embedded-postgres v2 的 DownloadProvider 用 2 分钟超时的
// http.Client 直连 GitHub release；本机经本地代理下载 ~35MB 的 PG 包常超时。
// 本工具做「可续传 + SHA-256 校验」的下载，并按库的缓存布局落盘
// （<cache>/<name>-<sha16>/<name>/ + .complete 标记），让库直接命中缓存。
//
// 用法：
//
//	fetch pg          # PG 18.6.1 aarch64-linux-gnu
//	fetch supervisor  # v2.0.1 supervisor 二进制
package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	pgName   = "postgresql-18.6.1-aarch64-unknown-linux-gnu"
	pgURL    = "https://github.com/theseus-rs/postgresql-binaries/releases/download/18.6.1/" + pgName + ".tar.gz"
	pgSHA256 = "1ab47fa2f06ff70454309ad02e5d89c46488300d43977231481fd6b160b9d556"
	supVer   = "v2.0.1"
	supName  = "embedded-postgres_linux_arm64"
	supBase  = "https://github.com/fergusstrange/embedded-postgres/releases/download/" + supVer
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: fetch <pg|supervisor>")
		os.Exit(2)
	}
	cache := os.Getenv("LAB_CACHE")
	if cache == "" {
		cache = filepath.Join(".", "_tmp", "cache")
	}
	if err := os.MkdirAll(cache, 0o700); err != nil {
		fail(err)
	}
	switch os.Args[1] {
	case "pg":
		fetchPG(cache)
	case "supervisor":
		fetchSupervisor(cache)
	default:
		fmt.Fprintln(os.Stderr, "unknown target")
		os.Exit(2)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "ERROR:", err)
	os.Exit(1)
}

// downloadWithResume 下载到 out；支持 Range 续传；每 2s 打印进度。
func downloadWithResume(url, out string) error {
	part := out + ".part"
	var offset int64
	if st, err := os.Stat(part); err == nil {
		offset = st.Size()
	}
	req, _ := http.NewRequest("GET", url, nil)
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	client := &http.Client{Timeout: 30 * time.Minute}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == 200 && offset > 0 {
		offset = 0 // 服务端忽略 Range，从头再来
	}
	if res.StatusCode != 200 && res.StatusCode != 206 {
		return fmt.Errorf("GET %s: HTTP %d", url, res.StatusCode)
	}
	f, err := os.OpenFile(part, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	start := time.Now()
	last := start
	done := offset
	buf := make([]byte, 256*1024)
	for {
		n, rerr := res.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				return err
			}
			done += int64(n)
		}
		if time.Since(last) > 2*time.Second {
			el := time.Since(start).Seconds()
			fmt.Fprintf(os.Stderr, "  %.1f MiB  %.1f KiB/s\n", float64(done)/1048576, float64(done)/1024/el)
			last = time.Now()
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	return os.Rename(part, out)
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func fetchPG(cache string) {
	tgz := filepath.Join(cache, pgName+".tar.gz")
	if _, err := os.Stat(tgz); err != nil {
		fmt.Fprintln(os.Stderr, "downloading PG bundle ...")
		if err := downloadWithResume(pgURL, tgz); err != nil {
			fail(err)
		}
	}
	sum, err := sha256File(tgz)
	if err != nil {
		fail(err)
	}
	if sum != pgSHA256 {
		fail(fmt.Errorf("sha256 mismatch: got %s want %s", sum, pgSHA256))
	}
	fmt.Fprintf(os.Stderr, "sha256 ok (%d bytes)\n", fileSize(tgz))

	entry := filepath.Join(cache, pgName+"-"+pgSHA256[:16])
	target := filepath.Join(entry, pgName)
	if _, err := os.Stat(filepath.Join(entry, ".complete")); err == nil {
		fmt.Println("cache entry already complete:", entry)
		return
	}
	if err := os.RemoveAll(target); err != nil {
		fail(err)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		fail(err)
	}
	fmt.Fprintln(os.Stderr, "extracting ...")
	if err := extractTarGz(tgz, target); err != nil {
		fail(err)
	}
	if err := os.WriteFile(filepath.Join(entry, ".complete"), []byte(pgSHA256), 0o600); err != nil {
		fail(err)
	}
	fmt.Println("pg cache entry:", entry)
}

func extractTarGz(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(hdr.Name)
		if strings.HasPrefix(name, "..") {
			return fmt.Errorf("unsafe path in tar: %s", hdr.Name)
		}
		// 剥掉 tar 里的顶层目录（postgresql-<ver>-<target>/），
		// 让内容直接落在 <entry>/<name>/ 下——与库的缓存布局一致。
		if i := strings.Index(name, string(os.PathSeparator)); i >= 0 {
			name = name[i+1:]
		} else if name == pgName {
			continue
		}
		if name == "" || name == "." {
			continue
		}
		out := filepath.Join(dst, name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(out, 0o755); err != nil {
				return err
			}
		case tar.TypeSymlink:
			os.MkdirAll(filepath.Dir(out), 0o755)
			os.Remove(out)
			if err := os.Symlink(hdr.Linkname, out); err != nil {
				return err
			}
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(out), 0o755)
			w, err := os.OpenFile(out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode).Perm())
			if err != nil {
				return err
			}
			if _, err := io.Copy(w, tr); err != nil {
				w.Close()
				return err
			}
			w.Close()
		}
	}
}

func fetchSupervisor(cache string) {
	cks := filepath.Join(cache, supVer+"-checksums.txt")
	if _, err := os.Stat(cks); err != nil {
		if err := downloadWithResume(supBase+"/checksums.txt", cks); err != nil {
			fail(err)
		}
	}
	b, err := os.ReadFile(cks)
	if err != nil {
		fail(err)
	}
	digest := ""
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == supName {
			digest = fields[0]
		}
	}
	if digest == "" {
		fail(fmt.Errorf("supervisor checksum not found in %s", cks))
	}
	bin := filepath.Join(cache, supVer+"-"+supName)
	if _, err := os.Stat(bin); err == nil {
		fmt.Println("supervisor already cached:", bin)
		return
	}
	tmp := filepath.Join(cache, supName+".part")
	if err := downloadWithResume(supBase+"/"+supName, tmp); err != nil {
		fail(err)
	}
	sum, err := sha256File(tmp)
	if err != nil {
		fail(err)
	}
	if sum != digest {
		fail(fmt.Errorf("supervisor sha256 mismatch: got %s want %s", sum, digest))
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		fail(err)
	}
	if err := os.Rename(tmp, bin); err != nil {
		fail(err)
	}
	fmt.Println("supervisor cached:", bin)
}

func fileSize(p string) int64 {
	st, err := os.Stat(p)
	if err != nil {
		return 0
	}
	return st.Size()
}
