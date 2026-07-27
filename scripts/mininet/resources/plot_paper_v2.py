#!/usr/bin/env python3
"""
Figuras para o artigo/TCC no estilo de papers de sistemas (SIGCOMM/MMSys/INFOCOM).

Cadeia causal do professor: Wp -> Sp -> Dp -> DDRp -> TMRp -> FHRp -> QoEp
Foco: SERVIDOR (fig 1-5). Cliente entra em fig 6-7 para fechar a cadeia.

    Fig 1 — Alocação temporal por classe (stacked area, 1Hz)
    Fig 2 — WFQ intenção × observado (scatter + diagonal)
    Fig 3 — ECDF/CCDF do queue delay por classe × política
    Fig 4 — Delay -> Drop (hexbin + curva logística)
    Fig 5 — Pareto: proteção FoV × Jain entre classes
    Fig 6 — TMR por cliente (violin/strip): variabilidade entre usuários
    Fig 7 — QoE decomposta em componentes (waterfall)
    Fig 8 — Heatmap conditional (política × regime)

Uso:
    python plot_paper_v2.py <MATRIX_DIR> [<MATRIX_DIR2> ...] [-o <out_dir>]

Aceita múltiplos diretórios (ex.: matrix-006 + matrix-007) e agrega as reps
automaticamente. Barras de erro/bandas mostram média±desvio-padrão entre reps.
"""

import os, glob, csv, argparse, math
from collections import defaultdict

import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
import matplotlib.patches as mpatches
import matplotlib.colors as mcolors
import numpy as np


# ─── Estilo (padrão de paper) ────────────────────────────────────────────────

PAPER_RC = {
    'font.family': 'serif',
    'font.serif': ['Times New Roman', 'DejaVu Serif', 'serif'],
    'font.size': 10, 'axes.labelsize': 10, 'axes.titlesize': 11,
    'xtick.labelsize': 9, 'ytick.labelsize': 9, 'legend.fontsize': 9,
    'legend.framealpha': 0.92, 'legend.edgecolor': '#cccccc',
    'axes.spines.top': False, 'axes.spines.right': False,
    'axes.linewidth': 0.8, 'grid.linewidth': 0.5, 'grid.alpha': 0.35,
    'lines.linewidth': 1.6, 'lines.markersize': 6,
    'figure.dpi': 150, 'savefig.dpi': 300,
    'savefig.bbox': 'tight', 'savefig.pad_inches': 0.06,
}

# Colorblind-safe (Wong 2011) — classes semânticas
CL_FOV, CL_NEAR, CL_BG = '#D55E00', '#E69F00', '#0072B2'
CLASS_ORDER = ['high', 'medium', 'low']
CLASS_LABEL = {'high': 'FoV', 'medium': 'Vizinhança', 'low': 'Fundo'}
CLASS_COLOR = {'high': CL_FOV, 'medium': CL_NEAR, 'low': CL_BG}
PRIO_TO_CLASS = {0: 'low', 1: 'medium', 2: 'high'}

# Políticas
POLICIES = ['FIFO', 'SP', 'WFQ']
PAL_POL  = {'FIFO': '#555555', 'SP': '#0072B2', 'WFQ': '#D55E00'}
LS_POL   = {'FIFO': '-', 'SP': '--', 'WFQ': ':'}
MK_POL   = {'FIFO': 'o', 'SP': 's', 'WFQ': '^'}

# ABR — o "legacy" do código é o algoritmo baseado em vazão descrito pelo
# professor na seção 14.4.2 (Throughput-Based). Nomes de exibição:
ABR_LABEL = {'BOLA': 'BOLA', 'LEGACY': 'Throughput'}

# Cenários — padronizados
SCENARIOS = ['1', '2', '5', '6']
SC_LABEL  = {'1': 'Sc1 (24 ms, 10%)', '2': 'Sc2 (24 ms, 30%)',
             '5': 'Sc5 (10 ms, 10%)', '6': 'Sc6 (10 ms, 30%)'}
SC_SHORT  = {'1': 'Sc1', '2': 'Sc2', '5': 'Sc5', '6': 'Sc6'}
SC_MK     = {'1': 'o', '2': 's', '5': '^', '6': 'D'}


# ─── Carregadores ────────────────────────────────────────────────────────────

def read_env(env_path):
    d = {}
    with open(env_path) as f:
        for line in f:
            if '=' in line and not line.startswith('#'):
                k, v = line.strip().split('=', 1)
                d[k.strip()] = v.strip()
    return d


def index_runs(matrix_dirs):
    """
    Retorna lista de dicts com metadata + path por run.
    Aceita 1+ matrix dirs (ex.: matrix-006 + matrix-007).
    """
    if isinstance(matrix_dirs, str):
        matrix_dirs = [matrix_dirs]
    idx = []
    for matrix_dir in matrix_dirs:
        for ep in glob.glob(os.path.join(matrix_dir, '**', 'experiment.env'), recursive=True):
            d = read_env(ep)
            sc  = d.get('scenario_id', '')
            pol = d.get('policy', '').upper()
            abr = d.get('abr_mode', d.get('abr', '')).upper()
            nc  = int(d.get('num_clients', '1'))
            mix = d.get('fov_mix', 'balanced')
            rep = int(d.get('rep', '1') or 1)
            if sc not in SCENARIOS or pol not in POLICIES:
                continue
            run_dir = os.path.dirname(ep)
            idx.append({
                'sc': sc, 'pol': pol, 'abr': abr, 'nc': nc, 'mix': mix,
                'rep': rep, 'dir': run_dir,
            })
    return idx


def match(idx, **kv):
    return [r for r in idx if all(r.get(k) == v for k, v in kv.items())]


