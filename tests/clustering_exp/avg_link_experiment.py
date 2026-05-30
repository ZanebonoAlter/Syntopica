"""
Average-link greedy clustering experiment for semantic board upgrade.

Compares:
1. Centroid + Pass2 (current Go implementation)
2. Average-link greedy (proposed replacement)

Reads real candidate embeddings from PostgreSQL.
"""

import os
import sys
import json
import numpy as np
import psycopg

# ─── DB config ───────────────────────────────────────────────────────
DB_DSN = os.getenv(
    "DATABASE_URL",
    "host=localhost port=5432 dbname=syntopica user=postgres password=postgres",
)

THRESHOLD = 0.35
REF_COUNT_MIN = 5

# ─── Data loading ────────────────────────────────────────────────────

QUERY = """
SELECT sl.id, sl.label, sl.slug, sl.ref_count, sl.embedding::text
FROM semantic_labels sl
WHERE sl.label_type = 'auxiliary'
  AND sl.status = 'active'
  AND sl.ref_count >= %s
  AND sl.embedding IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM board_composition bc
      WHERE bc.auxiliary_label_id = sl.id
  )
ORDER BY sl.id ASC
"""


def parse_pg_vector(vec_str: str) -> np.ndarray:
    """Parse '[0.1,0.2,...]' string to numpy array."""
    return np.array([float(x) for x in vec_str.strip("[]").split(",")], dtype=np.float64)


def load_candidates():
    with psycopg.connect(DB_DSN) as conn:
        with conn.cursor() as cur:
            cur.execute(QUERY, (REF_COUNT_MIN,))
            rows = cur.fetchall()
    candidates = []
    for rid, label, slug, ref_count, emb_str in rows:
        emb = parse_pg_vector(emb_str)
        candidates.append({"id": rid, "label": label, "slug": slug, "ref_count": ref_count, "embedding": emb})
    return candidates


# ─── Distance ────────────────────────────────────────────────────────

def cosine_distance(a: np.ndarray, b: np.ndarray) -> float:
    sim = np.dot(a, b) / (np.linalg.norm(a) * np.linalg.norm(b) + 1e-10)
    return 1.0 - sim


# ─── Strategy 1: Centroid + Pass2 (current) ──────────────────────────

def cluster_centroid_pass2(candidates, threshold):
    embs = np.array([c["embedding"] for c in candidates])

    # Pass 1: greedy with running-mean centroid
    cluster_members = []  # list of list of indices
    centroids = []

    for idx in range(len(candidates)):
        emb = embs[idx]
        matched = -1
        for ci, cent in enumerate(centroids):
            if cosine_distance(emb, cent) <= threshold:
                matched = ci
                break
        if matched >= 0:
            cluster_members[matched].append(idx)
            n = len(cluster_members[matched])
            centroids[matched] = (centroids[matched] * (n - 1) + emb) / n
        else:
            cluster_members.append([idx])
            centroids.append(emb.copy())

    if len(cluster_members) <= 1:
        return cluster_members

    # Compute stable centroids
    stable_centroids = []
    for members in cluster_members:
        stable_centroids.append(embs[members].mean(axis=0))

    # Pass 2: reassign
    new_clusters = {}  # orig_idx -> list of candidate indices
    for idx in range(len(candidates)):
        emb = embs[idx]
        best_ci = -1
        best_dist = threshold + 1
        for ci, cent in enumerate(stable_centroids):
            d = cosine_distance(emb, cent)
            if d <= threshold and d < best_dist:
                best_dist = d
                best_ci = ci
        if best_ci >= 0:
            new_clusters.setdefault(best_ci, []).append(idx)
        else:
            new_clusters[id(idx), idx] = [idx]  # unique key for solo

    return list(new_clusters.values())


# ─── Strategy 2: Average-link greedy (proposed) ──────────────────────

def cluster_avg_link_greedy(candidates, threshold, connectivity_threshold=None):
    """
    Average-link greedy clustering.

    For each candidate:
    1. Must have at least one neighbor within connectivity_threshold in the cluster
    2. Average pairwise distance to all cluster members <= threshold
    3. If multiple clusters qualify, pick the one with lowest average distance
    """
    if connectivity_threshold is None:
        connectivity_threshold = threshold

    embs = np.array([c["embedding"] for c in candidates])
    cluster_members = []  # list of list of indices

    for idx in range(len(candidates)):
        emb = embs[idx]
        best_ci = -1
        best_avg = threshold + 1

        for ci, members in enumerate(cluster_members):
            # Compute pairwise distances to all existing members
            dists = np.array([cosine_distance(emb, embs[m]) for m in members])

            # Condition 1: at least one neighbor within connectivity threshold
            min_dist = dists.min()
            if min_dist > connectivity_threshold:
                continue

            # Condition 2: average distance <= threshold
            avg_dist = dists.mean()
            if avg_dist > threshold:
                continue

            # Condition 3: pick lowest average
            if avg_dist < best_avg:
                best_avg = avg_dist
                best_ci = ci

        if best_ci >= 0:
            cluster_members[best_ci].append(idx)
        else:
            cluster_members.append([idx])

    return cluster_members


