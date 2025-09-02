#!/usr/bin/env python3
import argparse, json, math, sys
from pathlib import Path

import matplotlib.pyplot as plt
import networkx as nx
from matplotlib.cm import get_cmap
from matplotlib.patches import Patch

def load_graph(json_path: Path):
    with open(json_path, "r", encoding="utf-8") as f:
        data = json.load(f)

    # Accept either {"graph_state": {...}} or top-level {"nodes":..., "edges":...}
    gs = data.get("graph_state", data)

    nodes = gs.get("nodes", [])
    edges = gs.get("edges", [])

    # Normalize minimal fields we need
    norm_nodes = []
    for n in nodes:
        norm_nodes.append({
            "id": n.get("id") or n.get("ID"),
            "name": n.get("name") or n.get("Name") or n.get("id"),
            "layer": int(n.get("layer", 0)),
            "kind": n.get("kind", "unknown"),
        })

    norm_edges = []
    for e in edges:
        norm_edges.append({
            "id": e.get("id"),
            "source": e.get("source"),
            "target": e.get("target"),
            "type": e.get("type", "depends_on"),
        })
    return norm_nodes, norm_edges

def layered_layout(nodes):
    """
    Simple deterministic layered layout:
    - y = -layer
    - x = centered indices within each layer
    """
    layers = {}
    for n in nodes:
        layers.setdefault(n["layer"], []).append(n["id"])

    pos = {}
    ygap, xgap = 2.2, 2.2
    for layer in sorted(layers.keys()):
        ids = sorted(layers[layer])  # stable order
        n = len(ids)
        # center around 0: positions = [-k, ..., 0, ..., +k]
        for i, nid in enumerate(ids):
            x = (i - (n - 1) / 2.0) * xgap
            y = -layer * ygap
            pos[nid] = (x, y)
    return pos

def build_graph(nodes, edges):
    G = nx.DiGraph()
    for n in nodes:
        G.add_node(n["id"], **n)
    for e in edges:
        if e["source"] in G.nodes and e["target"] in G.nodes:
            G.add_edge(e["source"], e["target"], **e)
    return G

def draw_graph(G, out_path: Path, figsize=(14, 9), with_labels=True, font_size=8):
    # Colors by layer using a discrete colormap
    node_layers = {n: G.nodes[n].get("layer", 0) for n in G.nodes}
    unique_layers = sorted(set(node_layers.values()))
    cmap = get_cmap("tab20")
    color_map = {lay: cmap(i % cmap.N) for i, lay in enumerate(unique_layers)}

    pos = layered_layout([G.nodes[n] for n in G.nodes])

    # Draw nodes per layer for color legend
    plt.figure(figsize=figsize)
    for lay in unique_layers:
        layer_nodes = [n for n, l in node_layers.items() if l == lay]
        nx.draw_networkx_nodes(
            G, pos,
            nodelist=layer_nodes,
            node_color=[color_map[lay]],
            node_size=650,
            linewidths=0.8,
            edgecolors="black",
        )

    # Edge style by type (optional, simple styles)
    style_map = {
        "depends_on": "solid",
        "invokes": "solid",
        "exchanges": (0, (4, 2)),
        "persists": (0, (2, 2)),
        "configures": (0, (6, 1, 1, 1)),  # dash-dot-ish
        "observes": (0, (1, 1)),
        "contains": "solid",
    }
    edge_types = sorted(set(d.get("type", "depends_on") for _, _, d in G.edges(data=True)))
    for et in edge_types:
        es = [(u, v) for u, v, d in G.edges(data=True) if d.get("type") == et]
        if not es:
            continue
        nx.draw_networkx_edges(
            G, pos,
            edgelist=es,
            arrows=True,
            arrowstyle="-|>",
            width=1.2,
            alpha=0.8,
            connectionstyle="arc3,rad=0.08",
            style=style_map.get(et, "solid"),
        )

    if with_labels:
        labels = {n: _short_label(G.nodes[n]) for n in G.nodes}
        nx.draw_networkx_labels(G, pos, labels=labels, font_size=font_size)

    # Build legends
    layer_patches = [Patch(color=color_map[lay], label=f"Layer {lay}") for lay in unique_layers]
    if layer_patches:
        plt.legend(
            handles=layer_patches,
            loc="upper left",
            bbox_to_anchor=(1.01, 1.0),
            title="Layers",
            frameon=False,
        )

    plt.axis("off")
    plt.tight_layout()
    out_path.parent.mkdir(parents=True, exist_ok=True)
    plt.savefig(out_path, dpi=160, bbox_inches="tight")
    plt.close()

def _short_label(nattr):
    """Prefer name; fall back to id. Truncate to keep the figure tidy."""
    label = nattr.get("name") or nattr.get("id") or ""
    if len(label) > 26:
        return label[:23] + "…"
    return label

def main():
    ap = argparse.ArgumentParser(description="Visualize graph_state from p5.json")
    ap.add_argument("--in", dest="inp", required=True, help="Path to p5.json or graph.json")
    ap.add_argument("--out", dest="outp", default="graph.png", help="Output image path (PNG)")
    ap.add_argument("--w", type=float, default=14.0, help="Figure width in inches")
    ap.add_argument("--h", type=float, default=9.0, help="Figure height in inches")
    ap.add_argument("--font", type=int, default=8, help="Label font size")
    args = ap.parse_args()

    nodes, edges = load_graph(Path(args.inp))
    if not nodes or not edges:
        print("No nodes or edges found in input JSON.", file=sys.stderr)
        sys.exit(2)

    G = build_graph(nodes, edges)
    draw_graph(G, Path(args.outp), figsize=(args.w, args.h), font_size=args.font)
    print(f"Saved: {args.outp}  (nodes={len(nodes)}, edges={len(edges)})")

if __name__ == "__main__":
    main()