# --- Server: last row per class in class_agg.csv --------------------------

def last_per_class(ca_path):
    last = {}
    try:
        with open(ca_path, newline='') as f:
            for row in csv.DictReader(f):
                if row.get('class'):
                    last[row['class']] = row
    except Exception:
        pass
    return last


def server_metrics(run):
    """Retorna dict com métricas server-side agregadas por classe."""
    d = last_per_class(os.path.join(run['dir'], 'class_agg.csv'))
    out = {}
    for c in CLASS_ORDER:
        r = d.get(c, {})
        completed = float(r.get('completed', 0) or 0)
        drops     = float(r.get('dropped_deadline', 0) or 0)
        tot = completed + drops
        out[c] = {
            'bytes_sent':    float(r.get('bytes_sent', 0) or 0),
            'bytes_on_time': float(r.get('bytes_on_time', 0) or 0),
            'completed':     completed,
            'drops':         drops,
            'ddr_pct':       (100.0 * drops / tot) if tot else 0.0,
            'avg_qd_ms':     float(r.get('avg_queue_delay_ms', 0) or 0),
        }
    # Jain sobre useful goodput por classe (justiça de VOLUME de bytes).
    # Atenção: dominado pelo fundo (~96% dos tiles), então quase não varia com a
    # política — não serve para comparar escalonadores.
    v = np.array([out[c]['bytes_on_time'] for c in CLASS_ORDER], dtype=float)
    out['jain_c'] = (v.sum()**2 / (3.0 * (v**2).sum())) if (v**2).sum() > 0 else 0.0

    # Jain sobre a TAXA DE SUCESSO DE PRAZO por classe (justiça de TRATAMENTO):
    # mede se as três classes têm taxas de on-time parecidas. Diferencia as
    # políticas (uma classe faminta derruba o índice), ao contrário do de bytes.
    ot = np.array([max(0.0, 1.0 - out[c]['ddr_pct'] / 100.0) for c in CLASS_ORDER], dtype=float)
    out['jain_fair'] = (ot.sum()**2 / (3.0 * (ot**2).sum())) if (ot**2).sum() > 0 else 0.0
    return out


# --- Client: aggregate per-request statistics ------------------------------

def client_stats_one(csv_path):
    tmr_num = defaultdict(int); tmr_den = defaultdict(int)
    fov_num=0; fov_den=0; nfov_num=0; nfov_den=0
    bitrates=[]; prev_br=None; switches=0
    buffer_samples=[]; stall_samples=0
    try:
        with open(csv_path, newline='') as f:
            for row in csv.DictReader(f):
                prio = int(row.get('priority', 0))
                cls  = PRIO_TO_CLASS.get(prio, 'low')
                in_fov = row.get('in_fov', 'false') == 'true'
                on_time= row.get('on_time', 'false') == 'true'
                skipped= row.get('skipped', 'false') == 'true'
                timedout= row.get('timedout', 'false') == 'true'
                try: br = int(row.get('bitrate', 0) or 0)
                except: br = 0
                try: buf= float(row.get('buffer_s', 0) or 0)
                except: buf= 0.0

                tmr_den[cls] += 1
                if not on_time: tmr_num[cls] += 1
                if in_fov:
                    fov_den += 1
                    if not on_time: fov_num += 1
                else:
                    nfov_den += 1
                    if not on_time: nfov_num += 1

                if not skipped and not timedout and br > 0:
                    bitrates.append(br)
                    if prev_br is not None and br != prev_br: switches += 1
                    prev_br = br
                buffer_samples.append(buf)
                if buf <= 0.01: stall_samples += 1
    except Exception:
        return None
    if sum(tmr_den.values()) == 0:
        return None
    return {
        'tmr_class':      {c: (tmr_num[c]/tmr_den[c]*100.0 if tmr_den[c] else 0.0) for c in CLASS_ORDER},
        'tmr_fov':        (fov_num/fov_den*100.0) if fov_den else 0.0,
        'tmr_nonfov':     (nfov_num/nfov_den*100.0) if nfov_den else 0.0,
        'fhr_fov':        (100.0 - fov_num/fov_den*100.0) if fov_den else 100.0,
        'avg_bitrate':    (np.mean(bitrates) if bitrates else 0.0),
        'switch_rate':    (switches/len(bitrates) if bitrates else 0.0),
        'stall_ratio':    (stall_samples/len(buffer_samples) if buffer_samples else 0.0),
    }


def load_fov_assignment(run_dir):
    """Retorna lista ordenada de perfis (wide/normal/shorter) por índice de cliente,
    ou None se o arquivo fov_assignment.csv não existir (runs antigos).
    """
    p = os.path.join(run_dir, 'fov_assignment.csv')
    if not os.path.exists(p):
        return None
    profiles = []
    try:
        with open(p, newline='') as f:
            for row in csv.DictReader(f):
                profiles.append(row.get('profile', 'normal'))
    except Exception:
        return None
    return profiles


def client_stats_all(run):
    """Retorna lista de dicts, um por cliente (up to nc).

    Se fov_assignment.csv estiver disponível na pasta do run, anexa o campo
    'profile' a cada dict. Caso contrário, o campo fica ausente e a análise
    por perfil precisa recair sobre a proxy fov_mix (balanced × wide_heavy).
    """
    csvs = sorted(p for p in glob.glob(os.path.join(run['dir'], 'statistics-*.csv'))
                  if 'summary' not in p and os.path.getsize(p) > 10_000)
    profiles = load_fov_assignment(run['dir'])
    out = []
    for i, p in enumerate(csvs):
        s = client_stats_one(p)
        if not s: continue
        if profiles is not None and i < len(profiles):
            s['profile'] = profiles[i]
        out.append(s)
    return out