# ─── Analysis helpers ────────────────────────────────────────────────

def analyze_clusters(clusters, candidates, label=""):
    sizes = sorted([len(c) for c in clusters], reverse=True)
    singletons = sum(1 for s in sizes if s == 1)

    # Largest cluster pairwise quality
    largest = max(clusters, key=len)
    embs = np.array([candidates[i]["embedding"] for i in largest])
    if len(largest) > 1:
        # Compute pairwise distance matrix for largest cluster
        n = len(largest)
        pw_dists = []
        for i in range(n):
            for j in range(i + 1, n):
                pw_dists.append(cosine_distance(embs[i], embs[j]))
        pw_dists = np.array(pw_dists)
        pw_median = np.median(pw_dists)
        pw_mean = pw_dists.mean()
        pw_p90 = np.percentile(pw_dists, 90)
        pw_max = pw_dists.max()
        over_threshold = (pw_dists > THRESHOLD).mean()
    else:
        pw_median = pw_mean = pw_p90 = pw_max = over_threshold = 0

    print(f"\n{'='*60}")
    print(f"  {label}")
    print(f"{'='*60}")
    print(f"  Clusters: {len(clusters)}")
    print(f"  Max cluster size: {sizes[0] if sizes else 0}")
    print(f"  Top 10 sizes: {sizes[:10]}")
    print(f"  Singletons: {singletons}")
    print(f"  Largest cluster pairwise quality:")
    print(f"    median={pw_median:.3f}  mean={pw_mean:.3f}  p90={pw_p90:.3f}  max={pw_max:.3f}")
    print(f"    pairs > threshold: {over_threshold:.1%}")

    # Show top 5 largest cluster labels
    sorted_clusters = sorted(clusters, key=len, reverse=True)
    for rank, cl in enumerate(sorted_clusters[:5]):
        labels = [candidates[i]["label"] for i in cl[:20]]
        print(f"\n  Cluster #{rank+1} (size={len(cl)}):")
        print(f"    {', '.join(labels)}")


def compute_pairwise_histogram(clusters, candidates, label=""):
    """Compute pairwise distance stats across ALL clusters."""
    all_pw = []
    for cl in clusters:
        if len(cl) < 2:
            continue
        embs = np.array([candidates[i]["embedding"] for i in cl])
        n = len(cl)
        for i in range(n):
            for j in range(i + 1, n):
                all_pw.append(cosine_distance(embs[i], embs[j]))
    all_pw = np.array(all_pw)
    if len(all_pw) == 0:
        return
    print(f"\n  {label} — global pairwise stats (within clusters):")
    print(f"    median={np.median(all_pw):.3f}  mean={all_pw.mean():.3f}  "
          f"p90={np.percentile(all_pw, 90):.3f}  max={all_pw.max():.3f}")
    print(f"    pairs > threshold: {(all_pw > THRESHOLD).mean():.1%}")


# ─── Main ────────────────────────────────────────────────────────────

def main():
    print("Loading candidates from PostgreSQL...")
    candidates = load_candidates()
    print(f"Loaded {len(candidates)} candidates")

    if not candidates:
        print("No candidates found. Check DB connection and data.")
        sys.exit(1)

    # ─── Run strategies ──────────────────────────────────────────────

    clusters_cp2 = cluster_centroid_pass2(candidates, THRESHOLD)
    analyze_clusters(clusters_cp2, candidates, "Centroid + Pass2 (current)")

    compute_pairwise_histogram(clusters_cp2, candidates, "Centroid + Pass2")

    clusters_al = cluster_avg_link_greedy(candidates, THRESHOLD)
    analyze_clusters(clusters_al, candidates, f"Average-link greedy (th={THRESHOLD})")

    compute_pairwise_histogram(clusters_al, candidates, "Average-link greedy")

    # ─── Try a few threshold variations ──────────────────────────────
    for th in [0.30, 0.32, 0.35, 0.38, 0.40]:
        clusters_al_th = cluster_avg_link_greedy(candidates, th)
        sizes = sorted([len(c) for c in clusters_al_th], reverse=True)
        singles = sum(1 for s in sizes if s == 1)
        print(f"  avg-link th={th}: clusters={len(clusters_al_th)}, "
              f"max={sizes[0]}, top5={sizes[:5]}, singletons={singles}")

    # ─── Connectivity vs avg threshold split ─────────────────────────
    print("\n\n--- Connectivity threshold split experiment ---")
    for conn_th, avg_th in [(0.35, 0.35), (0.35, 0.40), (0.35, 0.45), (0.40, 0.40), (0.40, 0.45)]:
        clusters_split = cluster_avg_link_greedy(candidates, avg_th, connectivity_threshold=conn_th)
        sizes = sorted([len(c) for c in clusters_split], reverse=True)
        singles = sum(1 for s in sizes if s == 1)
        print(f"  conn={conn_th} avg={avg_th}: clusters={len(clusters_split)}, "
              f"max={sizes[0]}, top5={sizes[:5]}, singletons={singles}")


if __name__ == "__main__":
    main()
