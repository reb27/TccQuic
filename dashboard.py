import csv
import glob
import json
import os
from collections import defaultdict


def load_csv(path):
    with open(path, newline="", encoding="utf-8") as f:
        return list(csv.DictReader(f))


def _find_summary_csv(run_dir):
    """Aceita statistics-summary.csv ou statistics-summary-<pid>.csv (do test_client)."""
    p = os.path.join(run_dir, "statistics-summary.csv")
    if os.path.exists(p):
        return p
    candidates = glob.glob(os.path.join(run_dir, "statistics-summary-*.csv"))
    return candidates[0] if candidates else None


def discover_runs(base_dir):
    """
    Descobre execuções WFQ: subpastas cujo nome contenha 'wfq' e tenham statistics-summary + wfq_utilization.csv.
    Foco exclusivo em WFQ dinâmico com dados 100% dos testes.
    """
    runs = []
    if not os.path.isdir(base_dir):
        return runs
    for entry in os.scandir(base_dir):
        if not entry.is_dir():
            continue
        if "wfq" not in entry.name.lower():
            continue
        run_dir = entry.path
        summary = _find_summary_csv(run_dir)
        wfq_util = os.path.join(run_dir, "wfq_utilization.csv")
        if not summary or not os.path.exists(wfq_util):
            continue
        stats_candidates = glob.glob(os.path.join(run_dir, "statistics-*.csv"))
        stats = next((p for p in stats_candidates if "summary" not in p), None)
        runs.append(
            {
                "id": entry.name,
                "dir": run_dir,
                "statistics_summary": summary,
                "statistics": stats if stats and os.path.exists(stats) else None,
                "wfq_utilization": wfq_util,
            }
        )
    return runs


# Cenários de rede e FoV (compatível com implementation_plan e RUN_AND_VALIDATE)
NETWORK_SCENARIOS = {
    "net1": {
        "label": "Rede #1",
        "loss_pct": 10,
        "delay_ms": 24,
        "description": "Rede lenta, pouco ruído",
    },
    "net6": {
        "label": "Rede #6",
        "loss_pct": 30,
        "delay_ms": 10,
        "description": "Rede rápida, muito ruído",
    },
}
FOV_SCENARIOS = {
    "narrow": {"label": "FoV Estreito", "pct_high": "~13%", "pct_low": "~87%", "tiles": "~10/frame"},
    "normal": {"label": "FoV Normal", "pct_high": "~38%", "pct_low": "~62%", "tiles": "~30/frame"},
    "wide": {"label": "FoV Largo", "pct_high": "~77%", "pct_low": "~23%", "tiles": "~60/frame"},
}
# Apenas WFQ dinâmico: foco em como a adaptação reage à rede e ao FoV.
POLICY_LABELS = {"wfq_dyn": "WFQ (dinâmico)"}


def _parse_run_id(run_id):
    """Extrai rede e FoV do nome da pasta (ex: wfq_net1_narrow, wfq_net6_wide)."""
    parts = run_id.replace("-", "_").lower().split("_")
    policy = "wfq_dyn"
    network = next((p for p in parts if p in ("net1", "net6") or (p.startswith("net") and len(p) <= 5)), "")
    if network == "net":
        network = ""
    fov = next((p for p in parts if p in ("narrow", "normal", "wide")), "")
    if not fov:
        fov = "normal"
    return policy, network, fov


def build_dataset(base_dir):
    """
    Constrói o dataset com runs e descrições dos cenários de rede e FoV.
    """
    data = {
        "runs": {},
        "network_scenarios": NETWORK_SCENARIOS,
        "fov_scenarios": FOV_SCENARIOS,
        "policy_labels": POLICY_LABELS,
    }
    runs = discover_runs(base_dir)
    # Ordenar: rede e FoV para facilitar a leitura do dashboard.
    def run_sort_key(r):
        _, net, fov = _parse_run_id(r["id"])
        return (net or "z", fov or "normal")
    runs = sorted(runs, key=run_sort_key)
    for run in runs:
        run_id = run["id"]
        summary_rows = load_csv(run["statistics_summary"])
        wfq_rows = load_csv(run["wfq_utilization"]) if run["wfq_utilization"] else []

        policy, network, fov = _parse_run_id(run_id)
        meta = {
            "id": run_id,
            "policy": policy,
            "network": network,
            "fov": fov or "normal",
        }
        data["runs"][run_id] = {
            "meta": meta,
            "summary": summary_rows,
            "wfq_utilization": wfq_rows,
        }
    return data