def aggregate_run(run):
    """Junta métricas server + client agregadas por run.

    Inclui, conforme especificação do professor (seções 14.11 e 14.12):
      - jain_q: Jain entre usuários aplicado à QoE por cliente (eq. 29)
      - qoe_agg: QoE agregado do professor (eq. 31),
                 QoE = FHR − TMR − 0.1·Rebuffer − 0.05·Oscillation
    """
    srv = server_metrics(run)
    cli = client_stats_all(run)
    if not cli:
        return None
    def m(k):  return float(np.mean([c[k] for c in cli]))
    tmr_class = {c: float(np.mean([x['tmr_class'][c] for x in cli])) for c in CLASS_ORDER}

    # QoE por cliente (eq. 31): FHR e TMR em [0,1]; stall/switch já em [0,1].
    qoe_by_client = []
    for c in cli:
        fhr = c['fhr_fov'] / 100.0
        tmr = c['tmr_fov'] / 100.0
        qoe_u = fhr - tmr - 0.1 * c['stall_ratio'] - 0.05 * c['switch_rate']
        qoe_by_client.append(qoe_u)

    # Jain entre usuários (eq. 29). Só faz sentido com valores não-negativos —
    # deslocamos para garantir positividade sem alterar variância relativa.
    qs = np.array(qoe_by_client, dtype=float)
    if len(qs) > 0:
        qs_shift = qs - qs.min() + 1e-9
        denom = len(qs) * float((qs_shift ** 2).sum())
        jain_q = (float(qs_shift.sum()) ** 2) / denom if denom > 0 else 0.0
    else:
        jain_q = 0.0

    qoe_agg = float(np.mean(qs)) if len(qs) else 0.0

    return {
        **{k: run[k] for k in ('sc','pol','abr','nc','mix','dir','rep')},
        'server': srv,
        'tmr_fov':    m('tmr_fov'),
        'tmr_nonfov': m('tmr_nonfov'),
        'fhr_fov':    m('fhr_fov'),
        'tmr_class':  tmr_class,
        'tmr_fov_by_client': [c['tmr_fov'] for c in cli],
        'qoe_by_client': qoe_by_client,
        'qoe_agg':     qoe_agg,
        'jain_q':      jain_q,
        'avg_bitrate': m('avg_bitrate'),
        'switch_rate': m('switch_rate'),
        'stall_ratio': m('stall_ratio'),
        'n_clients':  len(cli),
    }


# --- Time series loaders -------------------------------------------------

def load_fairness(run):
    """Retorna arrays t (segundos desde início), bytes_low/med/high, share_low/med/high."""
    p = os.path.join(run['dir'], 'fairness.csv')
    bL=[]; bM=[]; bH=[]; sL=[]; sM=[]; sH=[]
    try:
        with open(p) as f:
            for row in csv.DictReader(f):
                try:
                    bL.append(float(row['bytes_low']))
                    bM.append(float(row['bytes_medium']))
                    bH.append(float(row['bytes_high']))
                    sL.append(float(row['share_low']))
                    sM.append(float(row['share_medium']))
                    sH.append(float(row['share_high']))
                except: continue
    except Exception:
        pass
    t = np.arange(len(bL))
    return t, np.array(bL), np.array(bM), np.array(bH), np.array(sL), np.array(sM), np.array(sH)


def load_wfq_util(run):
    """Retorna dict: share observado e peso normalizado por classe.

    Auto-detecta CSVs antigos (bug corrigido no wfq_utilization.go após 2026-07):
    antes do fix, weights[0]=HIGH_PRIORITY (FoV) era escrito na coluna 'w_low'.
    Heurística: os pesos base do WFQ são {LOW=1, MED=2, HIGH=3}, então raw_w
    da classe FoV começa em ~3.0. Se raw_w_low > raw_w_high na primeira linha
    com tráfego, o CSV está com labels invertidos e trocamos low ↔ high na
    leitura.
    """
    p = os.path.join(run['dir'], 'wfq_utilization.csv')
    obs = {'high':[], 'medium':[], 'low':[]}
    tgt = {'high':[], 'medium':[], 'low':[]}
    if not os.path.exists(p):
        return {c: (np.array(tgt[c]), np.array(obs[c])) for c in CLASS_ORDER}

    # Detecta inversão: acha primeira linha com bytes>0 e compara raw_w_low vs raw_w_high.
    inverted = False
    try:
        with open(p) as f:
            for row in csv.DictReader(f):
                try:
                    b_lo = float(row.get('bytes_low', 0))
                    b_hi = float(row.get('bytes_high', 0))
                    if b_lo + b_hi <= 0:
                        continue
                    rw_lo = float(row.get('raw_w_low', 0))
                    rw_hi = float(row.get('raw_w_high', 0))
                    # CSV correto: raw_w_low (fundo) ≤ raw_w_high (FoV, ~3.0).
                    # CSV invertido: raw_w_low ≈ 3.0 > raw_w_high.
                    if rw_lo > rw_hi + 0.5:
                        inverted = True
                    break
                except Exception:
                    continue
    except Exception:
        return {c: (np.array(tgt[c]), np.array(obs[c])) for c in CLASS_ORDER}

    # Mapeamento: em CSV invertido, coluna '_low' contém dados de HIGH_PRIORITY (FoV).
    if inverted:
        col_by_class = {'high': '_low', 'medium': '_medium', 'low': '_high'}
    else:
        col_by_class = {'high': '_high', 'medium': '_medium', 'low': '_low'}

    try:
        with open(p) as f:
            for row in csv.DictReader(f):
                try:
                    for c, k in col_by_class.items():
                        obs[c].append(float(row['share'+k]))
                        tgt[c].append(float(row['w'+k]))
                except: continue
    except Exception:
        pass
    return {c: (np.array(tgt[c]), np.array(obs[c])) for c in CLASS_ORDER}


