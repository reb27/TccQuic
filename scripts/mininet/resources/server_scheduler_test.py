#!/usr/bin/env python3

# Uploaded via server_scheduler_test.sh

from mininet.net import Mininet, Host
from mininet.log import setLogLevel
from mininet.util import pmonitor
import os, signal, subprocess, sys
from subprocess import Popen
from utils import HostParams, NetParams, createMininet

SERVER_MODE = os.environ['SERVER_MODE']
ABR_MODE = os.environ.get('ABR_MODE', 'bola')
SERVER_BW = float(os.environ['SERVER_BW'])
CLIENT_BW = float(os.environ['CLIENT_BW'])
LOSS = float(os.environ['LOSS'])
PARALELLISM = int(os.environ['PARALELLISM'])
DELAY = '%fms' % float(os.environ['DELAY'])
LOAD = float(os.environ['LOAD'])
BASE_LATENCY = int(os.environ['BASE_LATENCY'])
FOV_TRACE_PATH = os.environ.get('FOV_TRACE_PATH', '')
BOLA_DEBUG_PATH = os.environ.get('BOLA_DEBUG_PATH', '')
BOLA_QMAX_SEGMENTS = os.environ.get('BOLA_QMAX_SEGMENTS', '')
NUM_CLIENTS = int(os.environ.get('NUM_CLIENTS', '1'))
# FOV_MIX: 'balanced' (33%W+33%N+33%S) ou 'wide_heavy' (50%W+33%N+17%S)
FOV_MIX = os.environ.get('FOV_MIX', 'balanced')

# Distribui traces de FoV entre N clientes conforme o mix configurado.
def fov_traces_for_mix(data_dir: str, num_clients: int, mix: str) -> 'list[str]':
    wide   = data_dir + '/data/user_fov_wide.csv'
    normal = data_dir + '/data/user_fov.csv'
    narrow = data_dir + '/data/user_fov_narrow.csv'
    if num_clients == 1:
        return [FOV_TRACE_PATH or normal]
    if mix == 'wide_heavy':
        # 50%W + 33%N + 17%S → para 6 clientes: 3W+2N+1S
        n_wide   = max(1, round(num_clients * 0.50))
        n_normal = max(1, round(num_clients * 0.33))
        n_narrow = max(0, num_clients - n_wide - n_normal)
    else:  # balanced
        # 33%W + 33%N + 33%S → para 6 clientes: 2W+2N+2S
        n_wide   = max(1, round(num_clients / 3))
        n_normal = max(1, round(num_clients / 3))
        n_narrow = max(0, num_clients - n_wide - n_normal)
    return ([wide] * n_wide) + ([normal] * n_normal) + ([narrow] * n_narrow)

print('SERVER_MODE=', SERVER_MODE)
print('ABR_MODE=', ABR_MODE)
print('SERVER_BW=', SERVER_BW)
print('CLIENT_BW=', CLIENT_BW)
print('LOSS=', LOSS)
print('PARALELLISM=', PARALELLISM)
print('DELAY=', DELAY)
print('LOAD=', LOAD)
print('BASE_LATENCY=', BASE_LATENCY)
print('FOV_TRACE_PATH=', FOV_TRACE_PATH)
print('BOLA_DEBUG_PATH=', BOLA_DEBUG_PATH)
print('BOLA_QMAX_SEGMENTS=', BOLA_QMAX_SEGMENTS)
print('NUM_CLIENTS=', NUM_CLIENTS)
print('FOV_MIX=', FOV_MIX)

def safe_print(msg: str):
    try:
        print(msg)
    except UnicodeEncodeError:
        # Some remote TTYs expose latin-1; keep process alive by replacing chars.
        enc = getattr(sys.stdout, 'encoding', None) or 'utf-8'
        data = msg.encode(enc, errors='replace')
        print(data.decode(enc, errors='replace'))

