# -*- coding: utf-8 -*-
"""Regenera fig1_ddr_zonas.pdf e fig4_justica_usuarios.pdf com cenarios #1-#4.

Mapeamento de cenario: dado s1->#1, s2->#2, s5->#3, s6->#4 (remove os buracos 3/4).
Fonte de dados: logs/matrix-014 (6 clientes, BOLA, ambas distribuicoes de FoV, n=10).
Reaproveita a camada de agregacao de plot_paper_v2.py.
"""
import os, sys
import numpy as np
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import plot_paper_v2 as P

MATRIX = os.path.abspath(os.path.join(HERE, "..", "..", "..", "logs", "matrix-014"))
OUTDIR = os.path.abspath(os.path.join(
    HERE, "..", "..", "..", "..", "overleaf", "Server_side_FoV_Aware_TCC", "figuras"))
PNGDIR = r"C:\Users\REBECA~1\AppData\Local\Temp\claude\C--Users-rebeca-pedrosa-Documents-SINERGIA-TccQuic\a32669c5-dc58-4eb9-8009-22d4b8c76dfa\scratchpad"

# --- estilo (mesma identidade do TCC/apresentacao) ---
plt.rcParams.update({
    'font.family': 'serif',
    'font.serif': ['Times New Roman', 'DejaVu Serif', 'serif'],
    'font.size': 10, 'axes.labelsize': 10, 'axes.titlesize': 10,
    'xtick.labelsize': 9, 'ytick.labelsize': 9, 'legend.fontsize': 9,
    'axes.spines.top': False, 'axes.spines.right': False,
    'axes.linewidth': 0.8, 'grid.linewidth': 0.5, 'grid.alpha': 0.35,
    'lines.linewidth': 1.5, 'lines.markersize': 6,
    'savefig.dpi': 300, 'savefig.bbox': 'tight', 'savefig.pad_inches': 0.06,
})

# cores das politicas (identicas ao doc: colSPtxt/colWFQ/colFIFO)
COL = {'SP': '#1A1A1A', 'WFQ': '#C1272D', 'FIFO': '#1F5FA6'}
LBL = {'SP': 'Prioridade Estrita', 'WFQ': 'WFQ', 'FIFO': 'FIFO'}
MK  = {'SP': 'o', 'WFQ': 's', 'FIFO': '^'}
LS  = {'SP': '-', 'WFQ': '--', 'FIFO': '-.'}
FILL= {'SP': 'none', 'WFQ': 'full', 'FIFO': 'none'}
POLS = ['SP', 'WFQ', 'FIFO']

# cenarios: dado -> rotulo renumerado
SC_DATA  = ['1', '2', '5', '6']
SC_LABEL = ['#1', '#2', '#3', '#4']
XPOS = np.arange(len(SC_DATA))

# t de Student (bilateral 95%) por graus de liberdade
TCRIT = {1:12.706,2:4.303,3:3.182,4:2.776,5:2.571,6:2.447,7:2.365,8:2.306,
         9:2.262,10:2.228,11:2.201,12:2.179,13:2.160,14:2.145,15:2.131}

idx = P.index_runs(MATRIX)

def ci95(vals):
    a = np.asarray(vals, dtype=float)
    n = len(a)
    if n < 2:
        return (float(a.mean()) if n else 0.0), 0.0
    m = a.mean(); s = a.std(ddof=1)
    t = TCRIT.get(n-1, 1.96)
    return m, t * s / np.sqrt(n)

def runs_for(sc, pol):
    return [r for r in idx if r['sc']==sc and r['pol']==pol
            and r['abr']=='BOLA' and r['nc']==6]