# --- Per-request reqlog loader (for Fig 3, 4) ----------------------------

def load_reqlog(run, max_rows=None):
    """
    Retorna lista de tuplas (class_str, qd_ms, drop_bool) do reqlog.csv.
    """
    p = os.path.join(run['dir'], 'reqlog.csv')
    out = []
    try:
        with open(p, newline='') as f:
            for i, row in enumerate(csv.DictReader(f)):
                if max_rows is not None and i >= max_rows:
                    break
                try:
                    cls = PRIO_TO_CLASS.get(int(row['class']), 'low')
                    qd  = float(row['qd_ms'])
                    drop= row.get('drop', 'false') == 'true'
                    out.append((cls, qd, drop))
                except Exception:
                    continue
    except Exception:
        pass
    return out


# ─── FIG 1 — Alocação temporal (stacked area por política) ───────────────────

def fig1_alloc_timeseries(idx, out_dir, mix='balanced'):
    """
    Grade 2×3: 2 cenários extremos (Sc2 pior, Sc5 melhor) × 3 políticas.
    Cada painel: taxa por classe ao longo do tempo (Mbps).
    Revela contraste de mecanismo de alocação entre regimes e políticas.
    """
    plt.rcParams.update(PAPER_RC)
    fig, axes = plt.subplots(2, 3, figsize=(12, 5.5), sharex=True, sharey=True)

    def smooth(x, w=5):
        if len(x) < w: return x
        k = np.ones(w) / w
        return np.convolve(x, k, mode='same')

    scenarios_shown = ['2', '5']  # pior (Sc2: 24ms, 30%) e melhor (Sc5: 10ms, 10%)

    for ri, sc in enumerate(scenarios_shown):
        for ci, pol in enumerate(POLICIES):
            ax = axes[ri, ci]
            runs = match(idx, sc=sc, pol=pol, abr='BOLA', nc=6, mix=mix)
            if not runs:
                ax.text(0.5, 0.5, 'sem dado', ha='center', va='center',
                        transform=ax.transAxes, fontsize=10, color='#888')
                continue
            t, bL, bM, bH, _, _, _ = load_fairness(runs[0])
            mL = smooth(bL * 8.0 / 1e6)  # Mbps
            mM = smooth(bM * 8.0 / 1e6)
            mH = smooth(bH * 8.0 / 1e6)

            ax.plot(t, mH, color=CL_FOV,  lw=1.8, label=CLASS_LABEL['high'], zorder=5)
            ax.plot(t, mM, color=CL_NEAR, lw=1.4, label=CLASS_LABEL['medium'], zorder=4)
            ax.plot(t, mL, color=CL_BG,   lw=1.2, label=CLASS_LABEL['low'],  zorder=3, alpha=0.85)

            ax.set_yscale('symlog', linthresh=0.01)
            ax.set_ylim(0, 100)
            ax.grid(axis='y', which='both', alpha=0.3)

            if ci == 0:
                ax.set_ylabel(f'{SC_LABEL[sc]}\nTaxa (Mbps)', fontsize=10)
            if ri == 0:
                ax.set_title(pol, fontsize=11, fontweight='bold', color=PAL_POL[pol])
            if ri == 1:
                ax.set_xlabel('Tempo (s)', fontsize=10)

    # legenda das classes no topo
    from matplotlib.lines import Line2D
    handles = [Line2D([], [], color=CLASS_COLOR[c], lw=2, label=CLASS_LABEL[c]) for c in CLASS_ORDER]
    fig.legend(handles=handles, loc='upper center', ncol=3, fontsize=10,
               frameon=False, bbox_to_anchor=(0.5, 1.02))
    fig.suptitle('Alocação temporal de banda por classe — BOLA, 6 clientes',
                 fontsize=11, fontweight='bold', y=1.06)
    plt.tight_layout()
    _save(fig, out_dir, 'fig1_alloc_timeseries')


# ─── FIG 2 — WFQ intenção × observado ────────────────────────────────────────

def fig2_wfq_intent_vs_actual(idx, out_dir):
    plt.rcParams.update(PAPER_RC)
    fig, axes = plt.subplots(1, 3, figsize=(9.5, 3.3), sharex=True, sharey=True)

    # Coletar dados de todas as runs WFQ 6c BOLA balanced, agrupado por cenário
    scenario_colors = {'1': '#E41A1C', '2': '#377EB8', '5': '#4DAF4A', '6': '#984EA3'}
    for ax, cls in zip(axes, CLASS_ORDER):
        for sc in SCENARIOS:
            runs = match(idx, sc=sc, pol='WFQ', abr='BOLA', nc=6, mix='balanced')
            all_tgt, all_obs = [], []
            for r in runs:
                data = load_wfq_util(r)
                tgt, obs = data[cls]
                # descartar samples com bytes=0 (janelas ociosas)
                if len(tgt) > 0 and len(obs) > 0:
                    mask = (tgt > 0) | (obs > 0)
                    all_tgt.extend(tgt[mask].tolist())
                    all_obs.extend(obs[mask].tolist())
            if not all_tgt: continue
            ax.scatter(all_tgt, all_obs, s=6, alpha=0.35,
                       color=scenario_colors[sc], label=SC_SHORT[sc],
                       edgecolor='none')

        # diagonal de referência
        ax.plot([0, 1], [0, 1], color='#333', lw=0.8, ls='--', zorder=1)
        ax.set_xlim(-0.02, 1.02); ax.set_ylim(-0.02, 1.02)
        ax.set_xlabel(r'Peso pretendido $\phi_p$', fontsize=10)
        ax.set_title(f'{CLASS_LABEL[cls]}', fontsize=10)
        ax.grid(True)

    axes[0].set_ylabel('Share observado (bytes)', fontsize=10)
    axes[-1].legend(title='Cenário', fontsize=8, title_fontsize=8, loc='lower right', markerscale=2)
    fig.suptitle('WFQ dinâmico: intenção × realidade — BOLA · 6 clientes · balanced\n'
                 'cada ponto = 1 janela de 1 s em 1 rep (agregado de 5 reps × 4 cenários)',
                 fontsize=11, fontweight='bold')
    plt.tight_layout()
    _save(fig, out_dir, 'fig2_wfq_intent_vs_actual')


