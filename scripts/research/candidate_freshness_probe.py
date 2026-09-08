# -*- coding: utf-8 -*-
"""调研脚本：升级建议候选池「新鲜度规则」效果对比（方案 A / 规则 2）。

背景（2026-09-07 报障追踪）：7 天窗口圈进 8 月老泛化标签（欧元区/德意志银行/
默茨…），把"德国政治与选举"新标签簇稀释成大杂烩，LLM 全裁 skip；今日窗口
反而干净出建议。本脚本不动代码，只对比两种候选池规则 × 两种窗口下的：
  1) 池子大小 / 聚类后簇形态
  2) 德国相关标签各自落到哪个簇、簇成员是谁
以此验证「窗口内集中活跃」（规则 fresh）能否保住干净主题簇。

规则定义（复刻 backend-go internal/tagmanagement/service/board）：
  current: ref_count>=5 + active auxiliary + 未挂载任何版块 + embedding 非空
           + 窗口内 EXISTS 关联文章（一篇就算，现行 CollectCandidates 逻辑）
  fresh:   current 基础上 + 窗口内 DISTINCT 文章数 >= 2
           且 >= ref_count * 0.3（窗口内集中活跃，挡老标签零星刷存在感）
聚类复刻：clusterAverageLink——按 id ASC 贪心并入"平均余弦距离最近且
  平均与最近单边均 <= 0.35"的簇（candidateFitsClusterAverageLink）。

用法：uv run --with psycopg2-binary scripts/research/candidate_freshness_probe.py
"""
import math
from collections import defaultdict

import psycopg2

DSN = dict(host="localhost", port=5432, user="postgres", password="postgres", dbname="syntopica")
THRESHOLD = 0.35  # ClusterDistanceThreshold 默认
MIN_WINDOW_ARTICLES = 2
REF_RATIO = 0.3
# 德国主题相关标签（label 含德国 + 已知混簇成员），用于追踪簇形态
GERMANY_KEYWORD = "德国"
GERMANY_IDS = {141666, 149369, 159790, 184080, 184087}  # 欧元区/德意志银行/默茨/德国选择党/基民盟

BASE_SQL = """
SELECT l.id, l.label, l.ref_count, l.embedding::text AS emb,
       COALESCE(w.cnt, 0) AS window_articles
FROM semantic_labels l
LEFT JOIN LATERAL (
  SELECT COUNT(DISTINCT a.id) AS cnt
  FROM article_topic_tags att
  JOIN topic_tag_semantic_labels ttsl ON ttsl.topic_tag_id = att.topic_tag_id
  JOIN articles a ON a.id = att.article_id
  WHERE ttsl.semantic_label_id = l.id AND a.created_at >= now() - (%(days)s || ' days')::interval
) w ON true
WHERE l.label_type = 'auxiliary' AND l.status = 'active'
  AND l.ref_count >= 5 AND l.embedding IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM board_composition b WHERE b.auxiliary_label_id = l.id)
  AND w.cnt >= 1
ORDER BY l.id ASC
"""


def parse_pgvector(text):
    return [float(x) for x in text.strip("[]").split(",")]


def cosine_distance(a, b):
    dot = sum(x * y for x, y in zip(a, b))
    na = math.sqrt(sum(x * x for x in a))
    nb = math.sqrt(sum(x * x for x in b))
    if na == 0 or nb == 0:
        return 1.0
    return 1.0 - dot / (na * nb)


def cluster_average_link(items, threshold=THRESHOLD):
    """复刻 Go clusterAverageLink：id ASC 顺序贪心（SQL 已按 id ASC 返回）。"""
    clusters = []  # each: list[dict]
    for it in items:
        best_idx, best_avg = -1, threshold + 1.0
        for i, cl in enumerate(clusters):
            dists = [cosine_distance(it["emb"], m["emb"]) for m in cl]
            avg = sum(dists) / len(dists)
            has_connected = any(d <= threshold for d in dists)
            if has_connected and avg <= threshold and avg < best_avg:
                best_avg, best_idx = avg, i
        if best_idx >= 0:
            clusters[best_idx].append(it)
        else:
            clusters.append([it])
    return clusters


