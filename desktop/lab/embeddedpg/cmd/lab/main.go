// Command lab drives an embedded PostgreSQL instance for the iteration-0
// real-load verification (iter0-findings.md 0-7).
//
// Subcommands:
//
//	provision   下载 PG 18.6.1 二进制（embedded-postgres 的 DownloadProvider）
//	            并复制出「安装目录」（供 LocalProvider 使用 + 塞 pgvector）
//	start       起一个实例（LocalProvider + 持久 DataDir），打印 DSN 后驻留
//	bench       冷启动计时：initdb/复用 两档，各 N 轮
//
// 所有路径由 LAB_ROOT 派生（默认 <repo>/desktop/lab/embeddedpg/_tmp）。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres/v2"
)

const pgVersion = embeddedpostgres.V18 // 18.6.1

type labEnv struct {
	Root     string
	CacheDir string
	Install  string
	DataDir  string
	WorkDir  string
	Port     uint32
	DB       string
	User     string
	Pass     string
}

func env() labEnv {
	root := os.Getenv("LAB_ROOT")
	if root == "" {
		// 默认：可执行文件所在目录（构建产物落在 _tmp/ 里）。
		if exe, err := os.Executable(); err == nil {
			root = filepath.Dir(exe)
		} else {
			root = "."
		}
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	port := uint32(55433)
	if v := os.Getenv("LAB_PORT"); v != "" {
		if p, err := strconv.ParseUint(v, 10, 32); err == nil {
			port = uint32(p)
		}
	}
	return labEnv{
		Root:     abs,
		CacheDir: filepath.Join(abs, "cache"),
		Install:  filepath.Join(abs, "install", "postgresql-"+string(pgVersion)),
		DataDir:  filepath.Join(abs, "pgdata"),
		WorkDir:  filepath.Join(abs, "work"),
		Port:     port,
		DB:       envOr("LAB_DB", "syntopica"),
		User:     envOr("LAB_USER", "postgres"),
		Pass:     envOr("LAB_PASS", "postgres"),
	}
}

func envOr(k, d string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return d
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "provision":
		err = cmdProvision()
	case "start":
		err = cmdStart()
	case "bench":
		err = cmdBench()
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: lab <provision|start|bench>")
}

// ---------- provision ----------

func cmdProvision() error {
	e := env()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	fmt.Printf("provision: cache=%s\n", e.CacheDir)
	t0 := time.Now()
	dp := embeddedpostgres.DownloadProvider{CacheDir: e.CacheDir}
	inst, err := dp.Acquire(ctx, embeddedpostgres.BinaryRequest{Version: pgVersion, GOOS: "linux", GOARCH: "arm64"})
	if err != nil {
		return fmt.Errorf("acquire binaries: %w", err)
	}
	fmt.Printf("provision: acquired %s in %s\n", inst.Dir, time.Since(t0).Truncate(time.Millisecond))

	// 复制成「安装目录」：应用自有的运行时资产（pgvector 三件套也放这里）。
	marker := filepath.Join(e.Install, ".provisioned")
	if _, err := os.Stat(marker); err == nil {
		fmt.Printf("provision: install dir already provisioned: %s\n", e.Install)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(e.Install), 0o755); err != nil {
		return err
	}
	if err := copyTree(inst.Dir, e.Install); err != nil {
		return fmt.Errorf("copy install tree: %w", err)
	}
	if err := os.WriteFile(marker, []byte("ok\n"), 0o644); err != nil {
		return err
	}
	fmt.Printf("provision: install dir ready: %s\n", e.Install)
	return nil
}

func copyTree(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		switch {
		case info.IsDir():
			return os.MkdirAll(target, 0o755)
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		default:
			in, err := os.Open(p)
			if err != nil {
				return err
			}
			defer in.Close()
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
			if err != nil {
				return err
			}
			defer out.Close()
			_, err = out.ReadFrom(in)
			return err
		}
	})
}

// ---------- start ----------

type startResult struct {
	ConnectionURL string `json:"connection_url"`
	Port          uint32 `json:"port"`
	DataDir       string `json:"data_dir"`
	WorkDir       string `json:"work_dir"`
	BinariesDir   string `json:"binaries_dir"`
	StartMs       int64  `json:"start_ms"`
	ProbeMs       int64  `json:"probe_ms"`
	TotalMs       int64  `json:"total_ms"`
	Version       string `json:"version"`
}

func cmdStart() error {
	e := env()
	res, ep, err := startOnce(e, true)
	if err != nil {
		return err
	}
	out, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(out))
	if !holdRequested() {
		return ep.Stop()
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	return ep.Stop()
}

func holdRequested() bool {
	for _, a := range os.Args[2:] {
		if a == "--hold" {
			return true
		}
	}
	return os.Getenv("LAB_HOLD") == "1"
}