# ─── FIG 3 — CCDF do queue delay por classe × política ───────────────────────

def fig3_qd_ccdf(idx, out_dir, mix='balanced'):
    """
    Grade 2 (ABR) × 3 (classe), 3 curvas por painel (uma por política).
    Agrega TODOS os cenários para ficar legível.
    """
    plt.rcParams.update(PAPER_RC)
    fig, axes = plt.subplots(2, 3, figsize=(11, 6.0), sharex=True, sharey=True)

    ABRS = ['BOLA', 'LEGACY']
    for ri, abr in enumerate(ABRS):
        for ci, cls in enumerate(CLASS_ORDER):
            ax = axes[ri, ci]
            for pol in POLICIES:
                runs = []
                for sc in SCENARIOS:
                    runs += match(idx, sc=sc, pol=pol, abr=abr, nc=6, mix=mix)
                vals = []
                for r in runs:
                    for c, qd, _d in load_reqlog(r):
                        if c == cls and qd >= 0:
                            vals.append(qd)
                if not vals: continue
                v = np.sort(np.array(vals))
                ccdf = 1.0 - np.arange(len(v)) / len(v)
                ax.plot(np.maximum(v, 0.5), ccdf,
                        color=PAL_POL[pol], lw=1.8, alpha=0.9,
                        label=pol)

            ax.set_xscale('log'); ax.set_yscale('log')
            ax.set_xlim(0.5, 10000); ax.set_ylim(1e-4, 1.2)
            ax.grid(True, which='both', alpha=0.3)
            if ri == 0:
                ax.set_title(CLASS_LABEL[cls], fontsize=11, color=CLASS_COLOR[cls], fontweight='bold')
            if ci == 0:
                ax.set_ylabel(f'{ABR_LABEL.get(abr, abr)}\nP[qd > d]', fontsize=10)
            if ri == 1:
                ax.set_xlabel('Queue delay (ms)', fontsize=10)

    # legenda única
    from matplotlib.lines import Line2D
    els = [Line2D([],[], color=PAL_POL[p], lw=2, label=p) for p in POLICIES]
    fig.legend(handles=els, loc='upper center', ncol=3, fontsize=10,
               frameon=False, bbox_to_anchor=(0.5, 1.03))

    fig.suptitle('Cauda de atraso de fila por classe e ABR — 6 clientes · agregado '
                 'de 5 reps × 4 cenários × 2 mixes = 40 runs por curva',
                 fontsize=11, fontweight='bold', y=1.07)
    plt.tight_layout()
    _save(fig, out_dir, 'fig3_qd_ccdf')


# ─── FIG 4 — Delay → Drop (hexbin + logística) ───────────────────────────────

def fig4_delay_drop(idx, out_dir, mix='balanced'):
    plt.rcParams.update(PAPER_RC)
    fig, ax = plt.subplots(figsize=(7.5, 4.5))

    # coleta qd e drop por política, agregando todos os cenários
    curves = {}
    for pol in POLICIES:
        runs = []
        for sc in SCENARIOS:
            runs += match(idx, sc=sc, pol=pol, abr='BOLA', nc=6, mix=mix)
        qd_all = []; drop_all = []
        for r in runs:
            for _cls, qd, drop in load_reqlog(r):
                if qd >= 0:
                    qd_all.append(qd)
                    drop_all.append(1 if drop else 0)
        if not qd_all: continue
        qd = np.array(qd_all); dr = np.array(drop_all)
        qd_pos = np.maximum(qd, 0.5)

        # bins log — computa P[drop|qd] em cada bin
        edges = np.logspace(np.log10(1), np.log10(max(qd_pos.max(), 10) + 1), 30)
        centers = []; probs = []; ns = []
        for i in range(len(edges) - 1):
            m = (qd_pos >= edges[i]) & (qd_pos < edges[i + 1])
            if m.sum() > 30:
                centers.append(np.sqrt(edges[i] * edges[i + 1]))
                probs.append(dr[m].mean())
                ns.append(m.sum())
        curves[pol] = (np.array(centers), np.array(probs), np.array(ns), qd_pos)

    # plot: uma linha por política, mesmo painel
    for pol, (centers, probs, ns, qd_pos) in curves.items():
        # linha principal
        ax.plot(centers, probs, color=PAL_POL[pol], lw=2.2, marker=MK_POL[pol],
                markersize=6, label=pol, zorder=4)
        # p95 do delay observado como marcador
        p95 = np.percentile(qd_pos, 95)
        ax.axvline(p95, color=PAL_POL[pol], lw=0.8, ls=':', alpha=0.5, zorder=1)
        ax.text(p95, 1.05, f'p95', color=PAL_POL[pol], fontsize=8,
                ha='center', va='bottom')

    ax.set_xscale('log')
    ax.set_xlim(1, 10000); ax.set_ylim(-0.02, 1.1)
    ax.set_xlabel('Queue delay (ms)', fontsize=11)
    ax.set_ylabel(r'P[drop $\mid$ queue delay]', fontsize=11)
    ax.set_title('Probabilidade de drop condicional ao atraso de fila — BOLA · 6 clientes\n'
                 'agregado de 5 reps × 4 cenários × 2 mixes por política',
                 fontsize=11, fontweight='bold')
    ax.grid(True, which='both', alpha=0.3)
    ax.legend(title='Política', title_fontsize=10, loc='upper left', framealpha=0.9)

    plt.tight_layout()
    _save(fig, out_dir, 'fig4_delay_drop')


