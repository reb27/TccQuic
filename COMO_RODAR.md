# Como rodar o projeto do início ao fim

Guia completo para reproduzir os experimentos do TCC, do `git clone` à geração
das figuras, passando pela emulação de rede com Mininet.

As instruções da **máquina de desenvolvimento** estão divididas em 🐧 **Linux** e
🪟 **Windows**. O **host Mininet é sempre Linux** (o Mininet não roda em Windows).

---

## 1. Como o sistema funciona (leia antes)

O experimento tem **duas máquinas**:

```
  Máquina de desenvolvimento              Host Mininet (SEMPRE Linux)
  🐧 Linux  ou  🪟 Windows                ─────────────────────────
  ────────────────────────       SSH      - Mininet instalado
  - repositório Git            ────────▶   - roda a rede emulada
  - Go 1.19 (cross-compila)      scp       - 1 servidor + N clientes
  - scripts .sh (orquestram)   ◀────────     num switch compartilhado
  - Python (análise/figuras)     CSVs
```

Os scripts `.sh` (na máquina de dev) **compilam o binário Go para Linux**, **enviam
por SSH** para o host Mininet junto com o vídeo e o script Python, **executam** o
teste lá, e **baixam os CSVs** de volta para `logs/`.

> **Você não precisa de Mininet na máquina de desenvolvimento.** Precisa de um
> **host Linux com Mininet** acessível por SSH. **Se você ainda não tem esse host,
> o passo 5 explica como criá-lo do zero (VM no VirtualBox) e descobrir o IP dele.**

---

## 2. Preparar a máquina de desenvolvimento

Você precisa de: **Go 1.19**, um **shell bash**, **ssh/scp** e **Python 3** (só para a análise).

### 🐧 Linux

```bash
# Go 1.19 (exato — quic-go 0.31 não compila em Go >= 1.20)
wget https://go.dev/dl/go1.19.13.linux-amd64.tar.gz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.19.13.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin        # coloque no ~/.bashrc
go version                                  # deve mostrar go1.19.x

# ssh, git, python já costumam vir instalados; se faltar:
sudo apt install -y git openssh-client python3 python3-pip
```

Bash, `ssh`, `scp` e `tar` já são nativos. Pode ir direto para o passo 3.

### 🪟 Windows

O jeito **recomendado** é o **WSL2** (Linux dentro do Windows) — aí você segue as
instruções 🐧 Linux de dentro dele. A alternativa nativa (Git Bash) também funciona.

**Opção A — WSL2 (recomendada):**

```powershell
# PowerShell como Administrador
wsl --install -d Ubuntu
# reinicie, crie usuário/senha do Ubuntu, e abra "Ubuntu" no menu
```

Dentro do Ubuntu (WSL), **siga a seção 🐧 Linux acima** — a partir daí é tudo igual.
O repositório pode ficar no disco do Windows (`/mnt/c/Users/...`) ou dentro do WSL.

**Opção B — Windows nativo (Git Bash):**

1. **Go 1.19**: baixe e instale `go1.19.13.windows-amd64.msi` de
   <https://go.dev/dl/> (precisa ser a 1.19, não a mais recente).
2. **Git for Windows** (traz o **Git Bash**): <https://git-scm.com/download/win>.
   Rode **todos** os comandos `bash`/`.sh` deste guia **dentro do Git Bash**.
3. **OpenSSH**: o Windows 10/11 já tem `ssh`/`scp` embutidos; o Git Bash também.
4. **Python 3**: instale de <https://python.org> (marque "Add to PATH").

Verifique no Git Bash:
```bash
go version      # go1.19.x
ssh -V          # OpenSSH...
python --version
```

---

## 3. Clonar e compilar

Em qualquer ambiente (🐧 Linux, ou 🪟 WSL/Git Bash):

```bash
git clone https://github.com/quic-streaming/tcc TccQuic
cd TccQuic
go mod download
```

O vídeo em tiles (`data/segments/`, ~255 MB, ~31 mil arquivos `.m4s`) e os traces
de FoV (`data/user_fov*.csv`) **já vêm no repositório** — não precisa baixar nada à parte.

---

## 4. Teste rápido local (opcional, sem Mininet)

Só para conferir que servidor e cliente compilam e conversam. Em **dois terminais**:

```bash
# Terminal 1 — servidor (escolha a política)
go run main.go server wfq        # ou: fifo, sp, voi_sp, voi_wfq

# Terminal 2 — cliente de teste
go run main.go test-client localhost 128 250
#                           ^ip       ^paralelismo ^latência_base_ms
```

Funciona igual em 🐧 Linux, 🪟 WSL e 🪟 Windows nativo. Não usa emulação de rede — é só sanity check.

---

## 5. Conseguir o host Mininet (o `<IP>` que os comandos pedem)

**O que é o "host"?** É uma **máquina Linux com Mininet** — separada da sua máquina
de dev. Todos os comandos que pedem `<IP_DO_HOST_MININET>` se referem ao **endereço
IP dessa máquina**. Você não roda nada de verdade até ter esse host no ar. Se você
ainda não tem, o caminho mais simples é criar uma **VM do Mininet no VirtualBox** —
e há um motivo: os scripts do projeto logam como usuário **`mininet`**, que é
exatamente o usuário da **imagem oficial da VM do Mininet**.