def run(days, rule):
    with psycopg2.connect(**DSN) as conn:
        with conn.cursor() as cur:
            cur.execute(BASE_SQL, {"days": days})
            rows = cur.fetchall()
    items = [
        dict(id=r[0], label=r[1], ref=r[2], emb=parse_pgvector(r[3]), win=r[4])
        for r in rows
    ]
    if rule == "fresh":
        items = [x for x in items if x["win"] >= MIN_WINDOW_ARTICLES and x["win"] >= x["ref"] * REF_RATIO]
    clusters = cluster_average_link(items)
    multi = [c for c in clusters if len(c) > 1]
    return items, clusters, multi


def germany_report(clusters):
    out = []
    for ci, cl in enumerate(clusters):
        hit = [m for m in cl if GERMANY_KEYWORD in m["label"] or m["id"] in GERMANY_IDS]
        if not hit:
            continue
        members = "、".join(f"{m['label']}(win={m['win']}/{m['ref']})" for m in cl)
        out.append(f"    簇#{ci}(n={len(cl)}): {members}")
    return out


def sweep_threshold(days, rule, thresholds):
    """阈值敏感性：同一池子在不同距离阈值下的簇形态（看贪心 average-link 的传递混簇）。"""
    items, _, _ = run_pool(days, rule)
    tag = "今日" if days == 1 else "7天"
    for th in thresholds:
        clusters = cluster_average_link(items, threshold=th)
        print(f"== [{tag} | {rule} | 阈值 {th}] {len(items)} 标签 -> {len(clusters)} 簇")
        for ci, cl in enumerate(clusters):
            hit = [m for m in cl if GERMANY_KEYWORD in m["label"] or m["id"] in GERMANY_IDS or m["label"] in ("默茨",)]
            if hit:
                members = "、".join(m["label"] for m in cl)
                print(f"    德国主题簇#{ci}(n={len(cl)}): {members}")
        print()


def run_pool(days, rule):
    with psycopg2.connect(**DSN) as conn:
        with conn.cursor() as cur:
            cur.execute(BASE_SQL, {"days": days})
            rows = cur.fetchall()
    items = [
        dict(id=r[0], label=r[1], ref=r[2], emb=parse_pgvector(r[3]), win=r[4])
        for r in rows
    ]
    if rule == "fresh":
        items = [x for x in items if x["win"] >= MIN_WINDOW_ARTICLES and x["win"] >= x["ref"] * REF_RATIO]
    return items, None, None


def main():
    print(f"规则 fresh = 窗口内文章 >= {MIN_WINDOW_ARTICLES} 且 >= ref_count*{REF_RATIO}\n")
    summary = []
    for days in (1, 7):
        for rule in ("current", "fresh"):
            items, clusters, multi = run(days, rule)
            tag = "今日" if days == 1 else "7天"
            summary.append((tag, rule, len(items), len(clusters), len(multi)))
            print(f"== [{tag} | {rule}] 池子 {len(items)} 标签 -> {len(clusters)} 簇（多标签簇 {len(multi)}）")
            if rule == "fresh":
                dropped = sum(1 for d in (1, 7) if False)  # placeholder
            for line in germany_report(clusters):
                print(line)
            # 最大 3 个簇摘要
            for cl in sorted(clusters, key=len, reverse=True)[:3]:
                if len(cl) > 1:
                    print(f"    最大簇示例(n={len(cl)}): " + "、".join(m["label"] for m in cl[:8]) + ("…" if len(cl) > 8 else ""))
            print()

    # 德国相关标签的进出明细
    print("== 德国相关标签在两规则下的池内进出（7 天窗口）")
    with psycopg2.connect(**DSN) as conn, conn.cursor() as cur:
        cur.execute(BASE_SQL, {"days": 7})
        rows = {r[0]: r for r in cur.fetchall()}
    for lid in sorted(GERMANY_IDS | {i for i, r in rows.items() if GERMANY_KEYWORD in r[1]}):
        if lid not in rows:
            continue
        _, label, ref, _, win = rows[lid]
        keep = win >= MIN_WINDOW_ARTICLES and win >= ref * REF_RATIO
        print(f"    {label}: 窗口 {win}/{ref} -> {'保留' if keep else '被挡'}")


if __name__ == "__main__":
    import sys
    if "--sweep" in sys.argv:
        for days in (1, 7):
            for rule in ("current", "fresh"):
                sweep_threshold(days, rule, (0.35, 0.30, 0.25))
    else:
        main()