# ─── FIG 5 — Pareto FoV protection × Jain_C ──────────────────────────────────

def fig5_pareto(agg, out_dir):
    """
    Barras agrupadas: DDR (%) por classe × política × cenário, um painel por ABR.
    Cada barra mostra a MÉDIA das reps; strip plot em cima mostra os 5 pontos
    individuais para deixar a dispersão visível.
    Escala logarítmica no eixo Y — sem isso, valores como 0.2% e 2.3% ficam
    indistinguíveis. Dois painéis lado a lado: FoV (DDR_high) e fundo (DDR_low).
    """
    plt.rcParams.update(PAPER_RC)
    fig, axes = plt.subplots(2, 2, figsize=(13.5, 8.0), sharex=True)
    rng = np.random.default_rng(42)

    n_pol = len(POLICIES)
    n_sc  = len(SCENARIOS)
    bar_w = 0.24
    group_positions = np.arange(n_sc)
    pol_offsets = {p: (i - (n_pol - 1) / 2) * bar_w for i, p in enumerate(POLICIES)}

    for row, cls in enumerate(['high', 'low']):  # linha 0 = FoV, linha 1 = fundo
        for col, abr in enumerate(['BOLA', 'LEGACY']):
            ax = axes[row, col]

            for pol in POLICIES:
                means = []; xs_strip = []; ys_strip = []
                for si, sc in enumerate(SCENARIOS):
                    rr = [r for r in agg if r['pol']==pol and r['abr']==abr
                          and r['nc']==6 and r['sc']==sc]
                    if not rr:
                        means.append(0.0); continue
                    vals = [r['server'][cls]['ddr_pct'] for r in rr]
                    means.append(float(np.mean(vals)))
                    x_center = si + pol_offsets[pol]
                    jitter = rng.uniform(-bar_w * 0.25, bar_w * 0.25, len(vals))
                    xs_strip.extend((x_center + jitter).tolist())
                    ys_strip.extend([max(v, 1e-3) for v in vals])  # log scale precisa > 0

                positions = group_positions + pol_offsets[pol]
                bar_heights = [max(m, 1e-3) for m in means]
                ax.bar(positions, bar_heights, width=bar_w,
                       color=PAL_POL[pol], alpha=0.55,
                       edgecolor=PAL_POL[pol], linewidth=1.2,
                       label=pol if row == 0 and col == 0 else None,
                       zorder=2)
                ax.scatter(xs_strip, ys_strip, s=14, color=PAL_POL[pol],
                           edgecolor='white', linewidth=0.4, alpha=0.9, zorder=4)

            ax.set_yscale('log')
            ax.set_ylim(1e-3, 30)
            ax.set_xticks(group_positions)
            ax.set_xticklabels([SC_LABEL[s] for s in SCENARIOS], fontsize=8)
            ax.grid(True, axis='y', which='both', alpha=0.3)
            ax.axhline(1.0, color='#888', lw=0.6, ls=':', zorder=1)

            if row == 0:
                ax.set_title(f'{ABR_LABEL.get(abr, abr)}',
                             fontsize=12, fontweight='bold')
            if col == 0:
                cls_label = 'DDR do FoV (%)' if cls == 'high' else 'DDR do fundo (%)'
                ax.set_ylabel(cls_label, fontsize=11)
            if row == 1:
                ax.set_xlabel('Cenário', fontsize=10)

    axes[0, 0].legend(title='Política', title_fontsize=10, fontsize=9,
                      loc='upper left', framealpha=0.9)

    fig.suptitle(
        'Taxa de descarte por prazo perdido (DDR) — 6 clientes · média de 5 reps por barra '
        '· pontos = reps individuais\n(escala log · linha pontilhada = 1%)',
        fontsize=11, fontweight='bold', y=1.00)
    plt.tight_layout()
    _save(fig, out_dir, 'fig5_pareto')


# ─── FIG 6 — TMR por cliente (dispersão intra-run) ───────────────────────────

def fig6_tmr_by_client(agg, out_dir, mix='balanced'):
    plt.rcParams.update(PAPER_RC)
    fig, axes = plt.subplots(1, 4, figsize=(14, 3.6), sharey=True)

    rng = np.random.default_rng(42)
    x_by_pol = {p: i for i, p in enumerate(POLICIES)}

    for ax, sc in zip(axes, SCENARIOS):
        for pol in POLICIES:
            xi = x_by_pol[pol]
            rr = [r for r in agg if r['sc']==sc and r['pol']==pol and r['abr']=='BOLA' and r['nc']==6 and r['mix']==mix]
            all_vals = []
            for r in rr:
                for v in r['tmr_fov_by_client']:
                    all_vals.append(v)
            if not all_vals: continue
            # strip + boxplot minimalista
            jitter = rng.uniform(-0.18, 0.18, len(all_vals))
            ax.scatter(xi + jitter, all_vals, s=25, color=PAL_POL[pol],
                       alpha=0.55, edgecolor='white', linewidth=0.4, zorder=3)
            # box interno
            q1, med, q3 = np.percentile(all_vals, [25, 50, 75])
            ax.plot([xi-0.3, xi+0.3], [med, med], color=PAL_POL[pol], lw=2, zorder=5)
            ax.plot([xi-0.22, xi+0.22], [q1, q1], color=PAL_POL[pol], lw=1, zorder=5)
            ax.plot([xi-0.22, xi+0.22], [q3, q3], color=PAL_POL[pol], lw=1, zorder=5)
            ax.plot([xi, xi], [q1, q3], color=PAL_POL[pol], lw=1, zorder=5)

        ax.set_xticks(range(len(POLICIES)))
        ax.set_xticklabels(POLICIES, fontsize=10)
        ax.set_title(SC_LABEL[sc], fontsize=10)
        ax.grid(axis='y')
        ax.set_ylim(bottom=-0.5)
    axes[0].set_ylabel('TMR FoV por cliente (%)', fontsize=10)
    fig.suptitle('Variabilidade de TMR entre usuários — BOLA · 6 clientes · balanced\n'
                 'cada ponto = 1 cliente em 1 rep (até 6 × 5 = 30 pontos por barra)',
                 fontsize=11, fontweight='bold')
    plt.tight_layout()
    _save(fig, out_dir, 'fig6_tmr_by_client')