HTML_TEMPLATE = """<!DOCTYPE html>
<html lang="pt-BR">
<head>
  <meta charset="utf-8" />
  <title>WFQ dinâmico — Rede e FoV</title>
  <style>
    * {{ box-sizing: border-box; }}
    body {{ font-family: system-ui, -apple-system, "Segoe UI", sans-serif; margin: 0; padding: 0; background: #0f172a; color: #e5e7eb; }}
    .page {{ max-width: 960px; margin: 0 auto; padding: 16px 20px 32px; }}
    header {{ padding: 14px 0; border-bottom: 1px solid #1e293b; margin-bottom: 20px; }}
    h1 {{ font-size: 20px; margin: 0; font-weight: 600; }}
    .subtitle {{ font-size: 13px; color: #94a3b8; margin-top: 4px; }}

    .panel-context {{ background: #1e293b; border: 1px solid #334155; border-radius: 8px; padding: 14px 18px; margin-bottom: 24px; }}
    .panel-context h2 {{ font-size: 12px; color: #94a3b8; text-transform: uppercase; letter-spacing: 0.06em; margin: 0 0 10px; }}
    .context-grid {{ display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 16px; }}
    .context-block {{ }}
    .context-block .label {{ font-size: 11px; color: #64748b; text-transform: uppercase; margin-bottom: 2px; }}
    .context-block .value {{ font-size: 15px; font-weight: 600; color: #e5e7eb; }}
    .context-block .detail {{ font-size: 12px; color: #94a3b8; margin-top: 2px; }}

    .controls {{ display: flex; flex-wrap: wrap; gap: 12px; align-items: flex-end; margin-bottom: 20px; }}
    .control-group {{ }}
    .control-group label {{ display: block; font-size: 11px; color: #64748b; margin-bottom: 4px; text-transform: uppercase; }}
    select {{ background: #1e293b; color: #e5e7eb; border: 1px solid #334155; border-radius: 6px; padding: 8px 12px; font-size: 14px; min-width: 160px; }}

    .section {{ margin-bottom: 28px; }}
    .section-title {{ font-size: 13px; color: #94a3b8; font-weight: 600; margin-bottom: 6px; text-transform: uppercase; letter-spacing: 0.05em; }}
    .section-desc {{ font-size: 12px; color: #64748b; margin-bottom: 10px; line-height: 1.45; }}
    .chart-wrap {{ background: #0f172a; border: 1px solid #1e293b; border-radius: 8px; padding: 16px; }}
    canvas {{ display: block; }}
    .axis-label {{ font-size: 11px; color: #64748b; margin-top: 4px; }}
    .legend {{ display: flex; gap: 20px; margin-top: 10px; font-size: 12px; flex-wrap: wrap; }}
    .legend-item {{ display: inline-flex; align-items: center; gap: 8px; }}
    .legend-item .swatch {{ width: 14px; height: 4px; border-radius: 2px; }}

    .summary-grid {{ display: grid; grid-template-columns: repeat(3, 1fr); gap: 12px; margin-top: 12px; }}
    .summary-box {{ background: #1e293b; border-radius: 8px; padding: 14px; border: 1px solid #334155; }}
    .summary-box h4 {{ margin: 0 0 10px; font-size: 11px; color: #94a3b8; text-transform: uppercase; }}
    .summary-box .row {{ display: flex; justify-content: space-between; font-size: 12px; margin: 5px 0; }}
    .summary-box .row .val {{ color: #e5e7eb; font-weight: 500; }}

    .no-data {{ color: #64748b; font-size: 13px; padding: 24px; text-align: center; }}
    details {{ margin-top: 20px; border: 1px solid #1e293b; border-radius: 8px; overflow: hidden; }}
    details summary {{ padding: 10px 14px; background: #1e293b; cursor: pointer; font-size: 12px; color: #94a3b8; }}
    details table {{ width: 100%; border-collapse: collapse; font-size: 11px; }}
    details th, details td {{ padding: 6px 8px; text-align: left; border-bottom: 1px solid #1e293b; }}
    details th {{ color: #64748b; font-weight: 500; }}
  </style>
</head>
<body>
  <div class="page">
    <header>
      <h1>WFQ dinâmico — adaptação por rede e FoV</h1>
      <div class="subtitle">Dados 100% dos testes. O foco é observar como os pesos do WFQ dinâmico reagem aos cenários de rede (#1 e #6) e aos FoVs narrow e wide.</div>
    </header>

    <div class="panel-context" id="panel-context">
      <h2>Cenário atual</h2>
      <div class="context-grid" id="context-grid"></div>
    </div>

    <div class="controls">
      <div class="control-group">
        <label>Execução</label>
        <select id="sel-run"></select>
      </div>
    </div>

    <div class="section">
      <div class="section-title">1. Vazão por canal (share %) ao longo do tempo</div>
      <div class="section-desc">Cada faixa é a fração de bytes enviados por uma classe naquela janela. As três somam 100%. Assim você vê como a demanda (LOW / MED / HIGH) muda com o tempo.</div>
      <div class="chart-wrap">
        <canvas id="chart-share" width="880" height="240"></canvas>
      </div>
      <div class="axis-label">Eixo X: janela de tempo (1 s) — Eixo Y: share (0–100%)</div>
      <div class="legend" id="legend-share"></div>
    </div>

    <div class="section">
      <div class="section-title">2. Pesos WFQ ao longo do tempo</div>
      <div class="section-desc">Pesos W_low, W_med, W_high que o algoritmo aplica. O recálculo segue a vazão observada (dentro de ε_min e ε_max).</div>
      <div class="chart-wrap">
        <canvas id="chart-weights" width="880" height="240"></canvas>
      </div>
      <div class="axis-label">Eixo X: janela — Eixo Y: peso W</div>
      <div class="legend" id="legend-weights"></div>
    </div>

    <div class="section">
      <div class="section-title">3. Última janela: share %, peso W e bytes/peso</div>
      <div class="section-desc">Use esta seção para ver, no fim do teste, como a distribuição ficou entre LOW, MED e HIGH e quais pesos o algoritmo estava aplicando.</div>
      <div class="summary-grid" id="summary-boxes"></div>
    </div>

    <details>
      <summary>Resumo da execução (statistics-summary) e tabela bruta</summary>
      <div style="padding:12px;overflow-x:auto;">
        <table id="summary-table"></table>
      </div>
    </details>
  </div>

  <script>
    const DATA = {data_json};
    const COLORS = {{ low: '#22c55e', medium: '#eab308', high: '#f97316' }};
    const NETS = DATA.network_scenarios || {{}};
    const FOVS = DATA.fov_scenarios || {{}};
    const POLS = DATA.policy_labels || {{}};

    function networkFromRunId(id) {{
      const s = (id || '').toLowerCase();
      if (s.includes('net6')) return 'net6';
      if (s.includes('net1')) return 'net1';
      return '';
    }}

    function fovFromRunId(id) {{
      const s = (id || '').toLowerCase();
      if (s.includes('narrow')) return 'narrow';
      if (s.includes('wide')) return 'wide';
      return 'normal';
    }}

    function policyFromRunId(id) {{
      return 'wfq_dyn';
    }}

    function runLabel(id) {{
      const m = DATA.runs[id] && DATA.runs[id].meta;
      const netKey = networkFromRunId(id) || (m && m.network) || '';
      const fovKey = fovFromRunId(id) || (m && m.fov) || 'normal';
      const polKey = 'wfq_dyn';
      const netLabel = (NETS[netKey] && NETS[netKey].label) || netKey || '—';
      const fovLabel = (FOVS[fovKey] && FOVS[fovKey].label) || fovKey;
      const polLabel = POLS[polKey] || polKey;
      return `${{netLabel}} · ${{fovLabel}}`;
    }}

    function buildSelects() {{
      const runIds = Object.keys(DATA.runs);
      const selRun = document.getElementById('sel-run');
      if (!runIds.length) {{
        document.getElementById('panel-context').innerHTML = '<h2>Nenhum run WFQ encontrado</h2><p>Cada pasta deve ter <code>statistics-summary*.csv</code> e <code>wfq_utilization.csv</code>. Rode <code>./run_full_test_and_dashboard.sh &lt;IP&gt;</code> e gere o dashboard com <code>--base-dir logs/server_scheduler_test</code>.</p>';
        selRun.innerHTML = '<option value="">—</option>';
        return;
      }}
      selRun.innerHTML = runIds.map(id => `<option value="${{id}}">${{runLabel(id)}}</option>`).join('');
      selRun.addEventListener('change', () => {{ const id = selRun.value; if (id) updateUI(id); }});
      updateUI(runIds[0]);
    }}

    function updateContextPanel(run) {{
      const m = run.meta;
      const id = m.id || '';
      const netKey = networkFromRunId(id) || m.network || '';
      const fovKey = fovFromRunId(id) || m.fov || 'normal';
      const policyKey = 'wfq_dyn';
      const net = NETS[netKey] || {{}};
      const fov = FOVS[fovKey] || {{}};
      const policyLabel = POLS[policyKey] || policyKey;
      const policyDetail = 'Pesos dinâmicos (Algoritmo 1) — reagem ao share observado em cada janela';
      document.getElementById('context-grid').innerHTML = `
        <div class="context-block">
          <div class="label">Run</div>
          <div class="value">${{id}}</div>
        </div>
        <div class="context-block">
          <div class="label">Rede</div>
          <div class="value">${{net.label || netKey || '—'}}</div>
          <div class="detail">${{net.description || ''}} ${{net.loss_pct != null ? 'Loss ' + net.loss_pct + '%' : ''}} ${{net.delay_ms != null ? 'Delay ' + net.delay_ms + ' ms' : ''}}</div>
        </div>
        <div class="context-block">
          <div class="label">FoV</div>
          <div class="value">${{fov.label || fovKey}}</div>
          <div class="detail">${{fov.pct_high || ''}} HIGH · ${{fov.pct_low || ''}} LOW</div>
        </div>
        <div class="context-block">
          <div class="label">Política</div>
          <div class="value">${{policyLabel}}</div>
          <div class="detail">${{policyDetail}}</div>
        </div>
      `;
    }}

    function normShare(row) {{
      const keys = ['share_low', 'share_medium', 'share_high'];
      let a = parseFloat(row[keys[0]] || '0') || 0, b = parseFloat(row[keys[1]] || '0') || 0, c = parseFloat(row[keys[2]] || '0') || 0;
      const sum = a + b + c;
      if (sum <= 0) return [0, 0, 0];
      if (sum > 1.5) {{ a /= 100; b /= 100; c /= 100; }}
      const s = a + b + c;
      if (s <= 0) return [0, 0, 0];
      return [a/s, b/s, c/s];
    }}

    function drawStackedShare(canvasId, rows) {{
      const canvas = document.getElementById(canvasId);
      if (!canvas) return;
      const ctx = canvas.getContext('2d');
      const W = canvas.width, H = canvas.height;
      ctx.clearRect(0, 0, W, H);
      if (!rows.length) {{ ctx.fillStyle = '#64748b'; ctx.font = '13px sans-serif'; ctx.fillText('Sem wfq_utilization.csv', 20, 24); return; }}
      const pad = {{ left: 44, right: 20, top: 16, bottom: 32 }};
      const plotW = W - pad.left - pad.right, plotH = H - pad.top - pad.bottom;
      const n = rows.length;
      const xs = rows.map((_, i) => pad.left + (i / Math.max(1, n - 1)) * plotW);
      const yScale = v => pad.top + plotH - v * plotH;
      ctx.strokeStyle = '#334155';
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(pad.left, pad.top);
      ctx.lineTo(pad.left, H - pad.bottom);
      ctx.lineTo(W - pad.right, H - pad.bottom);
      ctx.stroke();
      ctx.fillStyle = '#64748b';
      ctx.font = '11px sans-serif';
      ctx.fillText('100%', 6, pad.top + 4);
      ctx.fillText('50%', 6, pad.top + plotH/2 + 4);
      ctx.fillText('0%', 6, H - pad.bottom + 4);
      const colors = [COLORS.low, COLORS.medium, COLORS.high];
      for (let band = 0; band < 3; band++) {{
        ctx.fillStyle = colors[band];
        ctx.beginPath();
        for (let i = 0; i < n; i++) {{
          const [s0, s1, s2] = normShare(rows[i]);
          let bot = 0;
          for (let b = 0; b < band; b++) bot += [s0,s1,s2][b];
          const top = bot + [s0,s1,s2][band];
          const yTop = yScale(top);
          const yBot = yScale(bot);
          if (i === 0) {{ ctx.moveTo(pad.left, yBot); ctx.lineTo(pad.left, yTop); }}
          ctx.lineTo(xs[i], yTop);
        }}
        const [s0l, s1l, s2l] = normShare(rows[n-1]);
        let botLast = 0;
        for (let b = 0; b < band; b++) botLast += [s0l,s1l,s2l][b];
        ctx.lineTo(xs[n-1], yScale(botLast));
        for (let i = n - 1; i >= 0; i--) {{
          const [s0, s1, s2] = normShare(rows[i]);
          let s = 0;
          for (let b = 0; b < band; b++) s += [s0,s1,s2][b];
          ctx.lineTo(xs[i], yScale(s));
        }}
        const [s00, s01, s02] = normShare(rows[0]);
        let s0 = 0;
        for (let b = 0; b < band; b++) s0 += [s00,s01,s02][b];
        ctx.lineTo(pad.left, yScale(s0));
        ctx.closePath();
        ctx.fill();
        ctx.strokeStyle = 'rgba(0,0,0,0.2)';
        ctx.lineWidth = 1;
        ctx.stroke();
        ctx.strokeStyle = '#334155';
      }}
    }}

    function weightValue(row, rawKey, normKey) {{
      const raw = parseFloat(row[rawKey] || '');
      if (!isNaN(raw) && raw > 0) return raw;
      const norm = parseFloat(row[normKey] || '');
      if (!isNaN(norm) && norm > 0) return norm * 6.0;
      return 0;
    }}

    function drawWeightsChart(canvasId, rows) {{
      const canvas = document.getElementById(canvasId);
      if (!canvas) return;
      const ctx = canvas.getContext('2d');
      const W = canvas.width, H = canvas.height;
      ctx.clearRect(0, 0, W, H);
      if (!rows.length) {{ ctx.fillStyle = '#64748b'; ctx.font = '13px sans-serif'; ctx.fillText('Sem wfq_utilization.csv', 20, 24); return; }}
      const pad = {{ left: 44, right: 20, top: 16, bottom: 32 }};
      const plotW = W - pad.left - pad.right, plotH = H - pad.top - pad.bottom;
      const series = [
        {{ raw: 'raw_w_low', norm: 'w_low', color: COLORS.low }},
        {{ raw: 'raw_w_medium', norm: 'w_medium', color: COLORS.medium }},
        {{ raw: 'raw_w_high', norm: 'w_high', color: COLORS.high }},
      ];
      let minV = Infinity, maxV = -Infinity;
      series.forEach(s => {{ rows.forEach(r => {{ const v = weightValue(r, s.raw, s.norm); minV = Math.min(minV, v); maxV = Math.max(maxV, v); }}); }});
      if (maxV <= minV) maxV = minV + 1;
      const range = maxV - minV;
      const yScale = v => pad.top + plotH - ((v - minV) / range) * plotH;
      const n = rows.length;
      const xs = rows.map((_, i) => pad.left + (i / Math.max(1, n - 1)) * plotW);
      ctx.strokeStyle = '#334155';
      ctx.lineWidth = 1;
      for (let t = 0; t <= 4; t++) {{
        const y = pad.top + plotH - (t/4) * plotH;
        ctx.setLineDash([2, 4]);
        ctx.beginPath();
        ctx.moveTo(pad.left, y);
        ctx.lineTo(W - pad.right, y);
        ctx.stroke();
      }}
      ctx.setLineDash([]);
      ctx.beginPath();
      ctx.moveTo(pad.left, pad.top);
      ctx.lineTo(pad.left, H - pad.bottom);
      ctx.lineTo(W - pad.right, H - pad.bottom);
      ctx.stroke();
      ctx.fillStyle = '#64748b';
      ctx.font = '11px sans-serif';
      ctx.fillText(maxV.toFixed(1), 6, pad.top + 4);
      ctx.fillText(minV.toFixed(1), 6, H - pad.bottom + 4);
      series.forEach(s => {{
        ctx.strokeStyle = s.color;
        ctx.lineWidth = 2.5;
        ctx.beginPath();
        rows.forEach((r, i) => {{
          const v = weightValue(r, s.raw, s.norm);
          const y = yScale(v);
          if (i === 0) ctx.moveTo(xs[i], y); else ctx.lineTo(xs[i], y);
        }});
        ctx.stroke();
      }});
    }}

    function updateShareChart(run) {{
      const rows = run.wfq_utilization || [];
      drawStackedShare('chart-share', rows);
      document.getElementById('legend-share').innerHTML = [
        {{ key: 'share_low', color: COLORS.low, label: 'LOW (vazão)' }},
        {{ key: 'share_medium', color: COLORS.medium, label: 'MED (vazão)' }},
        {{ key: 'share_high', color: COLORS.high, label: 'HIGH (vazão)' }},
      ].map(s => `<span class="legend-item"><i class="swatch" style="background:${{s.color}}"></i>${{s.label}}</span>`).join('');
    }}

    function updateWeightsChart(run) {{
      const rows = run.wfq_utilization || [];
      drawWeightsChart('chart-weights', rows);
      document.getElementById('legend-weights').innerHTML = [
        {{ color: COLORS.low, label: 'W LOW' }},
        {{ color: COLORS.medium, label: 'W MED' }},
        {{ color: COLORS.high, label: 'W HIGH' }},
      ].map(s => `<span class="legend-item"><i class="swatch" style="background:${{s.color}}"></i>${{s.label}}</span>`).join('');
    }}

    function updateSummaryBoxes(run) {{
      const container = document.getElementById('summary-boxes');
      const rows = run.wfq_utilization || [];
      if (!rows.length) {{ container.innerHTML = '<div class="no-data">Sem wfq_utilization.csv</div>'; return; }}
      const last = rows[rows.length - 1];
      const [shareLow, shareMed, shareHigh] = normShare(last);
      const classes = [
        {{ label: 'LOW', ratio: 'ratio_low', raw: 'raw_w_low', share: shareLow }},
        {{ label: 'MED', ratio: 'ratio_medium', raw: 'raw_w_medium', share: shareMed }},
        {{ label: 'HIGH', ratio: 'ratio_high', raw: 'raw_w_high', share: shareHigh }},
      ];
      function fmtRatio(v) {{
        const n = parseFloat(v) || 0;
        if (n >= 1e6) return (n/1e6).toFixed(1) + 'M';
        if (n >= 1e3) return (n/1e3).toFixed(1) + 'k';
        return n.toFixed(0);
      }}
      container.innerHTML = classes.map(c => {{
        const ratio = parseFloat(last[c.ratio] || '0');
        const w = weightValue(last, c.raw, c.raw.replace('raw_', ''));
        const sharePct = (c.share * 100).toFixed(1);
        return `<div class="summary-box"><h4>${{c.label}}</h4><div class="row"><span>Share</span><span class="val">${{sharePct}}%</span></div><div class="row"><span>Peso W</span><span class="val">${{w.toFixed(2)}}</span></div><div class="row"><span>bytes/peso</span><span class="val">${{fmtRatio(ratio)}}</span></div></div>`;
      }}).join('');
    }}

    function updateSummaryTable(run) {{
      const table = document.getElementById('summary-table');
      table.innerHTML = '';
      const rows = run.summary || [];
      if (!rows.length) {{ table.innerHTML = '<p class="no-data">Sem statistics-summary</p>'; return; }}
      const thead = document.createElement('thead');
      thead.innerHTML = '<tr>' + Object.keys(rows[0]).map(k => `<th>${{k}}</th>`).join('') + '</tr>';
      table.appendChild(thead);
      const tbody = document.createElement('tbody');
      rows.forEach(r => {{ tbody.innerHTML += '<tr>' + Object.values(r).map(v => `<td>${{v}}</td>`).join('') + '</tr>'; }});
      table.appendChild(tbody);
    }}

    function updateUI(runId) {{
      const run = DATA.runs[runId];
      if (!run) return;
      updateContextPanel(run);
      updateShareChart(run);
      updateWeightsChart(run);
      updateSummaryBoxes(run);
      updateSummaryTable(run);
    }}

    document.addEventListener('DOMContentLoaded', buildSelects);
  </script>
</body>
</html>
"""


def generate_dashboard(base_dir, output_path):
    dataset = build_dataset(base_dir)
    html = HTML_TEMPLATE.format(data_json=json.dumps(dataset))
    with open(output_path, "w", encoding="utf-8") as f:
        f.write(html)
    print(f"dashboard written to {output_path}")


if __name__ == "__main__":
    import argparse

    parser = argparse.ArgumentParser(
        description="Generate WFQ dynamic dashboard.html from CSV logs."
    )
    parser.add_argument(
        "--base-dir",
        default="logs",
        help="Directory containing one subdirectory per run (with statistics-summary.csv, wfq_utilization.csv, ...).",
    )
    parser.add_argument(
        "--output",
        default="dashboard.html",
        help="Output HTML file path.",
    )
    args = parser.parse_args()
    generate_dashboard(args.base_dir, args.output)