### 5.1. Criar a VM do Mininet (uma vez)

1. Instale o **VirtualBox**: <https://www.virtualbox.org/> (roda no Windows).
2. Baixe a **imagem oficial da VM do Mininet** (arquivo `.ovf`/`.zip`):
   <https://github.com/mininet/mininet/releases> → "Mininet VM images".
3. No VirtualBox: **Arquivo → Importar Appliance** → selecione o arquivo baixado.
4. Antes de ligar, ajuste a **rede** para permitir SSH:
   - Configurações da VM → **Rede** → Adaptador 1 → conectar como **"Placa em
     modo Bridge"** (Bridged). Assim a VM ganha um IP na sua rede local.
5. **Ligue a VM** e faça login:
   - usuário: **`mininet`**  ·  senha: **`mininet`**

### 5.2. Descobrir o IP da VM (esse é o seu `<IP>`)

Dentro da VM, rode:
```bash
ip addr show        # ou: ifconfig
```
Procure o endereço `inet` que **não** é `127.0.0.1` (algo como `192.168.0.42`).
**Esse número é o `<IP_DO_HOST_MININET>`** que você usa em todos os comandos.

> Se você importou a VM com "Bridged", o IP costuma ser da sua rede Wi-Fi/cabo
> (`192.168.x.x`). Anote-o.

### 5.3. Confirmar que o Mininet funciona

Ainda dentro da VM:
```bash
sudo mn --test pingall      # deve completar sem erro
```

### 5.4. Alternativas ao VirtualBox
- **Servidor Linux do laboratório** com Mininet: peça o IP e um usuário `mininet`
  (ou ajuste o usuário nos scripts). Aí você pula a criação da VM.
- **WSL2 como host** (avançado): funciona, mas o usuário do WSL **não é `mininet`**,
  então os scripts (que fixam `mininet@`) não funcionam sem editar. Só recomendo se
  você criar um usuário `mininet` no WSL. Para a maioria dos casos, a **VM é mais simples**.

### Acesso SSH sem senha (a partir da máquina de dev)

Os scripts logam como `mininet@<IP>` sem senha. Configure a chave uma vez, **da máquina de dev** (🐧 Linux ou 🪟 WSL/Git Bash):

```bash
# gere uma chave se ainda não tiver
ssh-keygen -t ed25519

# envie sua chave pública para o host (helper do repo)
cd scripts/mininet
./upload_ssh_key.sh <IP_DO_HOST_MININET>

# teste — tem que entrar SEM pedir senha
ssh mininet@<IP_DO_HOST_MININET>
```

> No 🪟 Windows nativo, faça isso pelo **Git Bash** (a chave vai para
> `C:\Users\<você>\.ssh`). Se o `upload_ssh_key.sh` não rodar, copie a chave
> manualmente: `ssh-copy-id mininet@<IP>` (Git Bash) ou cole o conteúdo de
> `~/.ssh/id_ed25519.pub` no `~/.ssh/authorized_keys` do host.

---

## 6. Rodar UM experimento

Os comandos abaixo rodam **na máquina de dev**, no shell bash (🐧 Linux nativo, ou
🪟 dentro do WSL/Git Bash), dentro de `scripts/mininet/`:

```bash
./server_scheduler_test.sh \
    --wfq \                 # política: --fifo, --sp ou --wfq
    --abr bola \            # ABR do cliente: bola ou legacy (vazão)
    --sbw 40 \              # banda do servidor em Mbps (o TCC usa 40)
    --delay 10 \            # atraso de base em ms
    --load 30 \             # carga de fundo em %
    --loss 0 \              # perda em %
    --clients 6 \           # nº de clientes simultâneos (1 ou 6)
    --fov-mix balanced \    # distribuição de FoV: balanced ou wide_heavy
    --beta 1.0 \            # expoente β do WFQ dinâmico (0.5, 1.0 ou 2.0)
    -p 120 \                # paralelismo interno do cliente
    <IP_DO_HOST_MININET>
```

O script compila, envia, executa e baixa os resultados para
`logs/server_scheduler_test/.../` (o `stdout` completo fica junto).

> Dica: acrescente `--no-build` nas execuções seguintes para **reaproveitar** o
> binário já enviado (pula a recompilação e o upload — bem mais rápido).

---

## 7. Rodar a MATRIZ completa (os 480 experimentos do TCC)

Este é o comando que gerou os dados da monografia (`logs/matrix-014`):

```bash
cd scripts/mininet
./run_matrix.sh --sbw 40 --reps 5 <IP_DO_HOST_MININET>
```

Executa **todas** as combinações:

> 4 cenários × 3 políticas (FIFO/SP/WFQ) × 2 ABR (BOLA/legacy) ×
> 2 nº de clientes (1/6) × 2 mix de FoV (balanced/wide_heavy) × 5 repetições
> = **480 execuções**