# ─── FIG 7 — QoE decomposta ──────────────────────────────────────────────────

def fig7_qoe_radar(agg, out_dir, mix='balanced'):
    """
    Radar chart com 6 dimensões da QoE:
      FHR_FoV, Qualidade (bitrate norm.), Playback (1-stall),
      Estabilidade (1-switch), Fairness (Jain_C), Responsividade (1-avg_qd norm.).
    Apenas o regime BOLA — regime com contenção, onde a política de fato importa.
    O ABR por vazão é omitido porque, em rede estável, mantém as filas vazias e
    torna FIFO/SP/WFQ indistinguíveis (ver texto). Média entre cenários.
    """
    plt.rcParams.update(PAPER_RC)

    # Eixo de qualidade: nível médio de bitrate normalizado pelo maior nível
    # médio observado no recorte (para o eixo ficar em [0,1] como os demais).
    _q = [r['avg_bitrate'] for r in agg
          if r['nc'] == 6 and r['mix'] == mix and r['avg_bitrate'] > 0]
    qmax = max(_q) if _q else 1.0

    METRICS = [
        ('FHR FoV',       'fhr_fov',      lambda v: v/100.0),
        ('Qualidade',     'avg_bitrate',  lambda v: v/qmax),
        ('Playback',      'stall_ratio',  lambda v: max(0.0, 1.0 - v)),
        ('Estabilidade',  'switch_rate',  lambda v: max(0.0, 1.0 - 10.0*v)),  # switch é pequeno
        ('Fairness',      'jain_fair',    lambda v: v),  # justiça de tratamento (taxa de prazo)
        ('Responsivid.',  'avg_qd_high',  lambda v: max(0.0, 1.0 - v/1000.0)),  # 0ms=1, 1s=0
    ]
    N = len(METRICS)
    angles = np.linspace(0, 2*np.pi, N, endpoint=False).tolist()
    angles += angles[:1]  # fecha o polígono

    fig, ax = plt.subplots(figsize=(6.2, 5.6), subplot_kw=dict(projection='polar'))

    abr = 'BOLA'
    for pol in POLICIES:
        rr = [r for r in agg if r['pol']==pol and r['abr']==abr and r['nc']==6 and r['mix']==mix]
        if not rr: continue
        vals = []
        for _, key, transform in METRICS:
            if key in ('jain_c', 'jain_fair'):
                raw = [r['server'][key] for r in rr]
            elif key == 'avg_qd_high':
                raw = [r['server']['high']['avg_qd_ms'] for r in rr]
            else:
                raw = [r[key] for r in rr]
            vals.append(transform(float(np.mean(raw))))
        vals += vals[:1]  # fecha o polígono
        lw = 2.6 if pol == 'SP' else 2.0
        ax.plot(angles, vals, color=PAL_POL[pol], lw=lw, label=pol,
                marker=MK_POL[pol], markersize=5, zorder=5 if pol == 'SP' else 3)
        ax.fill(angles, vals, color=PAL_POL[pol], alpha=0.12)

    ax.set_xticks(angles[:-1])
    ax.set_xticklabels([m[0] for m in METRICS], fontsize=10)
    ax.set_ylim(0, 1.05)
    ax.set_yticks([0.25, 0.5, 0.75, 1.0])
    ax.set_yticklabels(['0.25','0.50','0.75','1.00'], fontsize=7, color='#888')
    ax.grid(True, alpha=0.4)
    ax.set_theta_offset(np.pi/2)
    ax.set_theta_direction(-1)
    ax.legend(loc='upper right', bbox_to_anchor=(1.28, 1.12), fontsize=10)

    fig.suptitle('Assinatura de QoE por política — BOLA · 6 clientes, balanced\n'
                 r'(cada eixo em [0,1]: mais próximo da borda é melhor)',
                 fontsize=11, fontweight='bold', y=1.04)
    plt.tight_layout()
    _save(fig, out_dir, 'fig7_qoe_radar')


# ─── FIG 8 — Heatmap conditional dos regimes ─────────────────────────────────