# ------------------------------------------------------------------ FIG 1
def make_fig1():
    CLS = ['high', 'medium', 'low']
    PANEL = {'high': '(a) Campo de visão', 'medium': '(b) Vizinhança', 'low': '(c) Fundo'}
    # coleta media + IC por (zona, politica, cenario)
    data = {c: {p: ([], []) for p in POLS} for c in CLS}
    for c in CLS:
        for p in POLS:
            for sc in SC_DATA:
                vals = [P.server_metrics(r)[c]['ddr_pct'] for r in runs_for(sc, p)]
                m, e = ci95(vals)
                data[c][p][0].append(m); data[c][p][1].append(e)

    fig, axes = plt.subplots(3, 1, figsize=(6.0, 7.4), sharex=True)
    for ax, c in zip(axes, CLS):
        for p in POLS:
            m = np.array(data[c][p][0]); e = np.array(data[c][p][1])
            ax.errorbar(XPOS, m, yerr=e, label=LBL[p], color=COL[p],
                        marker=MK[p], linestyle=LS[p], fillstyle=FILL[p],
                        markeredgecolor=COL[p], markerfacecolor=(COL[p] if p=='WFQ' else 'white'),
                        capsize=3, elinewidth=0.9, capthick=0.9, clip_on=False, zorder=3)
        ax.set_title(PANEL[c], fontsize=10, loc='left')
        ax.set_ylabel('DDR (%)')
        ax.grid(True, axis='y', alpha=0.3)
        ax.set_xlim(-0.25, len(SC_DATA)-0.75)
        ax.margins(y=0.15)
        ax.set_ylim(bottom=0)
    axes[-1].set_xticks(XPOS)
    axes[-1].set_xticklabels(SC_LABEL)
    axes[-1].set_xlabel('Cenário de rede')
    axes[0].legend(loc='upper left', framealpha=0.9, ncol=1)
    fig.tight_layout()
    out = os.path.join(OUTDIR, 'fig1_ddr_zonas.pdf')
    fig.savefig(out)
    fig.savefig(os.path.join(PNGDIR, 'fig1_preview.png'), dpi=110)
    plt.close(fig)
    print('salvo:', out)

# ------------------------------------------------------------------ FIG 4
def make_fig4():
    data = {p: ([], []) for p in POLS}
    for p in POLS:
        for sc in SC_DATA:
            vals = []
            for r in runs_for(sc, p):
                a = P.aggregate_run(r)
                if a and a['tmr_fov_by_client']:
                    v = a['tmr_fov_by_client']
                    vals.append(max(v) - min(v))
            m, e = ci95(vals)
            data[p][0].append(m); data[p][1].append(e)

    fig, ax = plt.subplots(figsize=(6.0, 4.0))
    for p in POLS:
        m = np.array(data[p][0]); e = np.array(data[p][1])
        ax.errorbar(XPOS, m, yerr=e, label=LBL[p], color=COL[p],
                    marker=MK[p], linestyle=LS[p],
                    markeredgecolor=COL[p], markerfacecolor=(COL[p] if p=='WFQ' else 'white'),
                    capsize=3, elinewidth=0.9, capthick=0.9, clip_on=False, zorder=3)
    ax.set_xticks(XPOS); ax.set_xticklabels(SC_LABEL)
    ax.set_xlabel('Cenário de rede')
    ax.set_ylabel('Diferença de perda de FoV\nentre o pior e o melhor usuário (p.p.)')
    ax.grid(True, axis='y', alpha=0.3)
    ax.set_xlim(-0.25, len(SC_DATA)-0.75)
    ax.set_ylim(bottom=0)
    ax.margins(y=0.15)
    ax.legend(loc='upper left', framealpha=0.9)
    fig.tight_layout()
    out = os.path.join(OUTDIR, 'fig4_justica_usuarios.pdf')
    fig.savefig(out)
    fig.savefig(os.path.join(PNGDIR, 'fig4_preview.png'), dpi=110)
    plt.close(fig)
    print('salvo:', out)

if __name__ == '__main__':
    print('matrix:', MATRIX)
    print('outdir:', OUTDIR)
    make_fig1()
    make_fig4()