// startParams：LAB_START_PARAMS="k=v,k=v" → StartParameters。
// 用于验证「PG 日志落目录」「pg_stat_statements」等桌面端要用的运行时参数。
func (e labEnv) startParams() map[string]string {
	out := map[string]string{}
	raw := strings.TrimSpace(os.Getenv("LAB_START_PARAMS"))
	if raw == "" {
		return out
	}
	for _, kv := range strings.Split(raw, ",") {
		kv = strings.TrimSpace(kv)
		if kv == "" {
			continue
		}
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out
}

// startOnce 起一次实例：LocalProvider + 持久 DataDir + Ready hook 建 vector 扩展。
// probe=true 时用安装目录里的 psql 做一次真实外部连接探测（B1/B3 都要这个语义）。
func startOnce(e labEnv, probe bool) (*startResult, *embeddedpostgres.EmbeddedPostgres, error) {
	if err := os.MkdirAll(e.WorkDir, 0o755); err != nil {
		return nil, nil, err
	}
	cfg := embeddedpostgres.DefaultConfig().
		Version(pgVersion).
		Port(e.Port).
		Database(e.DB).
		Username(e.User).
		Password(e.Pass).
		Storage(embeddedpostgres.Storage{
			CacheDir: e.CacheDir,
			WorkDir:  e.WorkDir,
			DataDir:  e.DataDir,
		}).
		Provider(embeddedpostgres.LocalProvider(e.Install)).
		StartParameters(e.startParams()).
		Hooks(embeddedpostgres.Hooks{
			Ready: []embeddedpostgres.Hook{func(ctx context.Context, info embeddedpostgres.InstanceInfo) (func(context.Context) error, error) {
				// Ready hook：实例已能接受认证查询——此刻建扩展（pgvector）。
				if _, err := runPsql(e, info.ConnectionURL, "CREATE EXTENSION IF NOT EXISTS vector;"); err != nil {
					return nil, fmt.Errorf("create extension vector: %w", err)
				}
				return nil, nil
			}},
		})

	t0 := time.Now()
	ep := embeddedpostgres.NewDatabase(cfg)
	if err := ep.Start(); err != nil {
		return nil, nil, err
	}
	startMs := time.Since(t0).Milliseconds()
	info := ep.Info()
	res := &startResult{
		ConnectionURL: info.ConnectionURL,
		Port:          info.Port,
		DataDir:       info.DataDir,
		WorkDir:       info.WorkDir,
		BinariesDir:   info.BinariesDir,
		StartMs:       startMs,
	}
	if probe {
		t1 := time.Now()
		if _, err := runPsql(e, info.ConnectionURL, "SELECT 1;"); err != nil {
			_ = ep.Stop()
			return nil, nil, fmt.Errorf("psql probe: %w", err)
		}
		res.ProbeMs = time.Since(t1).Milliseconds()
	}
	res.TotalMs = time.Since(t0).Milliseconds()
	return res, ep, nil
}

// runPsql 用安装目录里的 psql（外部客户端，非进程内驱动）跑一条 SQL。
func runPsql(e labEnv, dsn, sql string) (string, error) {
	psql := filepath.Join(e.Install, "bin", "psql")
	cmd := exec.Command(psql, dsn, "-X", "-v", "ON_ERROR_STOP=1", "-Atc", sql)
	cmd.Env = append(os.Environ(), "PGCONNECT_TIMEOUT=5")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// ---------- bench ----------

func cmdBench() error {
	e := env()
	runs := 3
	if v := os.Getenv("LAB_RUNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			runs = n
		}
	}
	mode := envOr("LAB_BENCH_MODE", "warm") // warm | fresh
	type row struct {
		Mode    string `json:"mode"`
		Run     int    `json:"run"`
		StartMs int64  `json:"start_ms"`
		ProbeMs int64  `json:"probe_ms"`
		TotalMs int64  `json:"total_ms"`
	}
	var rows []row
	for i := 1; i <= runs; i++ {
		if mode == "fresh" {
			if err := os.RemoveAll(e.DataDir); err != nil {
				return err
			}
		}
		res, ep, err := startOnce(e, true)
		if err != nil {
			return fmt.Errorf("run %d: %w", i, err)
		}
		rows = append(rows, row{mode, i, res.StartMs, res.ProbeMs, res.TotalMs})
		if err := ep.Stop(); err != nil {
			return err
		}
	}
	out, _ := json.MarshalIndent(rows, "", "  ")
	fmt.Println(string(out))
	starts := make([]int64, 0, len(rows))
	totals := make([]int64, 0, len(rows))
	for _, r := range rows {
		starts = append(starts, r.StartMs)
		totals = append(totals, r.TotalMs)
	}
	fmt.Printf("summary: mode=%s runs=%d start_median_ms=%d total_median_ms=%d\n",
		mode, runs, median(starts), median(totals))
	return nil
}

func median(v []int64) int64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]int64(nil), v...)
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
	return s[len(s)/2]
}
