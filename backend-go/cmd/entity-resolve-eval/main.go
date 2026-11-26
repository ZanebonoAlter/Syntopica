package main

// entity-resolve-eval — CLI entry. Subcommands:
//
//	plan      — mechanically derive the 4-line budget from a sample file
//	freeze    — freeze the protocol (samples/snapshot/groups/models/budget/scoring)
//	worksheet — export the blind-annotation worksheet + disputed-item count
//	run       — bounded real run: REQUIRES a frozen protocol AND an explicit
//	            authorization file (BUDGET-APPROVED). Without authorization it
//	            refuses and never touches the database; with authorization it
//	            assembles config → DB → NewRunner → Run → records + budget
//	            artifacts (assembly failures report their own reason)
//	score     — post-run join of gold labels with run records
//
// Gold labels are only ever touched by `score`.

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/config"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "plan":
		err = cmdPlan(os.Args[2:])
	case "freeze":
		err = cmdFreeze(os.Args[2:])
	case "worksheet":
		err = cmdWorksheet(os.Args[2:])
	case "run":
		err = cmdRun(os.Args[2:])
	case "score":
		err = cmdScore(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "entity-resolve-eval: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `用法：entity-resolve-eval <plan|freeze|worksheet|run|score> [flags]

  plan      -samples <file>                      机械列示四项预算
  freeze    -samples <file> -models <json> -scoring <id> -out <file>
                                                  冻结协议（运行前不可再改）
  worksheet -samples <file> -out <file.csv>      导出盲标工作表 + 争议项报数
  run       -protocol <file> -samples <file> -authorize <file> -session <id> -out <records.json>
                                                  有界真实运行（未授权直接拒绝；授权后连库→跑→落 records+预算）
  score     -gold <file> -records <file>         事后评分（金标只在此处出现）
`)
}

func cmdPlan(args []string) error {
	fs := flag.NewFlagSet("plan", flag.ExitOnError)
	samplesPath := fs.String("samples", "", "samples JSON")
	_ = fs.Parse(args) // flag.ExitOnError 已经处理解析失败
	if *samplesPath == "" {
		return fmt.Errorf("-samples 必填")
	}
	samples, err := loadSamplesFile(*samplesPath)
	if err != nil {
		return err
	}
	plan := MechanicalBudget(samples)
	out, _ := json.MarshalIndent(plan, "", "  ")
	fmt.Println(string(out))
	fmt.Printf("样本 %d 条（争议 %d 条不计预算）\n", len(samples), CountDisputed(samples))
	return nil
}

func cmdFreeze(args []string) error {
	fs := flag.NewFlagSet("freeze", flag.ExitOnError)
	samplesPath := fs.String("samples", "", "samples JSON")
	modelsJSON := fs.String("models", "{}", "role→model JSON, e.g. {\"chat\":\"...\",\"embed\":\"...\"}")
	scoringID := fs.String("scoring", "pairwise-v1", "scoring-rule fingerprint")
	outPath := fs.String("out", "", "output protocol JSON")
	_ = fs.Parse(args) // flag.ExitOnError 已经处理解析失败
	if *samplesPath == "" || *outPath == "" {
		return fmt.Errorf("-samples 与 -out 必填")
	}
	samples, err := loadSamplesFile(*samplesPath)
	if err != nil {
		return err
	}
	models := map[string]string{}
	if err := json.Unmarshal([]byte(*modelsJSON), &models); err != nil {
		return fmt.Errorf("-models 非法 JSON：%w", err)
	}
	snapshotDigest := SamplesDigest(samples) // snapshot = the frozen sample texts themselves
	p := Freeze(Protocol{
		SampleDigest:   snapshotDigest,
		SnapshotDigest: snapshotDigest,
		GroupRules:     distinctGroups(samples),
		ModelVersions:  models,
		Budget:         MechanicalBudget(samples),
		ScoringRules:   *scoringID,
		DisputedCount:  CountDisputed(samples),
	}, time.Now().UTC().Format(time.RFC3339))
	raw, _ := json.MarshalIndent(p, "", "  ")
	return os.WriteFile(*outPath, raw, 0o600)
}

func cmdWorksheet(args []string) error {
	fs := flag.NewFlagSet("worksheet", flag.ExitOnError)
	samplesPath := fs.String("samples", "", "samples JSON")
	outPath := fs.String("out", "", "output CSV")
	_ = fs.Parse(args) // flag.ExitOnError 已经处理解析失败
	if *samplesPath == "" || *outPath == "" {
		return fmt.Errorf("-samples 与 -out 必填")
	}
	samples, err := loadSamplesFile(*samplesPath)
	if err != nil {
		return err
	}
	f, err := os.Create(*outPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if err := ExportWorksheet(f, samples); err != nil {
		return err
	}
	fmt.Println(FormatDisputedReport(samples))
	return nil
}

func cmdRun(args []string) error {
	return cmdRunWith(args, defaultRunDeps())
}

// runOptions is one `run` invocation's inputs (frozen artifacts + run tag).
type runOptions struct {
	protocolPath  string
	samplesPath   string
	authorizePath string
	sessionID     string
	outPath       string
	configDir     string
}

// cmdRunWith is the testable entry: deps carries the assembly seams (config +
// DB connection + provider client) so tests can exercise every branch with
// ZERO network and ZERO real database.
//
// Two failure branches are deliberately distinct (review R-16):
//  1. no authorization → 「未授权」 and NO config load / DB connection at all;
//  2. authorized but assembly fails (config/DB/provider) → the concrete
//     assembly reason, NEVER the 「当前无授权」 sentence (that would contradict
//     the fact that authorization was already verified).
func cmdRunWith(args []string, deps runDeps) error {
	opts, err := parseRunOptions(args)
	if err != nil {
		return err
	}
	if err := verifyRunAuthorization(opts.authorizePath); err != nil {
		return err
	}
	if opts.sessionID == "" {
		return fmt.Errorf("-session 必填（run 标识：审计分组 + 产物溯源 + 清理依据）")
	}
	if opts.outPath == "" {
		return fmt.Errorf("-out 必填（records 产物路径：逐提及记录 + 错例清单落盘后才能评分）")
	}
	protocol, samples, err := loadFrozenRunInputs(opts)
	if err != nil {
		return err
	}
	return executeRun(context.Background(), opts, protocol, samples, deps)
}

// parseRunOptions reads the run flags. Missing -protocol/-samples/-authorize
// are refused BEFORE anything else happens.
func parseRunOptions(args []string) (runOptions, error) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	opts := runOptions{}
	fs.StringVar(&opts.protocolPath, "protocol", "", "frozen protocol JSON")
	fs.StringVar(&opts.samplesPath, "samples", "", "samples JSON")
	fs.StringVar(&opts.authorizePath, "authorize", "", "authorization file (REQUIRED — no budget granted without it)")
	fs.StringVar(&opts.sessionID, "session", "", "run/session id (audit grouping + provenance)")
	fs.StringVar(&opts.outPath, "out", "", "records artifact path (JSON array of MentionRecord)")
	fs.StringVar(&opts.configDir, "config", "./configs", "config directory (same paradigm as the other CLIs)")
	_ = fs.Parse(args) // flag.ExitOnError 已经处理解析失败
	if opts.protocolPath == "" || opts.samplesPath == "" {
		return opts, fmt.Errorf("-protocol 与 -samples 必填")
	}
	if opts.authorizePath == "" {
		return opts, fmt.Errorf("本轮无模型额度授权：run 需要显式 -authorize 文件才会启动（不静默消耗预算）")
	}
	return opts, nil
}

// verifyRunAuthorization is branch ①: the explicit budget authorization file.
// It runs BEFORE any config load / DB connection — an unauthorized run never
// touches the database.
func verifyRunAuthorization(path string) error {
	auth, err := os.ReadFile(path) //nolint:gosec // CLI 输入路径由运行者显式提供
	if err != nil {
		return fmt.Errorf("读取授权文件失败（未授权，不启动）：%w", err)
	}
	if !containsMarker(string(auth), "BUDGET-APPROVED") {
		return fmt.Errorf("授权文件缺少 BUDGET-APPROVED 标记：拒绝运行（未授权，不连库、不调用模型）")
	}
	return nil
}

// loadFrozenRunInputs loads the frozen protocol and the sample list and refuses
// any post-freeze drift (pure file work — no network, no database).
func loadFrozenRunInputs(opts runOptions) (Protocol, []EvalSample, error) {
	protocol, err := loadProtocolFile(opts.protocolPath)
	if err != nil {
		return Protocol{}, nil, err
	}
	if err := VerifyFrozen(protocol); err != nil {
		return Protocol{}, nil, err
	}
	samples, err := loadSamplesFile(opts.samplesPath)
	if err != nil {
		return Protocol{}, nil, err
	}
	if SamplesDigest(samples) != protocol.SampleDigest {
		return Protocol{}, nil, fmt.Errorf("样本清单与冻结协议不一致：冻结后不得改样本")
	}
	return protocol, samples, nil
}

// runDeps are the assembly seams of the authorized run path (tests inject a
// fake connector + fake provider client: zero network, zero real database).
type runDeps struct {
	connectDB func(configDir string) (*gorm.DB, error)
	provider  airouter.ProviderClient
}

func defaultRunDeps() runDeps {
	return runDeps{connectDB: connectEvalDB, provider: airouter.NewOpenAICompatibleClient()}
}

// connectEvalDB follows the existing CLI paradigm (backfill-stories /
// calibrate-heat): config.LoadConfig + gorm.Open straight from the DSN — no
// InitDB, so no implicit migrations/seed writes from an eval run.
func connectEvalDB(configDir string) (*gorm.DB, error) {
	if err := config.LoadConfig(configDir); err != nil {
		return nil, fmt.Errorf("加载配置失败（%s）：%w", configDir, err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: config.AppConfig.Database.DSN, PreferSimpleProtocol: true,
	}), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败（DSN 来自配置 %s，需先跑过迁移）：%w", configDir, err)
	}
	return db, nil
}

// executeRun is the full authorized assembly: connect → NewRunner → Run →
// persist records + budget actuals.
func executeRun(ctx context.Context, opts runOptions, protocol Protocol, samples []EvalSample, deps runDeps) error {
	if deps.connectDB == nil || deps.provider == nil {
		return fmt.Errorf("run 装配失败（授权已通过，装配未通过）：provider/数据库接缝未装配")
	}
	db, err := deps.connectDB(opts.configDir)
	if err != nil {
		return fmt.Errorf("run 装配失败（授权已通过，装配未通过）：%w", err)
	}
	runner, err := NewRunner(db, protocol, opts.sessionID, protocolAllowedModels(protocol), deps.provider)
	if err != nil {
		return fmt.Errorf("run 装配失败（授权已通过，装配未通过）：%w", err)
	}
	records, report, err := runner.Run(ctx, samples)
	if err != nil {
		return fmt.Errorf("run 执行失败：%w", err)
	}
	if err := writeRunArtifacts(opts.outPath, records, report); err != nil {
		return err
	}
	fmt.Printf("run 完成：records %d 条 → %s（预算报告 %s.budget.json）\n", len(records), opts.outPath, opts.outPath)
	fmt.Printf("预算实际：抽取 %d / 二判 %d / embed 文本 %d / HTTP %d（发送前拦截 %d）\n",
		report.ActualExtraction, report.ActualSecondRound, report.ActualEmbedTexts, report.ActualHTTP, report.Rejected)
	return nil
}

// protocolAllowedModels turns the FROZEN model list into the send-guard model
// whitelist (cross-model/fallback requests are rejected pre-send). An empty
// frozen list means count-only mode (no whitelist) — documented, not silent.
func protocolAllowedModels(protocol Protocol) []string {
	allowed := make([]string, 0, len(protocol.ModelVersions))
	for _, model := range protocol.ModelVersions {
		if model != "" {
			allowed = append(allowed, model)
		}
	}
	return allowed
}

// writeRunArtifacts persists the per-mention records (the score subcommand's
// input shape) plus the budget actuals next to them. Both files are the
// recomputable artifact set: error examples (attempts/reason) ride on the
// records.
func writeRunArtifacts(recordsPath string, records []MentionRecord, report BudgetReport) error {
	raw, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 records：%w", err)
	}
	if err := os.WriteFile(recordsPath, raw, 0o600); err != nil {
		return fmt.Errorf("写 records 产物（%s）：%w", recordsPath, err)
	}
	budgetRaw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化预算报告：%w", err)
	}
	budgetPath := recordsPath + ".budget.json"
	if err := os.WriteFile(budgetPath, budgetRaw, 0o600); err != nil {
		return fmt.Errorf("写预算报告（%s）：%w", budgetPath, err)
	}
	return nil
}

func cmdScore(args []string) error {
	fs := flag.NewFlagSet("score", flag.ExitOnError)
	goldPath := fs.String("gold", "", "gold labels JSON (blind-annotated BEFORE the run)")
	recordsPath := fs.String("records", "", "run records JSON")
	_ = fs.Parse(args) // flag.ExitOnError 已经处理解析失败
	if *goldPath == "" || *recordsPath == "" {
		return fmt.Errorf("-gold 与 -records 必填")
	}
	goldRaw, err := os.ReadFile(*goldPath)
	if err != nil {
		return err
	}
	var gold []GoldEntry
	if err := json.Unmarshal(goldRaw, &gold); err != nil {
		return fmt.Errorf("parse gold: %w", err)
	}
	recRaw, err := os.ReadFile(*recordsPath)
	if err != nil {
		return err
	}
	var records []MentionRecord
	if err := json.Unmarshal(recRaw, &records); err != nil {
		return fmt.Errorf("parse records: %w", err)
	}
	report := Score(gold, records)
	out, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(out))
	if report.Blocked {
		return fmt.Errorf("评分被阻断：%s", report.BlockReason)
	}
	return nil
}

func distinctGroups(samples []EvalSample) []string {
	seen := map[string]bool{}
	var groups []string
	for _, s := range samples {
		if !seen[s.Group] {
			seen[s.Group] = true
			groups = append(groups, s.Group)
		}
	}
	return groups
}

func containsMarker(s, marker string) bool {
	return len(s) >= len(marker) && (s == marker || indexOf(s, marker) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