def fig8_regime_heatmap(agg, out_dir):
    """Grade 2×2: (DDR_FoV, Queue Delay FoV) × (BOLA, Throughput)."""
    plt.rcParams.update(PAPER_RC)
    fig, axes = plt.subplots(2, 2, figsize=(13, 6.5))

    combos = [(sc, mix) for sc in SCENARIOS for mix in ('balanced','wide_heavy')]
    col_labels = [f'{SC_SHORT[sc]}\n{"bal" if m=="balanced" else "wide"}' for sc,m in combos]

    panels = [
        # (row, col, key, abr, title, cmap, vmax)
        (0, 0, 'ddr', 'BOLA',   'DDR FoV (%) — BOLA',   'Reds', None),
        (0, 1, 'ddr', 'LEGACY', 'DDR FoV (%) — Throughput', 'Reds', None),
        (1, 0, 'qd',  'BOLA',   'Queue Delay FoV (ms) — BOLA',   'Purples', None),
        (1, 1, 'qd',  'LEGACY', 'Queue Delay FoV (ms) — Throughput', 'Purples', None),
    ]

    # calcula vmax global por métrica
    all_ddr = []; all_qd = []
    for r in agg:
        if r['nc']==6:
            all_ddr.append(r['server']['high']['ddr_pct'])
            all_qd.append(r['server']['high']['avg_qd_ms'])
    vmax_ddr = max(np.max(all_ddr) if all_ddr else 1, 1.0)
    vmax_qd  = max(np.max(all_qd)  if all_qd  else 1, 100.0)

    for (ri, ci, key, abr, title, cmap, _v) in panels:
        ax = axes[ri, ci]
        M = np.full((len(POLICIES), len(combos)), np.nan)
        for pi, pol in enumerate(POLICIES):
            for coli, (sc, mix) in enumerate(combos):
                rr = [r for r in agg if r['sc']==sc and r['pol']==pol
                      and r['abr']==abr and r['nc']==6 and r['mix']==mix]
                if not rr: continue
                if key == 'ddr':
                    M[pi, coli] = np.mean([r['server']['high']['ddr_pct'] for r in rr])
                else:
                    M[pi, coli] = np.mean([r['server']['high']['avg_qd_ms'] for r in rr])
        vmax = vmax_ddr if key=='ddr' else vmax_qd
        im = ax.imshow(M, aspect='auto', cmap=cmap, vmin=0, vmax=vmax, interpolation='nearest')
        for pi in range(M.shape[0]):
            for coli in range(M.shape[1]):
                v = M[pi, coli]
                if np.isnan(v):
                    ax.text(coli, pi, '—', ha='center', va='center', fontsize=9, color='#aaa')
                    continue
                is_dark = v > vmax * 0.60
                col = 'white' if is_dark else '#222'
                fmt = f'{v:.1f}' if key=='ddr' else f'{v:.0f}'
                ax.text(coli, pi, fmt, ha='center', va='center',
                        fontsize=9, color=col, fontweight='bold')
        ax.set_xticks(range(len(combos))); ax.set_xticklabels(col_labels, fontsize=8)
        ax.set_yticks(range(len(POLICIES))); ax.set_yticklabels(POLICIES, fontsize=10)
        ax.set_title(title, fontsize=10, pad=6)
        for sep in [1.5, 3.5, 5.5]:
            ax.axvline(sep, color='white', lw=1.5)
        for spine in ax.spines.values(): spine.set_visible(False)
        ax.tick_params(left=False, bottom=False)
        plt.colorbar(im, ax=ax, fraction=0.035, pad=0.02)

    fig.suptitle('Panorama do desempenho por regime — 6 clientes\n'
                 'cada célula = média de 5 reps',
                 fontsize=11, fontweight='bold')
    plt.tight_layout()
    _save(fig, out_dir, 'fig8_regime_heatmap')


# ─── Util ────────────────────────────────────────────────────────────────────

def _save(fig, out_dir, name):
    for ext in ('png', 'pdf'):
        p = os.path.join(out_dir, f'{name}.{ext}')
        fig.savefig(p)
    plt.close(fig)
    print(f'[Fig] {name}.png/pdf')


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('matrix_dirs', nargs='+', help='Um ou mais diretórios matrix-* (ex.: matrix-006 matrix-007)')
    ap.add_argument('-o', '--out', default=None)
    args = ap.parse_args()

    matrix_dirs = [os.path.abspath(d) for d in args.matrix_dirs]
    # dir de saída = pai comum ou primeiro dir
    if args.out:
        out_dir = args.out
    else:
        common = os.path.dirname(matrix_dirs[0])
        out_dir = os.path.join(common, 'figs_paper_v2')
    os.makedirs(out_dir, exist_ok=True)

    print(f'Indexando {len(matrix_dirs)} diretório(s):')
    for d in matrix_dirs:
        print(f'  - {d}')
    idx = index_runs(matrix_dirs)
    print(f'  {len(idx)} runs indexadas.')

    print('Agregando métricas por run (server + client)...')
    agg = []
    for r in idx:
        a = aggregate_run(r)
        if a is not None:
            agg.append(a)
    print(f'  {len(agg)} runs com métricas agregadas.')

    # Sanity check: reps por config
    from collections import Counter as _C
    rep_dist = _C()
    for a in agg:
        rep_dist[(a['sc'], a['pol'], a['abr'], a['nc'], a['mix'])] += 1
    print(f'  Reps por config: min={min(rep_dist.values())}, max={max(rep_dist.values())}, mediana={sorted(rep_dist.values())[len(rep_dist)//2]}')

    print('\nGerando figuras (isso pode demorar por causa do reqlog.csv)...')
    fig1_alloc_timeseries(idx, out_dir)
    fig2_wfq_intent_vs_actual(idx, out_dir)
    fig3_qd_ccdf(idx, out_dir)
    fig4_delay_drop(idx, out_dir)
    fig5_pareto(agg, out_dir)
    fig6_tmr_by_client(agg, out_dir)
    fig7_qoe_radar(agg, out_dir)
    fig8_regime_heatmap(agg, out_dir)

    print(f'\nFiguras em: {out_dir}')


if __name__ == '__main__':
    main()