def validate_media_dataset(root_dir: str):
    segments_dir = os.path.join(root_dir, 'data', 'segments')
    if not os.path.isdir(segments_dir):
        raise RuntimeError(f'Missing segments directory: {segments_dir}')

    # Server expects files named:
    # video_tiled_{representation}_dash_track{segment}_{tile}.m4s
    # Validate at least one representation exists and capture examples.
    count = 0
    by_rep = {}
    sample = []
    for name in os.listdir(segments_dir):
        if name.startswith('video_tiled_') and '_dash_track' in name and name.endswith('.m4s'):
            count += 1
            try:
                rep = int(name.split('_')[2])
                by_rep[rep] = by_rep.get(rep, 0) + 1
            except Exception:
                pass
            if len(sample) < 5:
                sample.append(name)

    if count == 0:
        raise RuntimeError(
            'Dataset de segmentos incompatível: nenhum arquivo '
            'video_tiled_{rep}_dash_track*.m4s encontrado.'
        )

    safe_print(f'[dataset] encontrados {count} arquivos de segmento')
    if by_rep:
        reps = ', '.join(f'rep{rep}={n}' for rep, n in sorted(by_rep.items()))
        safe_print(f'[dataset] representacoes: {reps}')
    if sample:
        safe_print(f'[dataset] exemplos: {", ".join(sample)}')

class Test():
    def __init__(self):
        net, server, clients = createMininet(NetParams(
            server=HostParams(bw=SERVER_BW, delay=DELAY, loss=LOSS),
            clients=[HostParams(bw=CLIENT_BW)] * NUM_CLIENTS))
        self.server_policy: str = SERVER_MODE
        self.net: Mininet = net
        self.server: Host = server
        self.clients: 'list[Host]' = clients
        self.processes: 'dict[Host, Popen]' = {}
        self.iperf_server: 'Popen | None' = None
        self.iperf_client: 'Popen | None' = None

    def run(self):
        try:
            self.__run()
        finally:
            self.__finalize()

    def __run(self):
        print('Starting Mininet')
        self.net.start()
        if not self.net.waitConnected():
            raise RuntimeError('Failed to connect switches')
        setLogLevel('info')

        if LOAD != 0.0:
            load = '%dM' % (SERVER_BW * LOAD / 100)
            print('Starting load: ' + load)
            self.iperf_server = self.server.popen(
                ['iperf', '-u', '-s', '-p', '5001'])
            self.iperf_client = self.clients[0].popen(
                ['iperf', '-u', '-c', self.server.IP(), '-p', '5001',
                 '-b', load, '-t', '99999'])

        print('Running test')
        dir = os.path.dirname(os.path.realpath(__file__))
        validate_media_dataset(dir)

        # Start server
        self.processes[self.server] = self.server.popen(
            [dir + '/main', 'server', self.server_policy],
            cwd=dir,
            stderr=subprocess.STDOUT)

        # Distribui traces de FoV entre clientes conforme FOV_MIX
        fov_traces = fov_traces_for_mix(dir, NUM_CLIENTS, FOV_MIX)

        # Start N clients em paralelo
        for i, client in enumerate(self.clients):
            client_env = os.environ.copy()
            client_env['ABR_MODE'] = ABR_MODE
            client_env['FOV_TRACE_PATH'] = fov_traces[i] if i < len(fov_traces) else fov_traces[-1]
            if BOLA_DEBUG_PATH:
                client_env['BOLA_DEBUG_PATH'] = BOLA_DEBUG_PATH
            if BOLA_QMAX_SEGMENTS:
                client_env['BOLA_QMAX_SEGMENTS'] = BOLA_QMAX_SEGMENTS
            self.processes[client] = client.popen(
                [dir + '/main', 'test-client', self.server.IP(), str(PARALELLISM),
                 str(BASE_LATENCY)],
                cwd=dir,
                env=client_env,
                stderr=subprocess.STDOUT)
            safe_print(f'[client-{i}] fov={fov_traces[i] if i < len(fov_traces) else fov_traces[-1]}')

        expected = 1 + NUM_CLIENTS
        for host, line in pmonitor(self.processes):
            if line:
                if line.endswith('\n'):
                    line = line[:-1]
                safe_print('[%s] %s' % (host, line))
            if len(self.processes) != expected:
                break

    def __finalize(self):
        setLogLevel('warning')
        for process in self.processes.values():
            process.terminate()
        if self.iperf_client != None:
            self.iperf_client.terminate()
        if self.iperf_server != None:
            self.iperf_server.terminate()
            print('iperf server results:')
            result, _ = self.iperf_server.communicate()
            print(str(result, 'utf8'))
        self.net.stop()

Test().run()