Opções úteis:
- `--dry-run` — lista o que seria executado, sem rodar.
- `--resume` — retoma a matriz mais recente pulando o que já concluiu (útil após queda).

Saída: `logs/matrix-NNN/<label>/rep<N>/`, com `label` tipo `s6_wfq_bola_6c_balanced`.

> ⚠️ **Demora horas.** Cada run reproduz ~86 s de vídeo. Rode em `screen`/`tmux`
> (🐧 Linux / 🪟 WSL) para sobreviver a quedas de SSH. No 🪟 Git Bash, mantenha a
> janela aberta ou prefira o WSL para tarefas longas.

---

## 8. Cenários de rede

O código e os logs usam os IDs originais `s1, s2, s5, s6`. Na monografia foram
**renumerados para #1–#4**. Mapeamento:

| Log (código) | Monografia | Atraso | Carga de fundo |
|:---:|:---:|:---:|:---:|
| `s1` | #1 | 24 ms | 10 % |
| `s2` | #2 | 24 ms | 30 % |
| `s5` | #3 | 10 ms | 10 % |
| `s6` | #4 | 10 ms | 30 % |

---

## 9. Estrutura de saída de cada run

```
logs/matrix-NNN/s6_wfq_bola_6c_balanced/rep1/
├── experiment.env              # todos os parâmetros da run
├── reqlog.csv                  # tempos por requisição (servidor)
├── class_agg.csv               # agregados por classe (bytes, delays, on-time)
├── server_summary.csv          # resumo final do servidor
├── queue_len.csv               # tamanho das filas (a cada 100 ms)
├── wfq_utilization.csv         # share real vs peso teórico
├── fov_assignment.csv          # qual perfil de FoV cada cliente recebeu
├── statistics-<pid>.csv        # detalhes por requisição (1 por cliente)
├── statistics-summary-<pid>.csv
├── fov-delivery-<pid>.csv      # taxa de entrega de FoV por segmento
└── fov-goodput-<pid>.csv       # goodput útil (FoV on-time)
```

---

## 10. Análise e geração das figuras (na máquina de dev)

Scripts em `scripts/mininet/resources/`. Instale as dependências Python:

```bash
# 🐧 Linux / 🪟 WSL
pip3 install numpy matplotlib pypdf
# 🪟 Windows nativo
pip install numpy matplotlib pypdf
```

Gerar as figuras estilo-paper a partir de uma matriz:
```bash
cd scripts/mininet/resources
python plot_paper_v2.py ../../../logs/matrix-014 -o ./figuras_saida
```

Regenerar as figuras da monografia (fig1 e fig4, com cenários #1–#4):
```bash
python regen_doc_figs.py
```

> 🪟 No Windows nativo, use `python`; no 🐧 Linux/WSL pode ser `python3`.

---

## 11. Solução de problemas

| Sintoma | Causa provável | Solução |
|---|---|---|
| `go build` falha com erros de quic-go | Go ≥ 1.20 | instale **Go 1.19.x** (obrigatório) |
| `.sh` não roda no Windows | usou CMD/PowerShell | rode os `.sh` no **Git Bash** ou **WSL** |
| `SSH connection failed!` | chave não instalada | `upload_ssh_key.sh <IP>` e teste `ssh mininet@<IP>` |
| Fim de linha estranho no `.sh` (🪟) | Git converteu para CRLF | `git config --global core.autocrlf false` e re-clone, ou `dos2unix scripts/mininet/*.sh` |
| Run trava / rede "suja" entre runs | estado residual do Mininet | no host: `sudo mn -c` (o `run_matrix.sh` já faz) |
| CSV do servidor só com cabeçalho | servidor encerrado antes do resumo | use as métricas do **cliente** (`statistics-*`) |
| Resultados diferentes do TCC | banda errada | reproduza com **`--sbw 40`** (padrão dos scripts é 60) |
| Muito lento a cada run | recompila toda vez | use `--no-build` a partir da 2ª run |

---

## 12. Resumo em 6 comandos (no shell bash da máquina de dev)

```bash
# 1. clonar e compilar
git clone https://github.com/quic-streaming/tcc TccQuic && cd TccQuic && go mod download

# 2. sanity local (opcional) — dois terminais
go run main.go server wfq
go run main.go test-client localhost 128 250

# 3. chave SSH para o host Mininet
cd scripts/mininet && ./upload_ssh_key.sh <IP>

# 4. um experimento
./server_scheduler_test.sh --wfq --abr bola --sbw 40 --delay 10 --load 30 --clients 6 --fov-mix balanced <IP>

# 5. matriz completa (480 runs) — os dados do TCC
./run_matrix.sh --sbw 40 --reps 5 <IP>

# 6. figuras
cd resources && python regen_doc_figs.py
```

> 🐧 Linux: rode direto no terminal.
> 🪟 Windows: rode tudo dentro do **WSL (Ubuntu)** — recomendado — ou do **Git Bash**.
> O host Mininet (`<IP>`) é sempre uma máquina Linux separada com Mininet.
